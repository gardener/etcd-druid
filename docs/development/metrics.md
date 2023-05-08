# Monitoring

etcd-druid uses [Prometheus][prometheus] for metrics reporting. The metrics can be used for real-time monitoring and debugging of compaction jobs.

The simplest way to see the available metrics is to cURL the metrics endpoint `/metrics`. The format is described [here](http://prometheus.io/docs/instrumenting/exposition_formats/).

Follow the [Prometheus getting started doc][prometheus-getting-started] to spin up a Prometheus server to collect etcd metrics.

The naming of metrics follows the suggested [Prometheus best practices][prometheus-naming]. All compaction related metrics are put under namespace `etcddruid` and subsystem `compaction`.

### Compaction

These metrics give an idea about the compaction jobs that run after some interval in shoot control planes. Studying the metrices, we can deduce how many compaction job ran successfully, how many failed, how many delta events compacted etc.

| Name | Description | Type |
|------|-------------|------|
| etcddruid_compaction_jobs_total | Total number of compaction jobs initiated by compaction controller. | Counter |
| etcddruid_compaction_jobs_current | Number of currently running comapction job. | Gauge |
| etcddruid_compaction_job_duration_seconds | Total time taken in seconds to finish running a compaction job. | Gauge |
| etcddruid_compaction_num_delta_events | Total number of etcd events to be compacted by a compaction job. | Gauge |

There are two labels for `etcddruid_compaction_jobs_total` metrics. The label `succeeded` shows how many of the compaction jobs are succeeded and label `failed` shows how many of compaction jobs are failed.

There are two labels for `etcddruid_compaction_job_duration_seconds` metrics. The label `succeeded` shows how much time taken by a successful job to complete and label `failed` shows how much time taken by a failed compaction job.

`etcddruid_compaction_jobs_current` metric comes with label `etcd_namespace` that indicates the namespace of the ETCD running in the control plane of a shoot cluster..


## Prometheus supplied metrics

The Prometheus client library provides a number of metrics under the `go` and `process` namespaces.

[glossary-proposal]: learning/glossary.md#proposal
[prometheus]: http://prometheus.io/
[prometheus-getting-started]: http://prometheus.io/docs/introduction/getting_started/
[prometheus-naming]: http://prometheus.io/docs/practices/naming/
