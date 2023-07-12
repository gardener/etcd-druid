# ETCD Druid

[![CI Build status](https://concourse.ci.gardener.cloud/api/v1/teams/gardener/pipelines/etcd-druid-master/jobs/master-head-update-job/badge)](https://concourse.ci.gardener.cloud/teams/gardener/pipelines/etcd-druid-master/jobs/master-head-update-job)
[![Go Report Card](https://goreportcard.com/badge/github.com/gardener/gardener)](https://goreportcard.com/report/github.com/gardener/etcd-druid)

## Background

[Etcd](https://github.com/etcd-io/etcd) in the control plane of Kubernetes clusters which are managed by Gardener is deployed as a StatefulSet. The statefulset has replica of a pod containing two containers namely, etcd and [backup-restore](https://github.com/gardener/etcd-backup-restore). The etcd container calls components in etcd-backup-restore via REST api to perform data validation before etcd is started. If this validation fails etcd data is restored from the latest snapshot stored in the cloud-provider's object store. Once etcd has started, the etcd-backup-restore periodically creates full and delta snapshots. It also performs defragmentation of etcd data periodically.

The etcd-backup-restore needs as input the cloud-provider information comprising of security credentials to access the object store, the object store bucket name and prefix for the directory used to store snapshots. Currently, for operations like migration and validation, the bash script has to be updated to initiate the operation.

## Goals

* Deploy etcd and etcd-backup-restore using an etcd CRD.
* Support more than one etcd replica.
* Perform scheduled snapshots.
* Support operations such as restores, defragmentation and scaling with zero-downtime.
* Handle cloud-provider specific operation logic.
* Trigger a full backup on request before volume deletion.
* Offline compaction of full and delta snapshots stored in object store.

## Proposal

The existing method of deploying etcd and backup-sidecar as a StatefulSet alleviates the pain of ensuring the pods are live and ready after node crashes. However, deploying etcd as a Statefulset introduces a plethora of challenges. The etcd controller should be smart enough to handle etcd statefulsets taking into account limitations imposed by statefulsets. The controller shall update the status regarding how to target the K8s objects it has created. This field in the status can be leveraged by `HVPA` to scale etcd resources eventually.

## CRD specification

The etcd CRD should contain the information required to create the etcd and backup-restore sidecar in a pod/statefulset.

```yaml
---

apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  finalizers:
  - druid.gardener.cloud/etcd
  name: test
  namespace: demo
spec:
  annotations:
    app: etcd-statefulset
    gardener.cloud/role: controlplane
    networking.gardener.cloud/to-dns: allowed
    networking.gardener.cloud/to-private-networks: allowed
    networking.gardener.cloud/to-public-networks: allowed
    role: test
  backup:
    deltaSnapshotMemoryLimit: 1Gi
    deltaSnapshotPeriod: 300s
    fullSnapshotSchedule: 0 */24 * * *
    garbageCollectionPeriod: 43200s
    garbageCollectionPolicy: Exponential
    imageRepository: eu.gcr.io/gardener-project/gardener/etcdbrctl
    imageVersion: v0.25.0
    port: 8080
    resources:
      limits:
        cpu: 500m
        memory: 2Gi
      requests:
        cpu: 23m
        memory: 128Mi
    snapstoreTempDir: /var/etcd/data/temp
  etcd:
    Quota: 8Gi
    clientPort: 2379
    defragmentationSchedule: 0 */24 * * *
    enableTLS: false
    imageRepository: eu.gcr.io/gardener-project/gardener/etcd-wrapper
    imageVersion: v0.1.0
    initialClusterState: new
    initialClusterToken: new
    metrics: basic
    pullPolicy: IfNotPresent
    resources:
      limits:
        cpu: 2500m
        memory: 4Gi
      requests:
        cpu: 500m
        memory: 1000Mi
    serverPort: 2380
    storageCapacity: 80Gi
    storageClass: gardener.cloud-fast
  sharedConfig:
    autoCompactionMode: periodic
    autoCompactionRetention: 30m
  labels:
    app: etcd-statefulset
    gardener.cloud/role: controlplane
    networking.gardener.cloud/to-dns: allowed
    networking.gardener.cloud/to-private-networks: allowed
    networking.gardener.cloud/to-public-networks: allowed
    role: test
  pvcRetentionPolicy: DeleteAll
  replicas: 1
  storageCapacity: 80Gi
  storageClass: gardener.cloud-fast
  store:
    storageContainer: test
    storageProvider: S3
    storePrefix: etcd-test
    storeSecret: etcd-backup
  tlsClientSecret: etcd-client-tls
  tlsServerSecret: etcd-server-tls
status:
  etcd:
    apiVersion: apps/v1
    kind: StatefulSet
    name: etcd-test
```

## Implementation Agenda

As first step implement defragmentation during maintenance windows. Subsequently, we will add zero-downtime upgrades and defragmentation.

## Workflow

### Deployment workflow

![controller-diagram](./docs/controller.png)

### Defragmentation workflow

![defrag-diagram](./docs/defrag.png)
