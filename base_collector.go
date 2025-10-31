// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"bytes"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	tagging "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// BaseCollector implements common functionality for most collectors.
type BaseCollector struct {
	config    CollectorConfig
	_client   Client
	store     Store
	time      Time
	telemetry *CollectorTelemetry
	id        uuid.UUID

	resourceName   string
	namespace      string
	dimension      string
	resourcePrefix string
}

// Valid checks BaseCollector and returns true in case of valid internal state.
// In case of invalid state it sets errors that can be collected with the
// .Errors() method and returns false.
func (b *BaseCollector) Valid() bool {
	if b.config.Offset < b.config.Interval {
		err := fmt.Errorf("offset must be greater than interval. Offset: %d, Interval: %d", b.config.Offset, b.config.Interval)
		_ = b.HandleError(err)
		return false
	}

	return true
}

// HandleError logs errors, increases error counters, and returns the error
// unchanged.
func (b *BaseCollector) HandleError(err error) error {
	if err != nil {
		Logger.Error(err)
		b.Telemetry().ErrorCount.Inc()
	}

	return err
}

// Time returns a time struct implementing Now() that either represents
// time.Now() or a static time used for testing.
func (b *BaseCollector) Time() Time {
	if b.time != nil {
		return b.time
	}

	return &realTime{}
}

// Telemetry returns the collector specific metrics aggregator. If it does not
// exist a new one will be initialized.
func (b *BaseCollector) Telemetry() *CollectorTelemetry {
	if b.telemetry == nil {
		b.telemetry = NewCollectorTelemetry(prometheus.Labels{
			"collector_id":   string(b.ID()),
			"collector_name": b.config.Name,
			"collector_type": b.config.Type,
		})
	}

	return b.telemetry
}

// ID returns a UUID that identifies a collector and does not change throuout a
// collector's life time.
func (b *BaseCollector) ID() CollectorID {
	if b.id == uuid.Nil {
		b.id, _ = uuid.NewUUID()
	}

	return CollectorID(b.id.String())
}

// getResourcesInput prepares the input for the request to the
// ResourceGroupsTaggingAPI with the resource type and configured tag filters.
func (b *BaseCollector) getResourcesInput(resourceType string) *tagging.GetResourcesInput {
	in := tagging.GetResourcesInput{
		ResourceTypeFilters: []*string{aws.String(resourceType)},
		TagFilters:          []*tagging.TagFilter{},
	}

	for _, f := range b.config.TagFilters {
		in.TagFilters = append(in.TagFilters, &tagging.TagFilter{
			Key:    aws.String(f.Key),
			Values: []*string{aws.String(f.Value)},
		})
	}

	return &in
}

// storeResults takes a *ResourceIndex and transforms the query results stored
// in it into prometheus compatible metrics and stores them in a buffer that
// gets used when the metrics get requested.
func (b *BaseCollector) storeResults(index *ResourceIndex) {
	buf := bytes.Buffer{}
	for id, r := range index.Resources {
		Logger.Debugw(*r.ResourceARN, "id", b.ID(), "name", b.config.Name, "type", b.config.Type)
		tags, err := defaultExtraTags(b.dimension, b.resourcePrefix)(r)
		_ = b.HandleError(err)
		t := convertTags(r, b.config.MergeTags, tags...)
		for _, query := range index.Queries[id] {
			res, ok := index.Results[*query.Id]
			if !ok {
				Logger.Warn(*query.Id, " not found in results")
				continue
			}
			for i, v := range res.Values {
				fmt.Fprintf(
					&buf,
					"promwatch_aws_%s_%s_%s{%s} %f %d\n",
					b.config.Type,
					toSnakeCase(sanitize(*query.MetricStat.Metric.MetricName)),
					toSnakeCase(sanitize(*query.MetricStat.Stat)),
					t,
					*v,
					index.Results[*query.Id].Timestamps[i].Unix()*1000)
			}
		}
	}
	b.store.Add(buf.String())
	b.store.Commit()
}

// makeQueries produces a list of CloudWatch metrics data queries from the
// resources in the passed in ResourceIndex and the collector config that
// defines the metrics that are supposed to be queried.
func (b *BaseCollector) makeQueries(index *ResourceIndex, namespace string, dimensions metricDimensions) []*cloudwatch.MetricDataQuery {
	dataQuery := []*cloudwatch.MetricDataQuery{}
	for id, r := range index.Resources {
		for i, s := range b.config.MetricStats {
			d, err := dimensions(r)
			if err != nil {
				_ = b.HandleError(err)
				continue
			}
			query := cloudwatch.MetricDataQuery{
				Id: aws.String(fmt.Sprintf("%s_%s_%d", "id", id, i)),
				MetricStat: &cloudwatch.MetricStat{
					Metric: &cloudwatch.Metric{
						Dimensions: d,
						MetricName: aws.String(s.MetricName),
						Namespace:  aws.String(namespace),
					},
					Period: aws.Int64(int64(b.config.Period)),
					Stat:   aws.String(s.Stat),
				},
			}
			dataQuery = append(dataQuery, &query)
			index.Queries[id] = append(index.Queries[id], &query)
		}
	}

	return dataQuery
}

// getMetricDataInput prepares the request payloads to query CloudWatch based on
// listed resources and the collector configuration. It will ensure each request
// only contains the allowed number of query items.
func (b *BaseCollector) getMetricDataInput(index *ResourceIndex, dim metricDimensions) []*cloudwatch.GetMetricDataInput {
	dataQuery := b.makeQueries(index, b.namespace, dim)
	ins := []*cloudwatch.GetMetricDataInput{}

	endTime := b.Time().Now().UTC().Add(time.Duration(-b.config.Offset) * time.Second)
	startTime := endTime.Add(time.Duration(-b.config.Interval) * time.Second)

	// Create a new getMetricDataInput for every MaxMetricDataQueryItems.
	for i := 0; i < len(dataQuery); i += MaxMetricDataQueryItems {
		end := i + MaxMetricDataQueryItems

		if end > len(dataQuery) {
			end = len(dataQuery)
		}

		in := &cloudwatch.GetMetricDataInput{
			EndTime:   &endTime,
			StartTime: &startTime,
			// Order matters later in the Prometheus metrics output where
			// timestamps have to be ordered as Prometheus will only ingest
			// ascending timestamps for the same time series.
			ScanBy:            &TimestampAscending,
			MetricDataQueries: dataQuery[i:end],
		}

		ins = append(ins, in)
	}

	return ins
}

// collect issues the requests to CloudWatch and transforms and stores the
// results.
func (b *BaseCollector) collect(getResources resourceGetter, dim metricDimensions) error {
	start := time.Now()
	Logger.Debugw("starting to collect", "id", b.ID(), "name", b.config.Name, "type", b.config.Type)
	defer func() {
		b.Telemetry().RunCount.Inc()
		b.Telemetry().RunDuration.Set(time.Since(start).Seconds())
	}()

	if getResources == nil {
		getResources = b.getResources
	}

	index, err := getResources()
	if err != nil {
		return err
	}
	b.Telemetry().MatchingResources.Set(float64(len(index.Resources)))

	b.getMetrics(index, dim)
	duration := time.Since(start)

	Logger.Debugw(fmt.Sprintf("Finished after %.2fs", duration.Seconds()), "id", b.ID(), "name", b.config.Name, "type", b.config.Type)
	return nil
}

func (b *BaseCollector) client() (Client, error) {
	// Check if a client is set explicitly (usually for testing) and create a
	// new one otherwise.
	client := b._client
	if client == nil {
		return DefaultAWSClient(b.config.Region)
	}

	return client, nil

}

func (b *BaseCollector) getResources() (*ResourceIndex, error) {
	client, err := b.client()
	if err != nil {
		return nil, err
	}

	input := b.getResourcesInput(b.resourceName)
	resources, err := client.GetResources(input, b.Telemetry())
	if err != nil {
		return nil, err
	}

	return NewResourceIndexFromTagMapping(resources, id), nil
}

func (b *BaseCollector) getMetrics(index *ResourceIndex, dim metricDimensions) {
	in := b.getMetricDataInput(index, dim)

	client, err := b.client()
	if err != nil {
		_ = b.HandleError(err)
		return
	}

	res, err := client.GetMetricData(in, b.Telemetry())
	if err != nil {
		_ = b.HandleError(err)
	}
	index.AddResults(res)

	go b.storeResults(index)
}

// run starts the collection job that periodically queries CloudWatch for
// metrics. It is also the place to hook in other collectors that embed the base
// collector as the parameters define the source of resources and what dimension
// to use for the metrics queries.
func (b *BaseCollector) run(getResources resourceGetter, dim metricDimensions) *CollectorProc {
	b.store = NewStore()
	proc := CollectorProc{
		ID:    b.ID(),
		Store: b.store,
		Done:  make(chan MetricCollector),
		Stop:  make(chan string),
	}

	go func() {
		// run once before starting the loop ticker
		_ = b.HandleError(b.collect(getResources, dim))
		for {
			select {
			case <-time.After(time.Duration(b.config.Interval) * time.Second):
				_ = b.HandleError(b.collect(getResources, dim))
			case <-proc.Stop:
				proc.Done <- b
				return
			}
		}
	}()

	return &proc
}

// Run starts the base collector.
func (b *BaseCollector) Run() *CollectorProc {
	return b.run(nil, defaultMetricDimension(b.dimension, b.resourcePrefix))
}

// withTime is only required for testing to have static deterministic time
// during test runs.
func (b *BaseCollector) withTime(t Time) *BaseCollector {
	b.time = t

	return b
}
