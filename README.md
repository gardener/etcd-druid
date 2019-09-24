# ETCD Druid

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

apiVersion: druid.sapcloud.io/v1
kind: Etcd
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"druid.sapcloud.io/v1","kind":"Etcd","metadata":{"annotations":{},"labels":{"app":"etcd-statefulset","garden.sapcloud.io/role":"controlplane","role":"test"},"name":"test","namespace":"shoot--dev--i308301-1"},"spec":{"annotations":{"app":"etcd-statefulset","garden.sapcloud.io/role":"controlplane","networking.gardener.cloud/to-dns":"allowed","networking.gardener.cloud/to-private-networks":"allowed","networking.gardener.cloud/to-public-networks":"allowed","role":"test"},"backup":{"deltaSnapshotMemoryLimit":104857600,"deltaSnapshotPeriod":"300s","etcdConnectionTimeout":"300s","etcdQuotaBytes":8589934592,"fullSnapshotSchedule":"0 */24 * * *","garbageCollectionPeriod":"43200s","garbageCollectionPolicy":"Exponential","imageRepository":"eu.gcr.io/gardener-project/gardener/etcdbrctl","imageVersion":"0.8.0-dev","port":8080,"pullPolicy":"IfNotPresent","resources":{"limits":{"cpu":"500m","memory":"2Gi"},"requests":{"cpu":"23m","memory":"128Mi"}},"snapstoreTempDir":"/var/etcd/data/temp"},"etcd":{"clientPort":2379,"defragmentationSchedule":"0 */24 * * *","enableTLS":false,"imageRepository":"quay.io/coreos/etcd","imageVersion":"v3.3.13","initialClusterState":"new","initialClusterToken":"new","metrics":"basic","pullPolicy":"IfNotPresent","resources":{"limits":{"cpu":"2500m","memory":"4Gi"},"requests":{"cpu":"500m","memory":"1000Mi"}},"serverPort":2380,"storageCapacity":"80Gi","storageClass":"gardener.cloud-fast"},"labels":{"app":"etcd-statefulset","garden.sapcloud.io/role":"controlplane","networking.gardener.cloud/to-dns":"allowed","networking.gardener.cloud/to-private-networks":"allowed","networking.gardener.cloud/to-public-networks":"allowed","role":"test"},"pvcRetentionPolicy":"DeleteAll","replicas":1,"storageCapacity":"80Gi","storageClass":"gardener.cloud-fast","store":{"storageContainer":"shoot--dev--i308301-1--b3caa","storageProvider":"S3","storePrefix":"etcd-test","storeSecret":"etcd-backup"},"tlsClientSecret":"etcd-client-tls","tlsServerSecret":"etcd-server-tls"}}
  creationTimestamp: 2019-09-12T12:20:04Z
  finalizers:
  - druid.sapcloud.io/etcd-druid
  generation: 3
  labels:
    app: etcd-statefulset
    garden.sapcloud.io/role: controlplane
    role: test
  name: test
  namespace: shoot--dev--i308301-1
  resourceVersion: "75172656"
  selfLink: /apis/druid.sapcloud.io/v1/namespaces/shoot--dev--i308301-1/etcds/test
  uid: a6afc65f-d557-11e9-8ea7-469a1879b8a9
spec:
  annotations:
    app: etcd-statefulset
    garden.sapcloud.io/role: controlplane
    networking.gardener.cloud/to-dns: allowed
    networking.gardener.cloud/to-private-networks: allowed
    networking.gardener.cloud/to-public-networks: allowed
    role: test
  backup:
    deltaSnapshotMemoryLimit: 104857600
    deltaSnapshotPeriod: 300s
    etcdConnectionTimeout: 300s
    etcdQuotaBytes: 8589934592
    fullSnapshotSchedule: 0 */24 * * *
    garbageCollectionPeriod: 43200s
    garbageCollectionPolicy: Exponential
    imageRepository: eu.gcr.io/gardener-project/gardener/etcdbrctl
    imageVersion: 0.8.0-dev
    port: 8080
    pullPolicy: IfNotPresent
    resources:
      limits:
        cpu: 500m
        memory: 2Gi
      requests:
        cpu: 23m
        memory: 128Mi
    snapstoreTempDir: /var/etcd/data/temp
  etcd:
    clientPort: 2379
    defragmentationSchedule: 0 */24 * * *
    enableTLS: false
    imageRepository: quay.io/coreos/etcd
    imageVersion: v3.3.13
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
  labels:
    app: etcd-statefulset
    garden.sapcloud.io/role: controlplane
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
