// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	tagging "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/stretchr/testify/assert"
)

func TestCacheNodeMetricDimension(t *testing.T) {
	cases := []struct {
		resource       *tagging.ResourceTagMapping
		expected       []*cloudwatch.Dimension
		expectedErrors []error
		message        string
	}{
		{
			message: "Resource should return properly formatted metric dimensions",
			resource: &tagging.ResourceTagMapping{
				ResourceARN: aws.String("arn:aws:ec2:us-east-1:000000000000:cluster:my-asg-name:0001"),
			},
			expected: []*cloudwatch.Dimension{
				{
					Name:  aws.String("CacheClusterId"),
					Value: aws.String("my-asg-name"),
				},
				{
					Name:  aws.String("CacheNodeId"),
					Value: aws.String("0001"),
				},
			},
		},
	}

	for _, c := range cases {
		got, _ := cacheNodeMetricDimension(c.resource)
		assert.Equal(t, c.expected, got, c.message)
	}
}
