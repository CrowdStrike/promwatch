// Copyright 2021 CrowdStrike, Inc.
// PromWatch collects metrics from AWS CloudWatch and presents them for scraping
// by Prometheus.
package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

var registry = prometheus.NewRegistry()

var (
	// PromWatch build information.
	buildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "promwatch_build_info",
		Help: "PromWatch build information.",
	}, []string{"version", "githash", "date"})
)

// InitializeTelemetry registers the global Prometheus metric collectors.
func InitializeTelemetry() {
	// Build info can be registered and set right away, it will not change
	registry.MustRegister(buildInfo)
	buildInfo.WithLabelValues(Version, GitHash, Date).Set(1)
}

// CollectorTelemetry holds the Prometheus metric collectors for each PromWatch
// collector.
type CollectorTelemetry struct {
	ErrorCount                            prometheus.Counter
	RunCount                              prometheus.Counter
	GetResourcesCount                     prometheus.Counter
	GetMetricDataCount                    prometheus.Counter
	DescribeAutoScalingGroupsCount        prometheus.Counter
	DescribeElasticacheCacheClustersCount prometheus.Counter
	RunDuration                           prometheus.Gauge
	MatchingResources                     prometheus.Gauge
}

// NewCollectorTelemetry creates and registers Prometheus metric collectors that
// get used to record per collector metrics.
func NewCollectorTelemetry(labels prometheus.Labels) *CollectorTelemetry {
	tele := &CollectorTelemetry{
		ErrorCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "promwatch_collector_errors_total",
			Help:        "Total count of errors in metrics collectors",
			ConstLabels: labels,
		}),
		RunCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "promwatch_collector_runs_total",
			Help:        "Total count of collector runs.",
			ConstLabels: labels,
		}),
		RunDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "promwatch_collector_run_duration_seconds",
			Help:        "Total count of collector runs.",
			ConstLabels: labels,
		}),
		MatchingResources: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "promwatch_collector_matching_resources",
			Help:        "Number of resources matching the collector's tag filters.",
			ConstLabels: labels,
		}),
		// Counters for AWS API requests. The metric names are following the
		// schema
		// promwatch_<service_sdk_name>_<request_method_name>_requests_total
		GetResourcesCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "promwatch_collector_rescourcegroupstaggingapi_getresources_requests_total",
			Help:        "Total number of resource requests issued against the AWS Resource Groups Tagging API.",
			ConstLabels: labels,
		}),
		GetMetricDataCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "promwatch_collector_cloudwatch_getmetricdata_requests_total",
			Help:        "Total number of requests issued against the AWS CloudWatch GetMetricData endpoint.",
			ConstLabels: labels,
		}),
		DescribeAutoScalingGroupsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "promwatch_collector_autoscaling_describeautoscalinggroups_requests_total",
			Help:        "Total number of requests issued against the AWS EC2 autoscaling endpoint.",
			ConstLabels: labels,
		}),
		DescribeElasticacheCacheClustersCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "promwatch_collector_elasticache_describecacheclusters_requests_total",
			Help:        "Total number of requests issued against the AWS Elasticache endpoint.",
			ConstLabels: labels,
		}),
	}

	registry.MustRegister(tele.ErrorCount)
	registry.MustRegister(tele.RunCount)
	registry.MustRegister(tele.RunDuration)
	registry.MustRegister(tele.MatchingResources)
	registry.MustRegister(tele.GetMetricDataCount)
	registry.MustRegister(tele.GetResourcesCount)
	registry.MustRegister(tele.DescribeAutoScalingGroupsCount)
	registry.MustRegister(tele.DescribeElasticacheCacheClustersCount)

	return tele
}
