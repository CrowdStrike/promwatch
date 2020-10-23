// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestConfigUnmarshalling(t *testing.T) {
	ebsC, _ := CollectorFromConfig(CollectorConfig{
		Type:     "ebs",
		Name:     "test collector",
		Offset:   600,
		Interval: 300,
		Period:   300,
		TagFilters: []TagFilter{
			{
				Key:   "testkey",
				Value: "testvalue",
			},
			{
				Key:   "more",
				Value: "tests",
			},
		},
		MetricStats: []MetricStat{
			{
				MetricName: "VolumeReadBytes",
				Stat:       "Average",
			},
			{
				MetricName: "VolumeReadBytes",
				Stat:       "Sum",
			},
		},
	})

	cases := []struct {
		str      []byte
		expected PromWatchConfig
		message  string
	}{
		{[]byte(`
listen: localhost:11999
log_level: debug
collectors:
- type: ebs
  name: test collector
  offset: 600
  interval: 300
  period: 300
  tag_filters:
  - key: testkey
    value: testvalue
  - key: more
    value: tests
  metric_stats:
  - name: VolumeReadBytes
    stat: Average
  - name: VolumeReadBytes
    stat: Sum `),
			PromWatchConfig{
				Listen:     "localhost:11999",
				LogLevel:   LogDebug,
				Collectors: []MetricCollector{ebsC},
			},
			"EBS config should parse correctly"},
		{[]byte("collectors:"),
			PromWatchConfig{
				Listen:   "localhost:11999",
				LogLevel: LogInfo},
			"Default values should be set"},
	}

	for _, c := range cases {
		var got PromWatchConfig
		err := yaml.Unmarshal(c.str, &got)
		assert.Nil(t, err, c.message)
		assert.Equal(t, c.expected, got, c.message)
	}
}
