// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	autoscalingTypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	ecTypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	tagging "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingTypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

const MaxMetricDataQueryItems = 500

// Client implements the set of AWS service methods used in the collectors. We
// use a small subset of what the AWS SDK provides across a multitude of
// service packages, this interface helps us to easily keep track of that usage
// and implement testing clients.
type Client interface {
	DescribeAutoScalingGroups(context.Context, *autoscaling.DescribeAutoScalingGroupsInput, *CollectorTelemetry) (*[]autoscalingTypes.AutoScalingGroup, error)
	DescribeCacheClusters(context.Context, *elasticache.DescribeCacheClustersInput, *CollectorTelemetry) (*[]ecTypes.CacheCluster, error)
	GetResources(context.Context, *tagging.GetResourcesInput, *CollectorTelemetry) (*[]taggingTypes.ResourceTagMapping, error)
	GetMetricData(context.Context, []*cloudwatch.GetMetricDataInput, *CollectorTelemetry) (*[]cwTypes.MetricDataResult, error)
}

// AWSClient implements the Client interface and provides the AWS requests we
// use throughout the project.
type AWSClient struct {
	Region      string
	MaxRetries  int
	cfg         aws.Config
	tagging     *tagging.Client
	cloudwatch  *cloudwatch.Client
	autoscaling *autoscaling.Client
	elasticache *elasticache.Client
}

func defaultConfig(region string) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithRetryMaxAttempts(5),
		config.WithRetryMode(aws.RetryModeStandard),
	)
	return cfg, err
}

// DefaultAWSClient returns a default AWSClient for the provided region with max
// retries set to 5 and all other values being set as in a stock aws.Config.
func DefaultAWSClient(region string) (Client, error) {
	cfg, err := defaultConfig(region)
	if err != nil {
		return nil, err
	}

	return &AWSClient{
		Region: region,
		cfg:    cfg,
	}, nil
}

func (client *AWSClient) getTaggingAPI() *tagging.Client {
	if client.tagging != nil {
		return client.tagging
	}

	client.tagging = tagging.NewFromConfig(client.cfg)

	return client.tagging
}

func (client *AWSClient) getCloudwatch() *cloudwatch.Client {
	if client.cloudwatch != nil {
		return client.cloudwatch
	}

	client.cloudwatch = cloudwatch.NewFromConfig(client.cfg)

	return client.cloudwatch
}

func (client *AWSClient) getAutoscaling() *autoscaling.Client {
	client.autoscaling = autoscaling.NewFromConfig(client.cfg)

	return client.autoscaling
}

func (client *AWSClient) getElasticache() *elasticache.Client {
	client.elasticache = elasticache.NewFromConfig(client.cfg)

	return client.elasticache
}

// GetResources proxies to
// resourcegroupstaggingapi GetResources paginator and handles
// aggregation of the paged results.
func (client *AWSClient) GetResources(ctx context.Context, input *tagging.GetResourcesInput, tele *CollectorTelemetry) (*[]taggingTypes.ResourceTagMapping, error) {
	res := []taggingTypes.ResourceTagMapping{}
	api := client.getTaggingAPI()

	paginator := tagging.NewGetResourcesPaginator(api, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return &res, err
		}
		tele.GetResourcesCount.Inc()
		res = append(res, page.ResourceTagMappingList...)
	}
	return &res, nil
}

// GetMetricData proxies to cloudwatch.GetMetricData paginator and handles aggregation
// of the paged results. The requests are issued concurrently.
func (client *AWSClient) GetMetricData(ctx context.Context, in []*cloudwatch.GetMetricDataInput, tele *CollectorTelemetry) (*[]cwTypes.MetricDataResult, error) {
	type lock struct {
		sync.Mutex
		r []cwTypes.MetricDataResult
	}
	res := lock{
		r: []cwTypes.MetricDataResult{},
	}
	wg := sync.WaitGroup{}
	for _, input := range in {
		wg.Add(1)
		go func(ip *cloudwatch.GetMetricDataInput) {
			defer wg.Done()
			api := client.getCloudwatch()
			paginator := cloudwatch.NewGetMetricDataPaginator(api, ip)
			for paginator.HasMorePages() {
				page, err := paginator.NextPage(ctx)
				if err != nil {
					Logger.Error("GetMetricData:", err.Error())
					tele.ErrorCount.Inc()
					return
				}
				tele.GetMetricDataCount.Inc()
				res.Lock()
				res.r = append(res.r, page.MetricDataResults...)
				res.Unlock()
			}
		}(input)
	}
	wg.Wait()

	return &res.r, nil
}

func (client *AWSClient) DescribeAutoScalingGroups(ctx context.Context, input *autoscaling.DescribeAutoScalingGroupsInput, tele *CollectorTelemetry) (*[]autoscalingTypes.AutoScalingGroup, error) {
	res := []autoscalingTypes.AutoScalingGroup{}
	api := client.getAutoscaling()

	paginator := autoscaling.NewDescribeAutoScalingGroupsPaginator(api, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			Logger.Error("DescribeAutoScalingGroups:", err.Error())
			tele.ErrorCount.Inc()
			return &res, err
		}
		tele.DescribeAutoScalingGroupsCount.Inc()
		res = append(res, page.AutoScalingGroups...)
	}

	return &res, nil
}

func (client *AWSClient) DescribeCacheClusters(ctx context.Context, input *elasticache.DescribeCacheClustersInput, tele *CollectorTelemetry) (*[]ecTypes.CacheCluster, error) {
	res := []ecTypes.CacheCluster{}
	api := client.getElasticache()

	paginator := elasticache.NewDescribeCacheClustersPaginator(api, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			Logger.Error("DescribeElasticacheCacheClusters:", err.Error())
			tele.ErrorCount.Inc()
			return &res, err
		}
		tele.DescribeElasticacheCacheClustersCount.Inc()
		res = append(res, page.CacheClusters...)
	}

	return &res, nil
}
