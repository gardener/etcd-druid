# Setting up etcd-druid in Production

You can get familiar with `etcd-druid` and all the resources that it creates by setting up etcd-druid locally by following the [detailed guide](getting-started-locally/getting-started-locally.md). This document lists down recommendations for a productive setup of etcd-druid.

## Helm Charts

You can use [helm](https://helm.sh/) charts at [this](https://github.com/gardener/etcd-druid/tree/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/charts/druid) location to deploy druid. Values for charts are present [here](https://github.com/gardener/etcd-druid/blob/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/charts/druid/values.yaml) and can be configured as per your requirement. Following charts are present:

* `deployment.yaml` - defines a kubernetes [Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/) for etcd-druid. To configure the CLI flags for druid you can refer to [this](configure-etcd-druid.md) document which explains these flags in detail.
* `serviceaccount.yaml` - defines a kubernetes [ServiceAccount](https://kubernetes.io/docs/concepts/security/service-accounts/) which will serve as a technical user to which role/clusterroles can be bound.

* `clusterrole.yaml` - etcd-druid can manage multiple etcd clusters. In a `hosted control plane` setup (e.g. [Gardener](https://github.com/gardener/gardener)), one would typically create separate [namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) per control-plane. This would require a [ClusterRole](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#role-and-clusterrole) to be defined which gives etcd-druid permissions to operate across namespaces. Packing control-planes via namespaces provides you better resource utilisation while providing you isolation from the data-plane (where the actual workload is scheduled).
* `rolebinding.yaml` -  binds the [ClusterRole](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#role-and-clusterrole) defined in `druid-clusterrole.yaml` to the [ServiceAccount](https://kubernetes.io/docs/concepts/security/service-accounts/) defined in `service-account.yaml`.
* `service.yaml` - defines a `Cluster IP` [Service](https://kubernetes.io/docs/concepts/services-networking/service/) allowing other control-plane components to communicate to `http` endpoints exposed out of etcd-druid (e.g. enables [prometheus](https://prometheus.io/) to scrap metrics, validating webhook to be invoked upon change to `Etcd` CR etc.)
* `secret-ca-crt.yaml` - Contains the base64 encoded CA certificate used for the etcd-druid webhook server.
* `secret-server-tls-crt.yaml` -  Contains the base64 encoded server certificate used for the etcd-druid webhook server.
* `validating-webhook-config.yaml` - Configuration for all webhooks that etcd-druid registers to the webhook server. At the time of writing this document [EtcdComponents](../concepts/etcd-cluster-resource-protection.md) webhook gets registered.

## Etcd cluster size

[Recommendation](https://etcd.io/docs/v3.3/faq/#why-an-odd-number-of-cluster-members) from upstream etcd is to always have an odd number of members in an `Etcd` cluster.

## Mounted Volume

All `Etcd` cluster member [Pods](https://kubernetes.io/docs/concepts/workloads/pods/) provisioned by etcd-druid mount a [Persistent Volume](https://kubernetes.io/docs/concepts/storage/persistent-volumes/). A mounted persistent  storage helps in faster recovery in case of single-member transient failures. `etcd` is I/O intensive and its performance is heavily dependent on the [Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/). It is therefore recommended that high performance SSD drives be used.

At the time of writing this document etcd-druid provisions the following volume types:

| Cloud Provider | Type                                        | Size |
| -------------- | ------------------------------------------- | ---- |
| AWS            | GP3                                         | 25Gi |
| Azure          | Premium SSD                                 | 33Gi |
| GCP            | Performance (SSD) Persistent Disks (pd-ssd) | 25Gi |

> Also refer: [Etcd Disk recommendation](https://etcd.io/docs/v3.4/op-guide/hardware/#disks).
>
> Additionally, each cloud provider offers redundancy for managed disks. You should choose redundancy as per your availability requirement.

## Backup & Restore

A permanent quorum loss or data-volume corruption is a reality in production clusters and one must ensure that data loss is minimized. `Etcd` clusters provisioned via etcd-druid offer two levels of data-protection

Via [etcd-backup-restore](https://github.com/gardener/etcd-backup-restore) all clusters started via etcd-druid get the capability to regularly take delta & full snapshots. These snapshots are stored in an object store. Additionally, a `snapshot-compaction` job is run to compact and defragment the latest snapshot, thereby reducing the time it takes to restore a cluster in case of a permanent quorum loss. You can read the [detailed guide](../usage/recovering-etcd-clusters.md) on how to restore from permanent quorum loss.

It is therefore recommended that you configure an `Object store` in the cloud/infra provider of your choice, enabled backup & restore functionality by filling in [store](https://github.com/gardener/etcd-druid/blob/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/api/v1alpha1/etcd.go#L143) configuration of an `Etcd` custom CR.

### Ransomware protection

Ransomware is a form of malware designed to encrypt files on a device, rendering any files and the systems that rely on them unusable. All cloud providers ([aws](https://aws.amazon.com/s3/features/object-lock/), [gcp](https://cloud.google.com/storage/docs/bucket-lock), [azure](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-storage-overview)) provide a feature of immutability that can be set at the bucket/object level which provides `WORM` access to objects as long as the bucket/lock retention duration.

All delta & full snapshots that are periodically taken by `etcd-backup-restore` are stored in Object store provided by a cloud provider. It is recommended that these backups be protected from ransomware protection by turning locking at the bucket/object level.

## Security

### Use Distroless Container Images

It is generally recommended to use a minimal base image which additionally reduces the attack surface. Google's [Distroless](https://github.com/GoogleContainerTools/distroless) is one way to reduce the attack surface and also minimize the size of the base image. It provides the following benefits:

* Reduces the attack surface
* Minimizes vulnerabilities
* No shell
* Reduced size - only includes what is necessary

For every `Etcd` cluster provisioned by etcd-druid, `distroless` images are used as base images.

### Enable TLS for Peer and Client communication

Generally you should enable TLS for peer and client communication for an `Etcd` cluster.  To enable TLS CA certificate, server and client certificates needs to be generated.
You can refer to the list of TLS artifacts that are generated for an `Etcd` cluster provisioned by etcd-druid [here](../usage/securing-etcd-clusters.md).

### Enable TLS for Druid Webhooks

If you choose to enable webhooks in `etcd-druid`  then it is necessary to create a separate CA and server certificate to be used by the webhooks.

### Rotate TLS artifacts

It is generally recommended to rotate all TLS certificates to reduce the chances of it getting leaked or have expired. Kubernetes does not support revocation of certificates (see [issue#18982](https://github.com/kubernetes/kubernetes/issues/18982)). One possible way to revoke certificates is to also revoke the entire chain including CA certificates.

## Scaling etcd pods

`etcd` clusters cannot be scaled-out horizontly to meet the increased traffic/storage demand for the following reasons:

* There is a soft limit of 8GB and a hard limit of 10GB for the etcd DB beyond which perfomance and stability of etcd is not guaranteed. 
* All members of etcd maintain the entire replica of the entire DB, thus scaling-out will not really help if the storage demand grows.
* Increasing the number of cluster members beyond 5 also increases the cost of consensus amongst now a larger quorum, increases load on the single leader as it needs to also participate in bringing up [etcd learner](https://etcd.io/docs/v3.3/learning/learner/).

Therefore the following is recommended:

* To meet the increased demand, configure a [VPA](https://github.com/kubernetes/autoscaler/tree/cecb34cb863fb015264098b5379bdba40a9113cf/vertical-pod-autoscaler). You have to be careful on selection of `containerPolicies`, `targetRef`.
* To meet the increased demand in storage etcd-druid already configures each etcd member to [auto-compact](https://etcd.io/docs/v3.4/op-guide/maintenance/#auto-compaction) and it also configures periodic [defragmentation](https://etcd.io/docs/v3.4/op-guide/maintenance/#defragmentation) of the etcd DB. The only case this will not help is when you only have unique writes all the time.

> **Note:** Care should be taken with usage of VPA. While it helps to vertically scale up etcd-member pods, it also can cause transient quorum loss. This is a direct consequence of the design of VPA - where recommendation is done by [Recommender](https://github.com/kubernetes/autoscaler/blob/2800c70d425b89e88cb6e608df494a0cd21f242d/vertical-pod-autoscaler/pkg/recommender/README.md) component, [Updater](https://github.com/kubernetes/autoscaler/blob/2800c70d425b89e88cb6e608df494a0cd21f242d/vertical-pod-autoscaler/pkg/updater/README.md) evicts the pods that do not have the resources recommended by the `Recommender` and [Admission Controller](https://github.com/kubernetes/autoscaler/blob/2800c70d425b89e88cb6e608df494a0cd21f242d/vertical-pod-autoscaler/pkg/admission-controller/README.md) which updates the resources on the Pods. All these three components act asynchronously and can fail independently, so while VPA respects PDB's it can easily enter into a state where updater evicts a pod while respecting PDB but the admission controller fails to apply the recommendation. The pod comes with a default resources which still differ from the recommended values, thus causing a repeat eviction. There are other race conditions that can also occur and one needs to be careful of using VPA for quorum based workloads.

## High Availability

To ensure that an `Etcd` cluster is highly available, following is recommended:

### Ensure that the `Etcd` cluster members are spread

`Etcd` cluster members should always be spread across nodes. This provides you failure tolerance at the node level. For failure tolerance of a zone, it is recommended that you spread the `Etcd` cluster members across zones.
We recommend that you use a combination of [TopologySpreadConstraints](https://kubernetes.io/docs/concepts/scheduling-eviction/topology-spread-constraints/) and [Pod Anti-Affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity). To set the scheduling constraints you can either specify these constraints using [SchedulingConstraints](https://github.com/gardener/etcd-druid/blob/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/api/v1alpha1/etcd.go#L257-L265) in the `Etcd` custom resource or use a [MutatingWebhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) to dynamically inject these into pods.

An example of scheduling constraints for a multi-node cluster with zone failure tolerance will be:
```yaml
  topologySpreadConstraints:
  - labelSelector:
      matchLabels:
        app.kubernetes.io/component: etcd-statefulset
        app.kubernetes.io/managed-by: etcd-druid
        app.kubernetes.io/name: etcd-main
        app.kubernetes.io/part-of: etcd-main
    maxSkew: 1
    minDomains: 3
    topologyKey: kubernetes.io/hostname
    whenUnsatisfiable: DoNotSchedule
  - labelSelector:
      matchLabels:
        app.kubernetes.io/component: etcd-statefulset
        app.kubernetes.io/managed-by: etcd-druid
        app.kubernetes.io/name: etcd-main
        app.kubernetes.io/part-of: etcd-main
    maxSkew: 1
    minDomains: 3
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule
```

For a 3 member etcd-cluster, the above TopologySpreadConstraints will ensure that the members will be spread across zones (assuming there are 3 zones -> minDomains=3) and no two members will be on the same node.

### Optimize Network Cost

In most cloud providers there is no network cost (ingress/egress) for any traffic that is confined within a single zone. For `Zonal` failure tolerance, it will become imperative to spread the `Etcd` cluster across zones within a region. Knowing that an `Etcd` cluster members are quite chatty (leader election, consensus building for writes and linearizable reads etc.), this can add to the network cost.

One could evaluate using [TopologyAwareRouting](https://kubernetes.io/docs/concepts/services-networking/topology-aware-routing/) which reduces cross-zonal traffic thus saving costs and latencies.

> **Note:** You can read about how it is done in Gardener [here](https://github.com/gardener/gardener/blob/master/docs/operations/topology_aware_routing.md).

## Metrics & Alerts

Monitoring `etcd` metrics is essential for fine tuning `Etcd` clusters. etcd already exports a lot of [metrics](https://etcd.io/docs/v3.4/metrics/). You can see the complete list of metrics that are exposed out of an `Etcd` cluster provisioned by etcd-druid [here](../monitoring/metrics.md). It is also recommended that you configure an alert for [etcd space quota alarms](https://etcd.io/docs/v3.2/op-guide/maintenance/#space-quota).

## Hibernation

If you have a concept of `hibernating` kubernetes clusters, then following should be kept in mind:

* Before you bring down the `Etcd` cluster, leverage the capability to take a `full snapshot` which captures the state of the etcd DB and stores it in the configured Object store. This ensures that when the cluster is woken up from hibernation it can restore from the last state with no data loss.
* To save costs you should consider deleting the [PersistentVolumeClaims](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) associated to the StatefulSet pods. However, it must be ensured that you take a full snapshot as highlighted in the previous point.
* When the cluster is woken up from hibernation then you should do the following (assuming prior to hibernation the cluster had a size of 3 members):
  * Start the `Etcd` cluster with 1 replica. Let it restore from the last full snapshot.
  * Once the cluster reports that it is ready, only then increase the replicas to its original value (e.g. 3). The other two members will start up each as learners and post learning they will join as voting members (`Followers`).

## Reference

* A nicely written [blog post](https://gardener.cloud/blog/2023/03-27-high-availability-and-zone-outage-toleration/) on `High Availability and Zone Outage Toleration` has a lot of recommendations that one can borrow from.
