// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"bytes"

	// sha1 is good enough for this use case, disabling linter
	"crypto/sha1" // nolint:gosec
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	t "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	tagging "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
)

// TimestampAscending is used to sort results received from CloudWatch
var TimestampAscending = "TimestampAscending"

var ErrCanNotParseARN = errors.New("Can not parse the provided ARN")
var ErrNoSuchCollectorType = errors.New("Unknown collector type in configuration")

type CollectorID string

// implementations of extraTags should take a resource mapping and create a list
// of tags mixing in any additional tags that should show up on the Prometheus
// metrcis as labels.
type extraTags func(*tagging.ResourceTagMapping) ([]*tagging.Tag, error)

// implementations of metricDimensions should produce dimensions to query
// CloudWatch with from a resource tag mapping.
type metricDimensions func(*tagging.ResourceTagMapping) ([]*cloudwatch.Dimension, error)

// implementations of resourceGetter should get a list of AWS resources from any
// source (AWS APIs or otherwise) and prepare a ResourceIndex that can be used
// to get metrics from CloudWatch.
type resourceGetter func() (*ResourceIndex, error)

// CollectorType specifies basic properties and behaviour of collectors.
type CollectorType struct {
	ResourceName   string
	Namespace      string
	Dimension      string
	ResourcePrefix string
}

// collectorTypes is a map of collector types for resources that are supported
// by the AWS ResourceGroupsTaggingAPI.
var collectorTypes = map[string]*CollectorType{
	"alb": {
		ResourceName:   "elasticloadbalancing:loadbalancer/app",
		Namespace:      "AWS/ApplicationELB",
		Dimension:      "LoadBalancer",
		ResourcePrefix: "loadbalancer/",
	},
	"ebs": {
		ResourceName:   "ec2:volume",
		Namespace:      "AWS/EBS",
		Dimension:      "VolumeId",
		ResourcePrefix: "volume/",
	},
	"ec": {
		ResourceName:   "elasticache:cluster",
		Namespace:      "AWS/ElastiCache",
		Dimension:      "CacheClusterId",
		ResourcePrefix: "cluster:",
	},
	"elb": {
		ResourceName:   "elasticloadbalancing:loadbalancer",
		Namespace:      "AWS/ELB",
		Dimension:      "LoadBalancerName",
		ResourcePrefix: "loadbalancer/",
	},
	"nlb": {
		ResourceName:   "elasticloadbalancing:loadbalancer/net",
		Namespace:      "AWS/NetworkELB",
		Dimension:      "LoadBalancer",
		ResourcePrefix: "loadbalancer/",
	},
	"sqs": {
		ResourceName:   "sqs",
		Namespace:      "AWS/SQS",
		Dimension:      "QueueName",
		ResourcePrefix: "",
	},
	"rds": {
		ResourceName:   "rds:db",
		Namespace:      "AWS/RDS",
		Dimension:      "DBInstanceIdentifier",
		ResourcePrefix: "db:",
	},
	"neptune": {
		ResourceName:   "rds:db",
		Namespace:      "AWS/Neptune",
		Dimension:      "DBInstanceIdentifier",
		ResourcePrefix: "db:",
	},
}

func CollectorFromConfig(c CollectorConfig) (MetricCollector, error) {
	if t, ok := collectorTypes[c.Type]; ok {
		Logger.Debugf("Found collector type %s", c.Type)

		return &BaseCollector{
			config:         c,
			namespace:      t.Namespace,
			resourceName:   t.ResourceName,
			dimension:      t.Dimension,
			resourcePrefix: t.ResourcePrefix,
		}, nil
	}

	switch c.Type {
	case "asg":
		Logger.Debug("Found asg collector type")
		return NewASGCollector(c)
	case "ec_host":
		Logger.Debug("Found ec_host collector type")
		return NewECHostCollector(c)
	}

	return nil, ErrNoSuchCollectorType
}

// CollectorProc represents a running collector. It is used to signal the
// collector to stop and to know when the collector is done, which usually means
// an unrecoverable error happened. In that case it is up to the caller to
// handle the situation.
type CollectorProc struct {
	ID CollectorID
	// Done will receive a collector whenever it stops running to allow further
	// inspection when required. Also when it was stopped using the stop
	// channel.
	Done chan MetricCollector
	// Stop signals the collector to shut down.
	Stop chan string
	// Store makes the internal store of a collector available, e.g. to
	// aggregate metrics in an HTTP handler.
	Store Store
}

// MetricCollector is the interface used to abstract out the collection of
// metrics from CloudWatch. It is the type the high level business logic is
// build around.
type MetricCollector interface {
	// Valid returns true if a collector is valid. Being valid should make sure
	// the configuration of the collector is correct and ensure the collector is
	// likely to be working when started.
	Valid() bool
	// Run starts a collector returning the CollectorProc that allows to
	// interface with the running collector.
	Run() *CollectorProc
}

// TagFilter is a key value pair used to filter for specific resources with
// matching tags in AWS.
type TagFilter struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

// MetricStat is a pair of metric name and a specific kind of statistic like sum
// or average. It is used to request those metrics from CloudWatch.
type MetricStat struct {
	MetricName string `yaml:"name"`
	Stat       string `yaml:"stat"`
}

// Time wraps around time.Now() to make testing easier in case the current time
// is used in the code.
type Time interface {
	Now() time.Time
}

type realTime struct{}

func (t *realTime) Now() time.Time {
	return time.Now()
}

type testTime struct {
	now *time.Time
}

func (t *testTime) Now() time.Time {
	// pin time.Now() to the first usage to make comparisons easier in testing
	if t.now == nil {
		now := time.Now()
		t.now = &now
	}

	return *t.now
}

// id creates a sha1 from the resource ARN provided by AWS
func id(r *t.ResourceTagMapping) string {
	// sha1 is good enough for this use case, disabling linter
	h := sha1.New() // nolint:gosec
	_, _ = h.Write([]byte(*r.ResourceARN))
	return fmt.Sprintf("%x", h.Sum(nil))
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

// toSnakeCase is a naive snake casing function, see the test cases for more
// details.
func toSnakeCase(str string) string {
	s := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	s = matchAllCap.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(s)
}

// sanitize converts a string into a Prometheus compatible label key. Certain
// characters are not supported and have to be scrubbed or replaced.
func sanitize(str string) string {
	replacer := strings.NewReplacer(
		" ", "_",
		",", "_",
		".", "_",
		":", "_",
		"-", "_",
		"=", "_",
		"/", "_",
		"%", "_pct",
	)
	return replacer.Replace(str)
}

// escapeValue escapes double quotes in label values to avoid syntax errors
// stringifying the metrics keys and values later on.
func escapeValue(str string) string {
	replacer := strings.NewReplacer(
		`"`, `\"`,
		`\`, `\\`,
	)
	return replacer.Replace(str)
}

// ResourceIndex holds resources, queries, and results throughout the lifetime
// of CloudWatch metrics query done by PromWatch. Using this index allows fast
// access to queries, results, and resources correlated by the same IDs (used as
// index keys) when iterating over one of the indices.
type ResourceIndex struct {
	// Queries and Results are used for all collectors
	Queries map[string][]*cloudwatch.MetricDataQuery
	Results map[string]*cloudwatch.MetricDataResult
	// Resources is used for all services that are supported by the
	// resourcegroupstaggingapi
	Resources map[string]*t.ResourceTagMapping
}

// NewResourceIndex returns *ResourceIndex with initialized properties.
func NewResourceIndex() *ResourceIndex {
	return &ResourceIndex{
		Queries:   make(map[string][]*cloudwatch.MetricDataQuery),
		Results:   make(map[string]*cloudwatch.MetricDataResult),
		Resources: make(map[string]*t.ResourceTagMapping),
	}
}

// NewResourceIndexFromTagMapping creates a *ResourceIndex from a resource tag
// mapping and an extractor function that will create an ID used to correlate
// resources, queries, and results.
func NewResourceIndexFromTagMapping(r *[]*t.ResourceTagMapping, ex func(*t.ResourceTagMapping) string) *ResourceIndex {
	index := NewResourceIndex()

	for _, item := range *r {
		index.Resources[ex(item)] = item
	}

	return index
}

func (i *ResourceIndex) AddResults(res *[]*cloudwatch.MetricDataResult) {
	for _, r := range *res {
		i.Results[*r.Id] = r
	}
}

// tagsToString transforms tags into a string of Prometheus compatible metrics
// labels.
func tagsToString(tags []*t.Tag) string {
	buf := bytes.Buffer{}
	for i, t := range tags {
		sep := ","
		if i == len(tags)-1 {
			sep = ""
		}

		fmt.Fprintf(&buf, `%s="%s"%s`, toSnakeCase(sanitize(*t.Key)), escapeValue(*t.Value), sep)
	}

	return buf.String()
}

// convertTags transforms AWS tags and extra tags into a string of Prometheus
// compatible metrics labels.
func convertTags(resource *t.ResourceTagMapping, mergeTags []string, tags ...*t.Tag) string {
	merge := map[string]struct{}{}

	for _, t := range mergeTags {
		merge[t] = struct{}{}
	}

	for _, t := range resource.Tags {
		if _, ok := merge[*t.Key]; ok {
			tags = append(tags, t)
		}
	}

	return tagsToString(tags)
}

// defaultExtraTags returns an extraTags function that adds the resource arn and
// dimension to the tags that end up being Prometheus compatible metrics labels.
func defaultExtraTags(dimension, resourcePrefix string) extraTags {
	return func(resource *tagging.ResourceTagMapping) ([]*tagging.Tag, error) {
		tags := []*tagging.Tag{
			{
				Key:   aws.String("arn"),
				Value: resource.ResourceARN,
			},
		}

		arn, err := arn.Parse(*resource.ResourceARN)
		if err != nil {
			return tags, ErrCanNotParseARN
		}

		val := strings.TrimPrefix(arn.Resource, resourcePrefix)
		tags = append(tags, &tagging.Tag{
			Key:   aws.String(dimension),
			Value: aws.String(val),
		})

		return tags, nil
	}
}

// defaultMetricDimension returns a metricDimentions function that uses the
// dimension and resource prefix to derive the dimension value from passed in
// resources.
func defaultMetricDimension(dimension, resourcePrefix string) metricDimensions {
	return func(resource *tagging.ResourceTagMapping) ([]*cloudwatch.Dimension, error) {
		arn, err := arn.Parse(*resource.ResourceARN)
		if err != nil {
			return []*cloudwatch.Dimension{}, ErrCanNotParseARN
		}

		val := strings.TrimPrefix(arn.Resource, resourcePrefix)

		return []*cloudwatch.Dimension{{Name: aws.String(dimension), Value: aws.String(val)}}, nil
	}
}
