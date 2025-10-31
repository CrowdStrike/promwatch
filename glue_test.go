// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	taggingTypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/stretchr/testify/assert"
)

func TestToSnakeCase(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"already_snake", "already_snake"},
		{"A", "a"},
		{"AA", "aa"},
		{"AaAa", "aa_aa"},
		{"HTTPRequest", "http_request"},
		{"BatteryLifeValue", "battery_life_value"},
		{"Id0Value", "id0_value"},
		{"ID0Value", "id0_value"},
		{"BIGBlob_ofSTUFF", "big_blob_of_stuff"},
	}
	for _, c := range cases {
		got := toSnakeCase(c.input)
		assert.Equal(t, c.expected, got)
	}
}

func TestSanitize(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"already_sane", "already_sane"},
		{" ,.:-=/", "_______"},
		{"balance%_average", "balance_pct_average"},
	}
	for _, c := range cases {
		got := sanitize(c.input)
		assert.Equal(t, c.expected, got)
	}
}

func TestNewResourceIndexFromTagMapping(t *testing.T) {
	testARN := "aws:arn:test"
	resources := []taggingTypes.ResourceTagMapping{
		{
			ResourceARN: aws.String(testARN),
		},
	}
	index := NewResourceIndexFromTagMapping(&resources, func(m *taggingTypes.ResourceTagMapping) string {
		return *m.ResourceARN
	})

	assert.Equal(t, 1, len(index.Resources))
	_, ok := index.Resources[testARN]
	assert.True(t, ok)
}

func TestConvertTags(t *testing.T) {
	cases := []struct {
		resource  *taggingTypes.ResourceTagMapping
		mergeTags []string
		extraTags []taggingTypes.Tag
		expected  string
		message   string
	}{
		{
			resource: &taggingTypes.ResourceTagMapping{Tags: []taggingTypes.Tag{}},
			expected: ``,
			message:  "No tags on the resource should produce the default set of tags",
		},
		{
			resource: &taggingTypes.ResourceTagMapping{
				Tags: []taggingTypes.Tag{
					{
						Key:   aws.String("someTagKey"),
						Value: aws.String("someTagValue"),
					},
					{
						Key:   aws.String("mergeMe"),
						Value: aws.String("someOtherTagValue"),
					},
				},
			},
			mergeTags: []string{
				"someTagKey",
				"mergeMe",
			},
			expected: `some_tag_key="someTagValue",merge_me="someOtherTagValue"`,
			message:  "Tags configured to be merged should be converted",
		},
		{
			resource: &taggingTypes.ResourceTagMapping{
				Tags: []taggingTypes.Tag{
					{
						Key:   aws.String("someTag%Key"),
						Value: aws.String("someTagValue"),
					},
				},
			},
			mergeTags: []string{
				"someTag%Key",
			},
			expected: `some_tag_pct_key="someTagValue"`,
			message:  "Tags containing % should be represented correctly",
		},
		{
			resource: &taggingTypes.ResourceTagMapping{
				Tags: []taggingTypes.Tag{
					{
						Key:   aws.String("someTagKey"),
						Value: aws.String(`"someTag\"Value"`),
					},
				},
			},
			mergeTags: []string{
				"someTagKey",
			},
			expected: `some_tag_key="\"someTag\\\"Value\""`,
			message:  "Tag values should be escaped",
		},
		{
			resource: &taggingTypes.ResourceTagMapping{
				Tags: []taggingTypes.Tag{
					{
						Key:   aws.String("someTagKey"),
						Value: aws.String(`“insane"`),
					},
				},
			},
			mergeTags: []string{
				"someTagKey",
			},
			expected: `some_tag_key="“insane\""`,
			message:  "Tag values should be escaped",
		},
		{
			resource: &taggingTypes.ResourceTagMapping{
				Tags: []taggingTypes.Tag{
					{
						Key:   aws.String("someTagKey"),
						Value: aws.String("someTagValue"),
					},
					{
						Key:   aws.String("notMe"),
						Value: aws.String("nope"),
					},
					{
						Key:   aws.String("mergeMe"),
						Value: aws.String("someOtherTagValue"),
					},
				},
			},
			mergeTags: []string{
				"someTagKey",
				"mergeMe",
			},
			expected: `some_tag_key="someTagValue",merge_me="someOtherTagValue"`,
			message:  "Only tags configured to be merged should be converted",
		},
		{
			resource: &taggingTypes.ResourceTagMapping{
				Tags: []taggingTypes.Tag{
					{
						Key:   aws.String("someTagKey"),
						Value: aws.String("someTagValue"),
					},
					{
						Key:   aws.String("mergeMe"),
						Value: aws.String("someOtherTagValue"),
					},
				},
			},
			mergeTags: []string{
				"someTagKey",
				"mergeMe",
			},
			extraTags: []taggingTypes.Tag{
				{
					Key:   aws.String("extra"),
					Value: aws.String("tagValue"),
				},
				{
					Key:   aws.String("moreExtra"),
					Value: aws.String("anotherExtraValue"),
				},
			},
			expected: `extra="tagValue",more_extra="anotherExtraValue",some_tag_key="someTagValue",merge_me="someOtherTagValue"`,
			message:  "Only tags configured to be merged should be converted",
		},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, convertTags(c.resource, c.mergeTags, c.extraTags...), c.message)
	}
}

func TestExtraTagsCallback(t *testing.T) {
	cases := []struct {
		resource      *taggingTypes.ResourceTagMapping
		expected      []taggingTypes.Tag
		expectedError error
		message       string
	}{
		{
			resource: &taggingTypes.ResourceTagMapping{ResourceARN: aws.String("invalid")},
			expected: []taggingTypes.Tag{
				{
					Key:   aws.String("arn"),
					Value: aws.String("invalid"),
				},
			},
			expectedError: ErrCanNotParseARN,
			message:       "An invalid ARN should result in an error",
		},
		{
			resource: &taggingTypes.ResourceTagMapping{
				ResourceARN: aws.String("arn:aws:ec2:us-east-1:00000000000:volume/vol-0000000000000000"),
			},
			expected: []taggingTypes.Tag{
				{
					Key:   aws.String("arn"),
					Value: aws.String("arn:aws:ec2:us-east-1:00000000000:volume/vol-0000000000000000"),
				},
				{
					Key:   aws.String("VolumeId"),
					Value: aws.String("vol-0000000000000000"),
				},
			},
			expectedError: nil,
			message:       "An invalid ARN should result in an error",
		},
	}

	for _, c := range cases {
		got, err := defaultExtraTags("VolumeId", "volume/")(c.resource)
		assert.Equal(t, c.expectedError, err, c.message)
		assert.Equal(t, c.expected, got, c.message)
	}
}

func TestCollectorFromConfig(t *testing.T) {
	cases := []struct {
		config   *CollectorConfig
		expected MetricCollector
		message  string
	}{
		{
			config:   &CollectorConfig{},
			expected: nil,
			message:  "Empty config should produce nil",
		},
		{
			config:   &CollectorConfig{Type: "not such type"},
			expected: nil,
			message:  "Unknown type should produce nil",
		},
		{
			config: &CollectorConfig{Type: "ebs"},
			expected: &BaseCollector{
				config:         CollectorConfig{Type: "ebs"},
				resourceName:   "ec2:volume",
				namespace:      "AWS/EBS",
				dimension:      "VolumeId",
				resourcePrefix: "volume/",
			},
			message: "Known type should produce collector",
		},
	}

	for _, c := range cases {
		got, _ := CollectorFromConfig(*c.config)
		assert.Equal(t, c.expected, got, c.message)
	}
}
