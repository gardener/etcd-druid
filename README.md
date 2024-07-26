# ETCD Druid

<image src="logo/etcd-druid-logo.png" style="width:300px"></image>

[![REUSE status](https://api.reuse.software/badge/github.com/gardener/etcd-druid)](https://api.reuse.software/info/github.com/gardener/etcd-druid)
[![CI Build status](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/etcd-druid-master/jobs/master-head-update-job/badge)](https://concourse.ci.gardener.cloud/teams/gardener/pipelines/etcd-druid-master/jobs/master-head-update-job)
[![Go Report Card](https://goreportcard.com/badge/github.com/gardener/etcd-druid)](https://goreportcard.com/report/github.com/gardener/etcd-druid)
[![Go Reference](https://pkg.go.dev/badge/github.com/gardener/etcd-druid.svg)](https://pkg.go.dev/github.com/gardener/etcd-druid)

## Background

[Etcd](https://github.com/etcd-io/etcd) in the control plane of Kubernetes clusters which are managed by Gardener is deployed as a StatefulSet. The statefulset has replica of a pod containing two containers namely, etcd and [backup-restore](https://github.com/gardener/etcd-backup-restore). The etcd container calls components in etcd-backup-restore via REST api to perform data validation before etcd is started. If this validation fails etcd data is restored from the latest snapshot stored in the cloud-provider's object store. Once etcd has started, the etcd-backup-restore periodically creates full and delta snapshots. It also performs defragmentation of etcd data periodically.

The etcd-backup-restore needs as input the cloud-provider information comprising security credentials to access the object store, the object store bucket name and prefix for the directory used to store snapshots. Currently, for operations like migration and validation, the bash script has to be updated to initiate the operation.

## Goals

* Deploy etcd and etcd-backup-restore using an etcd CRD.
* Support more than one etcd replica.
* Perform scheduled snapshots.
* Support operations such as restores, defragmentation and scaling with zero-downtime.
* Handle cloud-provider specific operation logic.
* Trigger a full backup on request before volume deletion.
* Offline compaction of full and delta snapshots stored in object store.

