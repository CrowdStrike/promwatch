// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	tagging "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
)

type ASGCollector struct {
	base *BaseCollector
}

func NewASGCollector(c CollectorConfig) (MetricCollector, error) {
	b := &BaseCollector{
		config:    c,
		namespace: "AWS/AutoScaling",
		dimension: "AutoScalingGroupName",
	}

	return &ASGCollector{
		base: b,
	}, nil
}

func (a *ASGCollector) Valid() bool {
	return a.base.Valid()
}

func (a *ASGCollector) getGroups() (*ResourceIndex, error) {
	client, err := DefaultAWSClient(a.base.config.Region)
	if err != nil {
		return nil, err
	}
	res, err := client.DescribeAutoScalingGroups(&autoscaling.DescribeAutoScalingGroupsInput{}, a.base.Telemetry())
	if err != nil {
		return nil, err
	}

	// convert autoscaling groups to resource tag mapping
	mapping := []*tagging.ResourceTagMapping{}
	for _, group := range *filter(res, a.base.config.TagFilters) {
		tags := []*tagging.Tag{}
		for _, tag := range group.Tags {
			tags = append(tags, &tagging.Tag{Key: tag.Key, Value: tag.Value})
		}

		mapping = append(mapping, &tagging.ResourceTagMapping{
			ResourceARN: group.AutoScalingGroupARN,
			Tags:        tags,
		})
		Logger.Debugf("ASG ARN: %s", aws.StringValue(group.AutoScalingGroupARN))
	}

	return NewResourceIndexFromTagMapping(&mapping, id), nil
}

func filter(groups *[]*autoscaling.Group, tf []TagFilter) *[]*autoscaling.Group {
	res := []*autoscaling.Group{}

outer:
	for _, g := range *groups {
		// continue if the group has less tags than we have filters as it can
		// not match in that case
		if len(g.Tags) >= len(tf) {
			// make key value pairs of group tags for easier checking
			tagMap := map[string]string{}
			for _, g := range g.Tags {
				tagMap[*g.Key] = *g.Value
			}

			// check all filter tags for matches and continue if matching fails
			for _, filterTag := range tf {
				v, ok := tagMap[filterTag.Key]
				// Key not found, no match, go to next group
				if !ok {
					continue outer
				}

				// Value does not match, go to next group
				if v != filterTag.Value {
					continue outer
				}
			}

			// all filter tags match if reach this code, keep group as it
			// matches all filter tags
			res = append(res, g)
		}
	}

	return &res
}

func (a *ASGCollector) Run() *CollectorProc {
	return a.base.run(a.getGroups, asgMetricDimension)
}

// asgMetricDimension sets the name of the autoscaling group as dimension for CloudWatch.
func asgMetricDimension(resource *tagging.ResourceTagMapping) ([]*cloudwatch.Dimension, error) {
	arn, err := arn.Parse(*resource.ResourceARN)
	if err != nil {
		return []*cloudwatch.Dimension{}, ErrCanNotParseARN
	}

	// Resources e.g.: autoScalingGroup:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:autoScalingGroupName/my-asg-name
	// to: my-asg-name
	val := arn.Resource[75:]

	return []*cloudwatch.Dimension{{Name: aws.String("AutoScalingGroupName"), Value: aws.String(val)}}, nil
}
