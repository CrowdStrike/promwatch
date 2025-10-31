// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	taggingTypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/stretchr/testify/assert"
)

func TestCacheNodeMetricDimension(t *testing.T) {
	cases := []struct {
		resource       *taggingTypes.ResourceTagMapping
		expected       []cwTypes.Dimension
		expectedErrors []error
		message        string
	}{
		{
			message: "Resource should return properly formatted metric dimensions",
			resource: &taggingTypes.ResourceTagMapping{
				ResourceARN: aws.String("arn:aws:ec2:us-east-1:000000000000:cluster:my-asg-name:0001"),
			},
			expected: []cwTypes.Dimension{
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
