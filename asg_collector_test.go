// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	autoscalingTypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/stretchr/testify/assert"
)

func TestFilter(t *testing.T) {
	cases := []struct {
		groups     []autoscalingTypes.AutoScalingGroup
		tagfilters []TagFilter
		expected   []autoscalingTypes.AutoScalingGroup
		message    string
	}{
		{
			groups: []autoscalingTypes.AutoScalingGroup{
				{
					Tags: []autoscalingTypes.TagDescription{
						{
							Key:   aws.String("testTag"),
							Value: aws.String("testValue"),
						},
						{
							Key:   aws.String("more"),
							Value: aws.String("tags"),
						},
					},
				},
				{
					Tags: []autoscalingTypes.TagDescription{
						{
							Key:   aws.String("someTag"),
							Value: aws.String("someValue"),
						},
						{
							Key:   aws.String("someMore"),
							Value: aws.String("tags"),
						},
					},
				},
			},
			tagfilters: []TagFilter{},
			expected: []autoscalingTypes.AutoScalingGroup{
				{
					Tags: []autoscalingTypes.TagDescription{
						{
							Key:   aws.String("testTag"),
							Value: aws.String("testValue"),
						},
						{
							Key:   aws.String("more"),
							Value: aws.String("tags"),
						},
					},
				},
				{
					Tags: []autoscalingTypes.TagDescription{
						{
							Key:   aws.String("someTag"),
							Value: aws.String("someValue"),
						},
						{
							Key:   aws.String("someMore"),
							Value: aws.String("tags"),
						},
					},
				},
			},
			message: "Empty tag filters should yield all groups",
		},
		{
			groups: []autoscalingTypes.AutoScalingGroup{
				{
					Tags: []autoscalingTypes.TagDescription{
						{
							Key:   aws.String("testTag"),
							Value: aws.String("testValue"),
						},
						{
							Key:   aws.String("more"),
							Value: aws.String("tags"),
						},
					},
				},
				{
					Tags: []autoscalingTypes.TagDescription{
						{
							Key:   aws.String("someTag"),
							Value: aws.String("someValue"),
						},
						{
							Key:   aws.String("someMore"),
							Value: aws.String("tags"),
						},
					},
				},
			},
			tagfilters: []TagFilter{
				{Key: "someTag", Value: "someValue"},
			},
			expected: []autoscalingTypes.AutoScalingGroup{
				{
					Tags: []autoscalingTypes.TagDescription{
						{
							Key:   aws.String("someTag"),
							Value: aws.String("someValue"),
						},
						{
							Key:   aws.String("someMore"),
							Value: aws.String("tags"),
						},
					},
				},
			},
			message: "Filter should only return groups matching tags",
		},
		{
			groups: []autoscalingTypes.AutoScalingGroup{
				{
					Tags: []autoscalingTypes.TagDescription{
						{
							Key:   aws.String("testTag"),
							Value: aws.String("testValue"),
						},
						{
							Key:   aws.String("more"),
							Value: aws.String("tags"),
						},
					},
				},
				{
					Tags: []autoscalingTypes.TagDescription{
						{
							Key:   aws.String("someTag"),
							Value: aws.String("someValue"),
						},
						{
							Key:   aws.String("someMore"),
							Value: aws.String("tags"),
						},
					},
				},
			},
			tagfilters: []TagFilter{
				{Key: "no", Value: "match"},
			},
			expected: []autoscalingTypes.AutoScalingGroup{},
			message:  "No match should return empty result",
		},
	}

	for _, c := range cases {
		got := filter(&c.groups, c.tagfilters)
		assert.Equal(t, c.expected, got, c.message)
	}
}
