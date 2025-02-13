# Etcd Cluster Components

For every `Etcd` cluster that is provisioned by `etcd-druid` it deploys a set of resources. Following sections provides information and code reference to each such resource.

## StatefulSet

[StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) is the primary kubernetes resource that gets provisioned for an etcd cluster.

* Replicas for the StatefulSet are derived from `Etcd.Spec.Replicas` in the custom resource.

* Each pod comprises two containers:
  * `etcd-wrapper` : This is the main container which runs an etcd process.
  
  * `etcd-backup-restore` : This is a side-container which does the following:
    
    * Orchestrates the initialization of etcd. This includes validation of any existing etcd data directory, restoration in case of corrupt etcd data directory files for a single-member etcd cluster.
    * Periodically renews member lease.
    * Optionally takes schedule and threshold based delta and full snapshots and pushes them to a configured object store.
    * Orchestrates scheduled etcd-db defragmentation.
    
    > NOTE: This is not a complete list of functionalities offered out of `etcd-backup-restore`. 

**Code reference:** [StatefulSet-Component](https://github.com/gardener/etcd-druid/tree/480213808813c5282b19aff5f3fd6868529e779c/internal/component/statefulset)

> For detailed information on each container you can visit [etcd-wrapper](https://github.com/gardener/etcd-wrapper) and [etcd-backup-restore](https://github.com/gardener/etcd-backup-restore) repositories.

## ConfigMap

Every `etcd` member requires [configuration](https://etcd.io/docs/v3.4/op-guide/configuration/) with which it must be started. `etcd-druid` creates a [ConfigMap](https://kubernetes.io/docs/concepts/configuration/configmap/) which gets mounted onto the `etcd-backup-restore` container. `etcd-backup-restore` container will modify the etcd configuration and serve it to the `etcd-wrapper` container upon request.

**Code reference:** [ConfigMap-Component](https://github.com/gardener/etcd-druid/tree/480213808813c5282b19aff5f3fd6868529e779c/internal/component/configmap)

## PodDisruptionBudget

An etcd cluster requires quorum for all write operations. Clients can additionally configure quorum based reads as well to ensure [linearizable](https://jepsen.io/consistency/models/linearizable) reads (kube-apiserver's etcd client is configured for linearizable reads and writes). In a cluster of size 3, only 1 member failure is tolerated. [Failure tolerance](https://etcd.io/docs/v3.3/faq/#what-is-failure-tolerance) for an etcd cluster with replicas `n` is computed as `(n-1)/2`.

To ensure that etcd pods are not evicted more than its failure tolerance, `etcd-druid` creates a [PodDisruptionBudget](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/#pod-disruption-budgets). 

!!! note
    For a single node etcd cluster a `PodDisruptionBudget` will be created, however `pdb.spec.minavailable` is set to 0 effectively disabling it.

**Code reference:** [PodDisruptionBudget-Component](https://github.com/gardener/etcd-druid/tree/480213808813c5282b19aff5f3fd6868529e779c/internal/component/poddistruptionbudget)

## ServiceAccount

`etch-backup-restore` container running as a side-car in every etcd-member, requires permissions to access resources like `Lease`, `StatefulSet` etc. A dedicated [ServiceAccount](https://kubernetes.io/docs/concepts/security/service-accounts/) is created per `Etcd` cluster for this purpose.

**Code reference:** [ServiceAccount-Component](https://github.com/gardener/etcd-druid/tree/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/component/serviceaccount)

## Role & RoleBinding

`etch-backup-restore` container running as a side-car in every etcd-member, requires permissions to access resources like `Lease`, `StatefulSet` etc. A dedicated [Role]() and [RoleBinding]() is created and linked to the [ServiceAccount](https://kubernetes.io/docs/concepts/security/service-accounts/) created per `Etcd` cluster.

**Code reference:** [Role-Component](https://github.com/gardener/etcd-druid/tree/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/component/role) & [RoleBinding-Component](https://github.com/gardener/etcd-druid/tree/master/internal/component/rolebinding)

## Client & Peer Service

To enable clients to connect to an etcd cluster a ClusterIP `Client` [Service](https://kubernetes.io/docs/concepts/services-networking/service/) is created. To enable `etcd` members to talk to each other(for discovery, leader-election, raft consensus etc.) `etcd-druid` also creates a [Headless Service](https://kubernetes.io/docs/concepts/services-networking/service/#headless-services).

**Code reference:** [Client-Service-Component](https://github.com/gardener/etcd-druid/tree/480213808813c5282b19aff5f3fd6868529e779c/internal/component/clientservice) & [Peer-Service-Component](https://github.com/gardener/etcd-druid/tree/480213808813c5282b19aff5f3fd6868529e779c/internal/component/peerservice)

## Member Lease

Every member in an `Etcd` cluster has a dedicated [Lease](https://kubernetes.io/docs/concepts/architecture/leases/) that gets created which signifies that the member is alive. It is the responsibility of the `etcd-backup-store` side-car container to periodically renew the lease.

!!! note 
    Today the lease object is also used to indicate the member-ID and the role of the member in an etcd cluster. Possible roles are `Leader`, `Member`(which denotes that this is a member but not a leader). This will change in the future with [EtcdMember resource](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/docs/proposals/04-etcd-member-custom-resource.md).

**Code reference:** [Member-Lease-Component](https://github.com/gardener/etcd-druid/tree/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/component/memberlease)

## Delta & Full Snapshot Leases

One of the responsibilities of `etcd-backup-restore` container is to take periodic or threshold based snapshots (delta and full) of the etcd DB.  Today `etcd-backup-restore` communicates the end-revision of the latest full/delta snapshots to `etcd-druid` operator via leases.

`etcd-druid` creates two [Lease](https://kubernetes.io/docs/concepts/architecture/leases/) resources one for delta and another for full snapshot. This information is used by the operator to trigger [snapshot-compaction](../proposals/02-snapshot-compaction.md) jobs. Snapshot leases are also used to derive the health of backups which gets updated in the `Status` subresource of every `Etcd` resource.

> In future these leases will be replaced by [EtcdMember resource](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/docs/proposals/04-etcd-member-custom-resource.md).

**Code reference:** [Snapshot-Lease-Component](https://github.com/gardener/etcd-druid/tree/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/component/snapshotlease)
