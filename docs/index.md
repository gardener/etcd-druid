<style>
  .md-content h1 { display: none; }
</style>
<p style="text-align: center;">
<img src="assets/logo/etcd-druid-with-tagline.png" style="width:120%" alt="etcd-druid-tagline">
</p>

<p style="text-align: center;">
    <a href="https://api.reuse.software/info/github.com/gardener/etcd-druid">
      <img alt="REUSE Status" src="https://api.reuse.software/badge/github.com/gardener/etcd-druid">
    </a>
    <a href="https://concourse.ci.gardener.cloud/teams/gardener/pipelines/etcd-druid-master/jobs/master-head-update-job">
      <img alt="CI Build Status" src="https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/etcd-druid-master/jobs/master-head-update-job/badge">
    </a>
    <a href="https://goreportcard.com/report/github.com/gardener/etcd-druid">
      <img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/gardener/etcd-druid">
    </a>
    <a href="https://github.com/gardener/etcd-druid/blob/master/LICENSE">
      <img alt="License: Apache-2.0" src="https://img.shields.io/badge/License-Apache--2.0-blue.svg">
    </a>
    <a href="https://github.com/gardener/etcd-druid/releases">
      <img alt="Release" src="https://img.shields.io/github/v/release/gardener/etcd-druid.svg?style=flat">
    </a>
    <a href="https://pkg.go.dev/github.com/gardener/etcd-druid">
      <img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/gardener/etcd-druid.svg">
    </a>
</p>

`etcd-druid` is an [etcd](https://github.com/etcd-io/etcd) [operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) which makes it easy to configure, provision, reconcile, monitor and delete etcd clusters. It enables management of etcd clusters through [declarative Kubernetes API model](../api/core/crds/druid.gardener.cloud_etcds.yaml).

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

<video controls>
  <source src="https://github.com/user-attachments/assets/cfe0d891-f709-4d7f-b975-4300c6de67e4" type="video/mp4">
</video>

For detailed documentation, see our `/docs` folder. Please find the [index](README.md) here.

## Contributions

If you wish to contribute then please see our [contributor guidelines](development/contribution.md).

## Feedback and Support

We always look forward to active community engagement. Please report bugs or suggestions on how we can enhance `etcd-druid` on [GitHub Issues](https://github.com/gardener/etcd-druid/issues).

## License

Release under [Apache-2.0](https://github.com/gardener/etcd-druid/blob/master/LICENSE) license.
