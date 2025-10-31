// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/elasticache"
	tagging "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/prometheus/client_golang/prometheus"
)

const MaxMetricDataQueryItems = 500

// Client implements the set of AWS service methods used in the collectors. We
// use a small subset of what the AWS SDK provides across a multitude of
// service packages, this interface helps us to easily keep track of that usage
// and implement testing clients.
type Client interface {
	DescribeAutoScalingGroups(*autoscaling.DescribeAutoScalingGroupsInput, *CollectorTelemetry) (*[]*autoscaling.Group, error)
	DescribeCacheClusters(*elasticache.DescribeCacheClustersInput, *CollectorTelemetry) (*[]*elasticache.CacheCluster, error)
	GetResources(*tagging.GetResourcesInput, *CollectorTelemetry) (*[]*tagging.ResourceTagMapping, error)
	GetMetricData([]*cloudwatch.GetMetricDataInput, *CollectorTelemetry) (*[]*cloudwatch.MetricDataResult, error)
}

// AWSClient implements the Client interface and provides the AWS requests we
// use throughout the project.
type AWSClient struct {
	Region      string
	MaxRetries  int
	sess        *session.Session
	tagging     *tagging.ResourceGroupsTaggingAPI
	cloudwatch  *cloudwatch.CloudWatch
	autoscaling *autoscaling.AutoScaling
	elasticache *elasticache.ElastiCache
}

func defaultSession(region string) (*session.Session, error) {
	retryer := client.DefaultRetryer{
		NumMaxRetries:    5,
		MinThrottleDelay: 500 * time.Millisecond,
		MaxThrottleDelay: 3 * time.Second,
		MinRetryDelay:    10 * time.Millisecond,
		MaxRetryDelay:    3 * time.Second,
	}
	// level := aws.LogDebugWithHTTPBody
	return session.NewSession(&aws.Config{
		Region:     aws.String(region),
		MaxRetries: aws.Int(5),
		Retryer:    retryer,
		// LogLevel:   &level,
	})
}

// DefaultAWSClient returns a default AWSClient for the provided region with max
// retries set to 5 and all other values being set as in a stock aws.Config.
func DefaultAWSClient(region string) (Client, error) {
	sess, err := defaultSession(region)
	if err != nil {
		return nil, err
	}

	return &AWSClient{
		Region: *sess.Config.Region,
		sess:   sess,
	}, nil
}

func (client *AWSClient) getTaggingAPI() *tagging.ResourceGroupsTaggingAPI {
	if client.tagging != nil {
		return client.tagging
	}

	client.tagging = tagging.New(client.sess)

	return client.tagging
}

func (client *AWSClient) getCloudwatch() *cloudwatch.CloudWatch {
	if client.cloudwatch != nil {
		return client.cloudwatch
	}

	client.cloudwatch = cloudwatch.New(client.sess)

	return client.cloudwatch
}

func (client *AWSClient) getAutoscaling() *autoscaling.AutoScaling {
	client.autoscaling = autoscaling.New(client.sess)

	return client.autoscaling
}

func (client *AWSClient) getElasticache() *elasticache.ElastiCache {
	client.elasticache = elasticache.New(client.sess)

	return client.elasticache
}

// GetResources proxies to
// resourcegroupstaggingapi.GetGetResourcesPagesWithContext and handles
// aggregation of the paged results.
func (client *AWSClient) GetResources(input *tagging.GetResourcesInput, tele *CollectorTelemetry) (*[]*tagging.ResourceTagMapping, error) {
	res := []*tagging.ResourceTagMapping{}
	ctx := context.Background()
	api := client.getTaggingAPI()

	err := api.GetResourcesPagesWithContext(ctx, input, callback(&res, tele.GetResourcesCount))
	return &res, err
}

func callback(res *[]*tagging.ResourceTagMapping, counter prometheus.Counter) func(page *tagging.GetResourcesOutput, lastPage bool) bool {
	return func(page *tagging.GetResourcesOutput, lastPage bool) bool {
		defer counter.Inc()
		*res = append(*res, page.ResourceTagMappingList...)
		return page.PaginationToken != nil
	}
}

// GetResources proxies to cloudwatch.GetMetricDataPage and handles aggregation
// of the paged results. The requests are issued concurrently.
func (client *AWSClient) GetMetricData(in []*cloudwatch.GetMetricDataInput, tele *CollectorTelemetry) (*[]*cloudwatch.MetricDataResult, error) {
	type lock struct {
		sync.Mutex
		r []*cloudwatch.MetricDataResult
	}
	res := lock{
		r: []*cloudwatch.MetricDataResult{},
	}
	wg := sync.WaitGroup{}
	for _, input := range in {
		wg.Add(1)
		go func(ip *cloudwatch.GetMetricDataInput) {
			defer wg.Done()
			err := client.getCloudwatch().GetMetricDataPages(ip, func(page *cloudwatch.GetMetricDataOutput, last bool) bool {
				defer tele.GetMetricDataCount.Inc()
				res.Lock()
				res.r = append(res.r, page.MetricDataResults...)
				res.Unlock()
				return !last
			})

			if err != nil {
				Logger.Error("GetMetricData:", err.Error())
				tele.ErrorCount.Inc()
			}
		}(input)
	}
	wg.Wait()

	return &res.r, nil
}

func (client *AWSClient) DescribeAutoScalingGroups(input *autoscaling.DescribeAutoScalingGroupsInput, tele *CollectorTelemetry) (*[]*autoscaling.Group, error) {
	type lock struct {
		sync.Mutex
		r []*autoscaling.Group
	}
	res := lock{
		r: []*autoscaling.Group{},
	}

	err := client.getAutoscaling().DescribeAutoScalingGroupsPages(input, func(page *autoscaling.DescribeAutoScalingGroupsOutput, last bool) bool {
		tele.DescribeAutoScalingGroupsCount.Inc()
		res.Lock()
		res.r = append(res.r, page.AutoScalingGroups...)
		res.Unlock()
		return !last
	})

	if err != nil {
		Logger.Error("DescribeAutoScalingGroups:", err.Error())
		tele.ErrorCount.Inc()
	}

	return &res.r, err
}

func (client *AWSClient) DescribeCacheClusters(input *elasticache.DescribeCacheClustersInput, tele *CollectorTelemetry) (*[]*elasticache.CacheCluster, error) {
	type lock struct {
		sync.Mutex
		r []*elasticache.CacheCluster
	}
	res := lock{
		r: []*elasticache.CacheCluster{},
	}

	err := client.getElasticache().DescribeCacheClustersPages(input, func(page *elasticache.DescribeCacheClustersOutput, last bool) bool {
		tele.DescribeElasticacheCacheClustersCount.Inc()
		res.Lock()
		res.r = append(res.r, page.CacheClusters...)
		res.Unlock()
		return !last
	})

	if err != nil {
		Logger.Error("DescribeElasticacheCacheClusters]:", err.Error())
		tele.ErrorCount.Inc()
	}

	return &res.r, err
}
