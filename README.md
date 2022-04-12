# PromWatch

PromWatch is an exporter for CloudWatch metrics in a Prometheus compatible
format.

## Build And Run

Check out this repository:

    git clone https://github.com/crowdstrike/promwatch

Build the `promwatch` binary with make:

    make

Run:

    ./promwatch -config <config-file>

By default `promwatch` starts binds to `localhost:11999` and provides metrics
via `http://localhost:11999/metrics`.

## Configuration

PromWatch is configured using a YAML configuration file.

### Terminology

**Collector**:

A collector is a logical process created from a collector configuration that
collects metrics of a specific AWS service. Each collector has its own set of
configuration options and different collectors can collect metrics of the same
service but using different offset, interval, or period, matching different tags
or carrying over different labels.

Each collector provides metrics to monitor its health and performance.

Currently implemented collector types are:

- alb
- asg
- ebs
- ec
- ec_host (Elasticache Host-level)
- elb
- neptune
- nlb
- rds
- sqs

**Offset**:

The offset specifies the duration substracted from the current time that
determines the most recent data point returned. For example choosing an offset
of 10 minutes means the most recent datapoint returned will be at least 10
minutes old.

Most cloudwatch data gets provided by an offset so asking for a short period of
data with a small or no offset might lead to no data being provided.

**Interval**:

The interval is the duration each collector waits before collecting data from
CloudWatch again.

**Period**:

The period determines the time span a collector will request data for from
CloudWatch.

**Tag Filters**

Tag filters are key value pairs that define the resources metrics are collected
for by matching their AWS tag keys and tag values. Using different tag filters
on collectors of the same collector type allows to shard collection in large
environments.

**Metric Stats**

Metric stats are pairs of a metric and a statistic to collect for that metric
from CloudWatch. Metrics and statistics are [CloudWatch
concepts](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/cloudwatch_concepts.html)
and are passed through to CloudWatch as they are provided.

**Merge Tags**

PromWatch allows to carry over AWS tags as Prometheus labels. The keys defined
as merge tags will be converted to Prometheus label keys (snake case, special
characters replaced with underscores), the values will be used as label values
as they are.

### Formal Configuration Specification

Generic placeholders:

- `<loglevel>`: one of `"info"` and `"debug"` determining the detail of log
                messages
- `<int>`: an integer value
- `<string>`: a regular string
- `<aws_region>`: a valid [AWS region](https://docs.aws.amazon.com/general/latest/gr/rande.html#regional-endpoints)
- `<collector_type>`: a valid collector type as listed above

Top level:

``` yaml
log_level: <loglevel | default = "info">
collectors: [ <collector> ] | default = []
```

`<collector>`:

``` yaml
type: <collector_type>
name: <string>
offset: <int>
interval: <int>
period: <int>
region: <aws_region>
merge_tags: [<string>] | default = []
tag_filters: [ <tag_filter> ] | default = []
metric_stats: [ <metric_stat> ] | default = []
```

`<tag_filter>`:

``` yaml
key: <string>
value: <string>
```

`<metric_stat>`:

``` yaml
name: <string>
stat: <string>
```

### AWS Permissions

For PromWatch to be able to collect metrics from CloudWatch the user or instance
running PromWatch has to have IAM permissions that partially depend on the
collectors used.

All collectors have to be granted the `cloudwatch:GetMetricData` permission.

The collectors for services supported by the ResourceGroupsTaggingAPI have to be
granted the `tag:GetResources` permission. Those services are:

- alb
- ebs
- ec
- elb
- neptune
- nlb
- rds

To collect ASG metrics from CloudWatch the
`autoscaling.DescribeAutoScalingGroups` permission is required.

To collect Host-level Elasticache metrics from CloudWatch the
`elasticache:DescribeCacheClusters` permission is required.

An example policy document to collect all supported metrics might look like
this:

``` json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "AllowPromWatchMetricCollection",
            "Effect": "Allow",
            "Action": [
                "cloudwatch:GetMetricData",
                "tag:GetResources",
                "autoscaling:DescribeAutoScalingGroups",
                "elasticache:DescribeCacheClusters"
            ],
            "Resource": "*"
        }
    ]
}
```

## Metrics

PromWatch exposes global and collector level metrics.

### Global

| | |
|-|-|
|promwatch_build_info | A vector containing `version`, `githash`, and the build date as `date` |

### Collector

| | |
|-|-|
|promwatch_collector_errors_total                                          | Total count of errors in metrics collectors                                          |
|promwatch_collector_runs_total                                            | Total count of collector runs                                                        |
|promwatch_collector_run_duration_seconds                                  | Total count of collector runs                                                        |
|promwatch_collector_matching_resources                                    | Number of resources matching the collector's tag filters                             |
|promwatch_collector_rescourcegroupstaggingapi_getresources_requests_total | Total number of resource requests issued against the AWS Resource Groups Tagging API |
|promwatch_collector_cloudwatch_getmetricdata_requests_total               | Total number of requests issued against the AWS CloudWatch GetMetricData endpoint    |
|promwatch_collector_autoscaling_describeautoscalinggroups_requests_total  | Total number of requests issued against the AWS EC2 autoscaling endpoint.            |
|promwatch_collector_elasticache_describecacheclusters_requests_total      | Total number of requests issued against the AWS Elasticache endpoint.                |
