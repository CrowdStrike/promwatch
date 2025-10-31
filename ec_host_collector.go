// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	ecTypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	taggingTypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

type ECHostCollector struct {
	base *BaseCollector
}

type CacheClusterWithTags struct {
	ecTypes.CacheCluster
	Tags []taggingTypes.Tag
}

func NewCacheClusterWithTags(c ecTypes.CacheCluster, t []taggingTypes.Tag) *CacheClusterWithTags {
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
	resourceMap := make(map[string][]taggingTypes.Tag, len(resources.Resources))

	for _, r := range resources.Resources {
		resourceMap[*r.ResourceARN] = r.Tags
	}

	client, err := DefaultAWSClient(a.base.config.Region)
	if err != nil {
		return nil, err
	}

	res, err := client.DescribeCacheClusters(context.TODO(), &elasticache.DescribeCacheClustersInput{
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
		cluster := NewCacheClusterWithTags(c, rt)
		cacheClusters = append(cacheClusters, cluster)
	}

	// convert cache clusters to resource tag mapping
	mapping := []taggingTypes.ResourceTagMapping{}
	for _, cluster := range cacheClusters {
		for _, n := range cluster.CacheNodes {
			// append node id to the cluster name so it looks similar to a redis cluster id
			arnWithNodeID := fmt.Sprintf("%s:%s", *cluster.ARN, *n.CacheNodeId)
			mapping = append(mapping, taggingTypes.ResourceTagMapping{
				ResourceARN: &arnWithNodeID,
				Tags:        cluster.Tags,
			})
			Logger.Debugf("Cache ARN: %s", aws.ToString(cluster.ARN))
		}
	}

	return NewResourceIndexFromTagMapping(&mapping, id), nil
}

func (a *ECHostCollector) Run() *CollectorProc {
	return a.base.run(a.getClusters, cacheNodeMetricDimension)
}

func cacheNodeMetricDimension(resource *taggingTypes.ResourceTagMapping) ([]cwTypes.Dimension, error) {
	arnParsed, err := arn.Parse(*resource.ResourceARN)
	if err != nil {
		return []cwTypes.Dimension{}, ErrCanNotParseARN
	}

	// Resources e.g.: cluster:my-asg-name:0001
	// to cluster: my-cluster-name, node: 0001

	val := strings.Split(arnParsed.Resource, ":")
	cluster := val[1]
	node := val[2]

	return []cwTypes.Dimension{
		{Name: aws.String("CacheClusterId"), Value: aws.String(cluster)},
		{Name: aws.String("CacheNodeId"), Value: aws.String(node)},
	}, nil
}
