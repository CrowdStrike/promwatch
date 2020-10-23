// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/stretchr/testify/assert"
)

func TestFilter(t *testing.T) {
	cases := []struct {
		groups     []*autoscaling.Group
		tagfilters []TagFilter
		expected   []*autoscaling.Group
		message    string
	}{
		{
			groups: []*autoscaling.Group{
				{
					Tags: []*autoscaling.TagDescription{
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
					Tags: []*autoscaling.TagDescription{
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
			expected: []*autoscaling.Group{
				{
					Tags: []*autoscaling.TagDescription{
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
					Tags: []*autoscaling.TagDescription{
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
			groups: []*autoscaling.Group{
				{
					Tags: []*autoscaling.TagDescription{
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
					Tags: []*autoscaling.TagDescription{
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
			expected: []*autoscaling.Group{
				{
					Tags: []*autoscaling.TagDescription{
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
			groups: []*autoscaling.Group{
				{
					Tags: []*autoscaling.TagDescription{
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
					Tags: []*autoscaling.TagDescription{
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
			expected: []*autoscaling.Group{},
			message:  "No match should return empty result",
		},
	}

	for _, c := range cases {
		got := filter(&c.groups, c.tagfilters)
		assert.Equal(t, &c.expected, got, c.message)
	}
}
