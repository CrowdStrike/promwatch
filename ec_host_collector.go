// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/elasticache"
	tagging "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
)

type ECHostCollector struct {
	base *BaseCollector
}

type CacheClusterWithTags struct {
	elasticache.CacheCluster
	Tags []*tagging.Tag
}

func NewCacheClusterWithTags(c elasticache.CacheCluster, t []*tagging.Tag) *CacheClusterWithTags {
	return &CacheClusterWithTags{
		CacheCluster: c,
		Tags:         t,
	}
}

func NewECHostCollector(c CollectorConfig) (MetricCollector, error) {
	b := &BaseCollector{
		config:         c,
		resourceName:   "elasticache:cluster",
		namespace:      "AWS/ElastiCache",
		dimension:      "CacheClusterId",
		resourcePrefix: "cluster:",
	}

	return &ECHostCollector{
		base: b,
	}, nil
}

func (a *ECHostCollector) Valid() bool {
	return a.base.Valid()
}

func (a *ECHostCollector) getClusters() (*ResourceIndex, error) {
	resources, err := a.base.getResources()
	if err != nil {
		return nil, err
	}
	resourceMap := make(map[string][]*tagging.Tag, len(resources.Resources))

	for _, r := range resources.Resources {
		resourceMap[*r.ResourceARN] = r.Tags
	}

	client, err := DefaultAWSClient(a.base.config.Region)
	if err != nil {
		return nil, err
	}

	res, err := client.DescribeCacheClusters(&elasticache.DescribeCacheClustersInput{
		ShowCacheClustersNotInReplicationGroups: aws.Bool(true),
		ShowCacheNodeInfo:                       aws.Bool(true),
	}, a.base.Telemetry())
	if err != nil {
		return nil, err
	}

	cacheClusters := []*CacheClusterWithTags{}
	for _, c := range *res {
		// Only memcached has host level metrics
		if *c.Engine != "memcached" {
			continue
		}

		rt, ok := resourceMap[*c.ARN]
		if !ok {
			continue
		}
		cluster := NewCacheClusterWithTags(*c, rt)
		cacheClusters = append(cacheClusters, cluster)
	}

	// convert cache clusters to resource tag mapping
	mapping := []*tagging.ResourceTagMapping{}
	for _, cluster := range cacheClusters {
		for _, n := range cluster.CacheNodes {
			// append node id to the cluster name so it looks similar to a redis cluster id
			arnWithNodeID := fmt.Sprintf("%s:%s", *cluster.ARN, *n.CacheNodeId)
			mapping = append(mapping, &tagging.ResourceTagMapping{
				ResourceARN: &arnWithNodeID,
				Tags:        cluster.Tags,
			})
			Logger.Debugf("Cache ARN: %s", aws.StringValue(cluster.ARN))
		}
	}

	return NewResourceIndexFromTagMapping(&mapping, id), nil
}

func (a *ECHostCollector) Run() *CollectorProc {
	return a.base.run(a.getClusters, cacheNodeMetricDimension)
}

func cacheNodeMetricDimension(resource *tagging.ResourceTagMapping) ([]*cloudwatch.Dimension, error) {
	arn, err := arn.Parse(*resource.ResourceARN)
	if err != nil {
		return []*cloudwatch.Dimension{}, ErrCanNotParseARN
	}

	// Resources e.g.: cluster:my-asg-name:0001
	// to cluster: my-cluster-name, node: 0001

	val := strings.Split(arn.Resource, ":")
	cluster := val[1]
	node := val[2]

	return []*cloudwatch.Dimension{
		{Name: aws.String("CacheClusterId"), Value: aws.String(cluster)},
		{Name: aws.String("CacheNodeId"), Value: aws.String(node)},
	}, nil
}
