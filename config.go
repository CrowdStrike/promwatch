// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"os"

	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
)

const (
	DefaultListen = "localhost:11999"

	LogError = "error"
	LogWarn  = "warn"
	LogInfo  = "info"
	LogDebug = "debug"
)

// levels allows to resolve a string value like "debug" to a zap Level which are
// represented by int8.
type levels map[string]zapcore.Level

func (l levels) Get(s string) zapcore.Level {
	if lvl, ok := l[s]; ok {
		return lvl
	}

	return zapcore.InfoLevel
}

// Levels maps string constants representing log levels to zap log levels.
var Levels = levels{
	LogError: zapcore.ErrorLevel,
	LogWarn:  zapcore.WarnLevel,
	LogInfo:  zapcore.InfoLevel,
	LogDebug: zapcore.DebugLevel,
}

// PromWatchConfig holds definitions of the collectors.
type PromWatchConfig struct {
	Listen     string            `yaml:"listen"`
	LogLevel   string            `yaml:"log_level"`
	Collectors []MetricCollector `yaml:"collectors"`
}

// CollectorConfig is the configuration of a specific collector as defined in
// the YAML configuration.
type CollectorConfig struct {
	Offset   int    `yaml:"offset"`
	Interval int    `yaml:"interval"`
	Period   int    `yaml:"period"`
	Region   string `yaml:"region"`
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`

	TagFilters  []TagFilter  `yaml:"tag_filters"`
	MetricStats []MetricStat `yaml:"metric_stats"`
	MergeTags   []string     `yaml:"merge_tags"`
}

// UnmarshalYAML implements the Unmarshaller interface for PromWatchConfig to
// unmarshal the different collectors while still maintaining the interface type
// for the list of collectors.
func (c *PromWatchConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type tmp struct {
		Listen     string
		LogLevel   string `yaml:"log_level"`
		Collectors []CollectorConfig
	}
	var t tmp
	if err := unmarshal(&t); err != nil {
		return err
	}

	// quick and easy and given the config is loaded only once on
	// service startup the performance impact is negligible
	for _, v := range t.Collectors {
		collector, err := CollectorFromConfig(v)
		if err != nil {
			return err
		}
		// should never happen without also producing an err that is non-nil above
		if collector == nil {
			continue
		}
		c.Collectors = append(c.Collectors, collector)
	}

	// Ensure defaults, this should be factored out into c.ensureDefaults() in
	// case more get added
	if t.Listen == "" {
		c.Listen = DefaultListen
	} else {
		c.Listen = t.Listen
	}

	if t.LogLevel == "" {
		c.LogLevel = LogInfo
	} else {
		c.LogLevel = t.LogLevel
	}

	return nil
}

func loadConfig(config string) (*PromWatchConfig, error) {
	parsed := PromWatchConfig{}
	content, err := os.ReadFile(config)
	if err != nil {
		return &parsed, err
	}

	err = yaml.Unmarshal(content, &parsed)
	return &parsed, err
}
