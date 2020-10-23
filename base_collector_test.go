// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"sort"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	tagging "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/stretchr/testify/assert"
)

func TestValid(t *testing.T) {
	cases := []struct {
		collector *BaseCollector
		expected  bool
		message   string
	}{
		{
			collector: &BaseCollector{
				config: CollectorConfig{
					Type:     "ebs",
					Offset:   1,
					Interval: 2,
				},
			},
			expected: false,
			message:  "Offset smaller than Interval should be invalid",
		},
		{
			collector: &BaseCollector{
				config: CollectorConfig{
					Type:     "ebs",
					Offset:   2,
					Interval: 2,
				},
			},
			expected: true,
			message:  "Offset equal to Interval should be valid",
		},
		{
			collector: &BaseCollector{
				config: CollectorConfig{
					Type:     "ebs",
					Offset:   3,
					Interval: 2,
				},
			},
			expected: true,
			message:  "Offset larger than Interval should be valid",
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, c.collector.Valid(), c.message)
	}
}

func TestGetResourcesInput(t *testing.T) {
	testType := "some:type"
	cases := []struct {
		collector *BaseCollector
		expected  *tagging.GetResourcesInput
		message   string
	}{
		{
			collector: &BaseCollector{config: CollectorConfig{}},
			expected: &tagging.GetResourcesInput{
				ResourceTypeFilters: []*string{aws.String(testType)},
				TagFilters:          []*tagging.TagFilter{},
			},
			message: "Empty EBS collector config should produce query for all volumes",
		},
		{
			collector: &BaseCollector{
				config: CollectorConfig{
					TagFilters: []TagFilter{
						{
							Key:   "tagKey",
							Value: "tagValue",
						},
						{
							Key:   "anotherTagKey",
							Value: "anotherTagValue",
						},
					},
				},
			},
			expected: &tagging.GetResourcesInput{
				ResourceTypeFilters: []*string{aws.String(testType)},
				TagFilters: []*tagging.TagFilter{
					{
						Key:    aws.String("tagKey"),
						Values: []*string{aws.String("tagValue")},
					},
					{
						Key:    aws.String("anotherTagKey"),
						Values: []*string{aws.String("anotherTagValue")},
					},
				},
			},
			message: "Empty EBS collector config should produce query for all volumes",
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, c.collector.getResourcesInput(testType), c.message)
	}
}

func TestMakeQueries(t *testing.T) {
	cases := []struct {
		collector      *BaseCollector
		resources      []*tagging.ResourceTagMapping
		expected       []*cloudwatch.MetricDataQuery
		expectedErrors []error
		message        string
	}{
		{
			message:   "Empty entities should produce empty results",
			collector: stripInterface(CollectorFromConfig(CollectorConfig{Type: "ebs"})),
			resources: []*tagging.ResourceTagMapping{},
			expected:  []*cloudwatch.MetricDataQuery{},
		},
		{
			message:   "Invalid ARNs should produce errors",
			collector: stripInterface(CollectorFromConfig(CollectorConfig{Type: "ebs"})),
			resources: []*tagging.ResourceTagMapping{
				{
					ResourceARN: aws.String("broken"),
				},
			},
			expected: []*cloudwatch.MetricDataQuery{},
			expectedErrors: []error{
				ErrCanNotParseARN,
			},
		},
		{
			message:   "Empty metric stats should produce empty results",
			collector: stripInterface(CollectorFromConfig(CollectorConfig{Type: "ebs"})),
			resources: []*tagging.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:ec2:us-east-1:000000000000:volume/vol-fffffffffffffffff"),
				},
			},
			expected: []*cloudwatch.MetricDataQuery{},
		},
		{
			message: "Resources should be properly zipped into metric data queries",
			collector: stripInterface(CollectorFromConfig(CollectorConfig{
				Type:   "ebs",
				Period: 300,
				MetricStats: []MetricStat{
					{
						MetricName: "MyMetricName",
						Stat:       "Sum",
					},
					{
						MetricName: "MyOtherMetricName",
						Stat:       "Average",
					},
				},
			})),
			resources: []*tagging.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:ec2:us-east-1:000000000000:volume/vol-fffffffffffffffff"),
				},
				{
					ResourceARN: aws.String("arn:aws:ec2:us-east-1:000000000000:volume/vol-00000000000000000"),
				},
			},
			expected: []*cloudwatch.MetricDataQuery{
				{
					Id: aws.String("id_43c1360ea31ff82de65453d44cabeb5307b8a1f5_0"),
					MetricStat: &cloudwatch.MetricStat{
						Stat:   aws.String("Sum"),
						Period: aws.Int64(300),
						Metric: &cloudwatch.Metric{
							MetricName: aws.String("MyMetricName"),
							Namespace:  aws.String("AWS/EBS"),
							Dimensions: []*cloudwatch.Dimension{
								{
									Name:  aws.String("VolumeId"),
									Value: aws.String("vol-00000000000000000"),
								},
							},
						},
					},
				},
				{
					Id: aws.String("id_43c1360ea31ff82de65453d44cabeb5307b8a1f5_1"),
					MetricStat: &cloudwatch.MetricStat{
						Stat:   aws.String("Average"),
						Period: aws.Int64(300),
						Metric: &cloudwatch.Metric{
							MetricName: aws.String("MyOtherMetricName"),
							Namespace:  aws.String("AWS/EBS"),
							Dimensions: []*cloudwatch.Dimension{
								{
									Name:  aws.String("VolumeId"),
									Value: aws.String("vol-00000000000000000"),
								},
							},
						},
					},
				},
				{
					Id: aws.String("id_d714b664b1f99367e6962cabb2463495ce4aa395_0"),
					MetricStat: &cloudwatch.MetricStat{
						Stat:   aws.String("Sum"),
						Period: aws.Int64(300),
						Metric: &cloudwatch.Metric{
							MetricName: aws.String("MyMetricName"),
							Namespace:  aws.String("AWS/EBS"),
							Dimensions: []*cloudwatch.Dimension{
								{
									Name:  aws.String("VolumeId"),
									Value: aws.String("vol-fffffffffffffffff"),
								},
							},
						},
					},
				},
				{
					Id: aws.String("id_d714b664b1f99367e6962cabb2463495ce4aa395_1"),
					MetricStat: &cloudwatch.MetricStat{
						Stat:   aws.String("Average"),
						Period: aws.Int64(300),
						Metric: &cloudwatch.Metric{
							MetricName: aws.String("MyOtherMetricName"),
							Namespace:  aws.String("AWS/EBS"),
							Dimensions: []*cloudwatch.Dimension{
								{
									Name:  aws.String("VolumeId"),
									Value: aws.String("vol-fffffffffffffffff"),
								},
							},
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		typ := collectorTypes["ebs"]
		index := NewResourceIndexFromTagMapping(&c.resources, id)
		zipped := c.collector.makeQueries(index, typ.Namespace, defaultMetricDimension(typ.Dimension, typ.ResourcePrefix))
		// we have to sort zipped as the order is not guaranteed
		sort.Slice(zipped, func(x, y int) bool {
			return *zipped[x].Id < *zipped[y].Id
		})

		assert.Equal(t, zipped, c.expected, c.message)
	}
}

func TestGetMetricDataInput(t *testing.T) {
	offset := 300
	interval := 300
	period := 300
	ttime := &testTime{}
	ttime.Now()
	endTime := ttime.Now().UTC().Add(time.Duration(-offset) * time.Second)
	startTime := endTime.Add(time.Duration(-interval) * time.Second)

	cases := []struct {
		message   string
		collector *BaseCollector
		resources []*tagging.ResourceTagMapping
		expected  []*cloudwatch.GetMetricDataInput
	}{
		{
			collector: stripInterface(CollectorFromConfig(CollectorConfig{Type: "ebs"})).withTime(ttime),
			resources: []*tagging.ResourceTagMapping{},
			expected:  []*cloudwatch.GetMetricDataInput{},
			message:   "Empty index should produce empty metric data input",
		},
		{
			collector: stripInterface(CollectorFromConfig(CollectorConfig{
				Type:        "ebs",
				MetricStats: []MetricStat{},
			})).withTime(ttime),
			resources: []*tagging.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:ec2:us-east-1:000000000000:volume/vol-fffffffffffffffff"),
				},
				{
					ResourceARN: aws.String("arn:aws:ec2:us-east-1:000000000000:volume/vol-00000000000000000"),
				},
			},
			expected: []*cloudwatch.GetMetricDataInput{},
			message:  "Empty metric stats should produce empty metric data input",
		},
		{
			collector: stripInterface(CollectorFromConfig(CollectorConfig{
				Type:     "ebs",
				Interval: interval,
				Offset:   offset,
				Period:   period,
				MetricStats: []MetricStat{
					{
						MetricName: "MyMetricName",
						Stat:       "Sum",
					},
					{
						MetricName: "MyOtherMetricName",
						Stat:       "Average",
					},
				},
			})).withTime(ttime),
			resources: []*tagging.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:ec2:us-east-1:000000000000:volume/vol-fffffffffffffffff"),
				},
				{
					ResourceARN: aws.String("arn:aws:ec2:us-east-1:000000000000:volume/vol-00000000000000000"),
				},
			},
			expected: []*cloudwatch.GetMetricDataInput{
				{
					EndTime:   &endTime,
					StartTime: &startTime,
					ScanBy:    &TimestampAscending,
					MetricDataQueries: []*cloudwatch.MetricDataQuery{
						{
							Id: aws.String("id_43c1360ea31ff82de65453d44cabeb5307b8a1f5_0"),
							MetricStat: &cloudwatch.MetricStat{
								Metric: &cloudwatch.Metric{
									Dimensions: []*cloudwatch.Dimension{
										{
											Name:  aws.String("VolumeId"),
											Value: aws.String("vol-00000000000000000"),
										},
									},
									MetricName: aws.String("MyMetricName"),
									Namespace:  aws.String("AWS/EBS"),
								},
								Stat:   aws.String("Sum"),
								Period: aws.Int64(int64(period)),
							},
						},
						{
							Id: aws.String("id_43c1360ea31ff82de65453d44cabeb5307b8a1f5_1"),
							MetricStat: &cloudwatch.MetricStat{
								Metric: &cloudwatch.Metric{
									Dimensions: []*cloudwatch.Dimension{
										{
											Name:  aws.String("VolumeId"),
											Value: aws.String("vol-00000000000000000"),
										},
									},
									MetricName: aws.String("MyOtherMetricName"),
									Namespace:  aws.String("AWS/EBS"),
								},
								Stat:   aws.String("Average"),
								Period: aws.Int64(int64(period)),
							},
						},
						{
							Id: aws.String("id_d714b664b1f99367e6962cabb2463495ce4aa395_0"),
							MetricStat: &cloudwatch.MetricStat{
								Metric: &cloudwatch.Metric{
									Dimensions: []*cloudwatch.Dimension{
										{
											Name:  aws.String("VolumeId"),
											Value: aws.String("vol-fffffffffffffffff"),
										},
									},
									MetricName: aws.String("MyMetricName"),
									Namespace:  aws.String("AWS/EBS"),
								},
								Stat:   aws.String("Sum"),
								Period: aws.Int64(int64(period)),
							},
						},
						{
							Id: aws.String("id_d714b664b1f99367e6962cabb2463495ce4aa395_1"),
							MetricStat: &cloudwatch.MetricStat{
								Metric: &cloudwatch.Metric{
									Dimensions: []*cloudwatch.Dimension{
										{
											Name:  aws.String("VolumeId"),
											Value: aws.String("vol-fffffffffffffffff"),
										},
									},
									MetricName: aws.String("MyOtherMetricName"),
									Namespace:  aws.String("AWS/EBS"),
								},
								Stat:   aws.String("Average"),
								Period: aws.Int64(int64(period)),
							},
						},
					},
				},
			},
			message: "Metric data input should be computed correctly.",
		},
	}

	for _, c := range cases {
		index := NewResourceIndexFromTagMapping(&c.resources, id)
		input := c.collector.getMetricDataInput(index, defaultMetricDimension("VolumeId", "volume/"))
		// we have to sort the data queries here as order is not guaranteed
		for i := range input {
			sort.Slice(input[i].MetricDataQueries, func(x, y int) bool {
				return *input[i].MetricDataQueries[x].Id < *input[i].MetricDataQueries[y].Id
			})
		}
		assert.Equal(t, c.expected, input, c.message)
	}
}

// stripInterface is used for easier access to internal data during testing
func stripInterface(i MetricCollector, e error) *BaseCollector {
	if c, ok := i.(*BaseCollector); ok {
		return c
	}

	return nil
}
