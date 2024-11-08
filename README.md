<img src="docs/assets/logo/etcd-druid-with-tagline.png" style="width:120%"></img>

[![REUSE status](https://api.reuse.software/badge/github.com/gardener/etcd-druid)](https://api.reuse.software/info/github.com/gardener/etcd-druid) [![CI Build status](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/etcd-druid-master/jobs/master-head-update-job/badge)](https://concourse.ci.gardener.cloud/teams/gardener/pipelines/etcd-druid-master/jobs/master-head-update-job) [![Go Report Card](https://goreportcard.com/badge/github.com/gardener/etcd-druid)](https://goreportcard.com/report/github.com/gardener/etcd-druid) [![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE) [![Release](https://img.shields.io/github/v/release/gardener/etcd-druid.svg?style=flat)](https://github.com/gardener/etcd-druid) [![Go Reference](https://pkg.go.dev/badge/github.com/gardener/etcd-druid.svg)](https://pkg.go.dev/github.com/gardener/etcd-druid) [![Docs](https://img.shields.io/badge/Docs-reference-orange)](https://gardener.github.io/etcd-druid/index.html)

`etcd-druid` is an [etcd](https://github.com/etcd-io/etcd) [operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) which makes it easy to configure, provision, reconcile, monitor and delete etcd clusters. It enables management of etcd clusters through [declarative Kubernetes API model](config/crd/bases/crd-druid.gardener.cloud_etcds.yaml). 

In every etcd cluster managed by `etcd-druid`, each etcd member is a two container `Pod` which consists of:

- [etcd-wrapper](https://github.com/gardener/etcd-wrapper) which manages the lifecycle (validation & initialization) of an etcd.
- [etcd-backup-restore](https://github.com/gardener/etcd-backup-restore) sidecar which currently provides the following capabilities (the list is not comprehensive):
  - [etcd](https://github.com/etcd-io/etcd) DB validation.
  - Scheduled [etcd](https://github.com/etcd-io/etcd) DB defragmentation.
  - Backup - [etcd](https://github.com/etcd-io/etcd) DB snapshots are taken regularly and backed in an object store if one is configured.
  - Restoration - In case of a DB corruption for a single-member cluster it helps in restoring from latest set of snapshots (full & delta).
  - Member control operations.

`etcd-druid` additional provides the following capabilities:

- Facilitates declarative scale-out of [etcd](https://github.com/etcd-io/etcd) clusters.
- Provides protection against accidental deletion/mutation of resources provisioned as part of an etcd cluster.
- Offers an asynchronous and threshold based capability to process backed up snapshots to:
  - Potentially minimize the recovery time by leveraging restoration from backups followed by [etcd's compaction and defragmentation](https://etcd.io/docs/v3.4/op-guide/maintenance/). 
  - Indirectly assert integrity of the backed up snaphots.
- Allows seamless copy of backups between any two object store buckets.

## Start using or developing `etcd-druid` locally

If you are looking to try out druid then you can use a [Kind](https://kind.sigs.k8s.io/) cluster based setup.  

https://github.com/user-attachments/assets/cfe0d891-f709-4d7f-b975-4300c6de67e4

For detailed documentation, see our [docs](https://gardener.github.io/etcd-druid/index.html).

## Contributions

If you wish to contribute then please see our [contributor guidelines](docs/development/contribution.md).

## Feedback and Support

We always look forward to active community engagement. Please report bugs or suggestions on how we can enhance `etcd-druid` on [GitHub Issues](https://github.com/gardener/etcd-druid/issues).

## License

Release under [Apache-2.0](https://github.com/gardener/etcd-druid/blob/master/LICENSE) license.
