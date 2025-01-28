---
title: Immutable etcd Cluster Backups
dep-number: 06
creation-date: 2024-09-25
status: implementable
authors:
- "@seshachalam-yv"
- "@renormalize"
- "@ishan16696"
reviewers:
- "@etcd-druid-maintainers"
---

# DEP-06: Immutable etcd Cluster Backups

## Summary

This proposal introduces immutable backups for etcd clusters managed by `etcd-druid`. By leveraging cloud provider immutability features, backups can neither be modified nor deleted once created. This approach strengthens the reliability and fault tolerance of the etcd restoration process.

## Terminology

- **etcd-druid:** An etcd operator that configures, provisions, reconciles, and monitors etcd clusters.
- **etcd-backup-restore:** A sidecar container that manages backups and restores of etcd cluster state. For more information, see the [etcd-backup-restore](https://github.com/gardener/etcd-backup-restore/blob/master/README.md) documentation.
- **WORM (Write Once, Read Many):** A storage model in which data, once written, cannot be modified or deleted until certain conditions are met.
- **Immutability:** The property of an object that prevents it from being modified or deleted after creation.
- **Immutability Period:** The duration for which data must remain immutable before it can be modified or deleted.
- **Bucket-Level Immutability:** A policy that applies a uniform immutability period to all objects within a bucket.
- **Object-Level Immutability:** A policy that allows setting immutability periods individually for objects within a bucket, offering more granular control.
- **Garbage Collection:** The process of deleting old snapshot data that is no longer needed to free up storage space.
- **Hibernation:** A state in which an etcd cluster is scaled down to zero replicas, effectively pausing its operations. This is typically done to save costs when the cluster is not needed for an extended period. During hibernation, the cluster's data remains intact, and it can be resumed to its previous state when required.

## Motivation

`etcd-druid` provisions etcd clusters and manages their lifecycle. For every etcd cluster, consumers can enable periodic backups of the cluster state by configuring the `spec.backup` section in an Etcd custom resource. Periodic backups are taken via the `etcd-backup-restore` sidecar container that runs in each etcd member pod.

Periodic backups of an etcd cluster state ensure the ability to recover from a complete quorum loss, enhancing reliability and fault tolerance. It is crucial that these backups, which are vital for restoring the etcd cluster, remain protected from any form of tampering, whether intentional or accidental. To safeguard the integrity of these backups, the authors recommend utilizing `WORM` protection, a feature offered by various cloud providers, to ensure the backups remain immutable and secure.

### Goals

- Protect backup data against modifications and deletions post-creation through immutability policies offered by storage providers.

### Non-Goals

- Implementing hibernation support via `etcd.spec` or annotations on the `Etcd` CR (i.e., specifying an intent for hibernation), as noted in [gardener/etcd-druid#922](https://github.com/gardener/etcd-druid/issues/922).
- Supporting immutable backups on storage providers that do not offer immutability features (e.g., OpenStack Swift).

## Proposal

This proposal aims to improve backup storage security by using immutability features available on major cloud providers.

### Supported Cloud Providers

- **Google Cloud Storage (GCS):** [Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
- **Amazon S3 (S3):** [Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
- **Azure Blob Storage (ABS):** [Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)


#### Types of Immutability

1. **Object-Level Immutability:** Allows setting immutability periods independently for each object within a bucket.
2. **Bucket-Level Immutability:** Applies a uniform immutability policy to all objects in a bucket.

### Comparison of Bucket-Level and Object-Level Immutability

| Feature                                                                   | GCS                     | S3            | ABS                                |
|---------------------------------------------------------------------------|-------------------------|--------------|-------------------------------------|
| Can bucket-level immutability period be increased?                        | Yes                     | Yes*         | Yes (only 5 times)                  |
| Can bucket-level immutability period be decreased?                        | No                      | Yes*         | No                                  |
| Is bucket-level immutability a prerequisite for object-level immutability?| No                      | Yes          | Yes (for existing buckets)          |
| Can object-level immutability period be increased?                        | Yes                     | Yes          | Yes                                 |
| Can object-level immutability period be decreased?                        | No                      | No           | No                                  |
| Support for enabling object-level immutability in existing buckets        | No                      | Yes          | Yes                                 |
| Support for enabling object-level immutability in new buckets             | Yes                     | Yes          | Yes                                 |
| Precedence between bucket-level and object-level immutability periods     | Max(bucket, object)     | Object-level | Max(bucket, object)                 |

> [!NOTE]
> In AWS S3, it is possible to decrease the bucket-level immutability period; however, this action may be blocked by specific bucket policy settings.  
> For GCS, object-level immutability is not yet supported for existing buckets; see [this issue](https://issuetracker.google.com/issues/346679415?pli=1).

### Recommended Approach

Given the nuances across providers:

- **S3 and ABS:** typically require bucket-level immutability as a prerequisite for object-level immutability.
- **GCS** does not currently support object-level immutability in existing buckets.
- **ABS** requires a [migration process](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-version-level-worm-policies#migration) to enable version-level immutability on existing containers.

Consequently, the authors recommend **bucket-level immutability**. This approach simplifies configuration and ensures a uniform immutability policy for all backups in a bucket.

### Configuring Immutable Backups

Currently, `etcd-druid` does not directly configure immutable buckets. The specific method of enabling immutability depends on your use case:

- **Large-Scale Consumers (e.g., Gardener):**  
  Typically, these consumers automate the configuration of immutability for both existing and new buckets, as detailed [here](https://github.com/gardener/gardener/issues/10866).

- **Standalone Consumers of `etcd-druid`:**  
  These users must manually configure immutability settings using their cloud provider's CLI, SDK, or web console.

#### Prerequisites

1. **Configure or Update the Immutable Bucket**  
   - Use your cloud provider’s CLI, SDK, or console to create (or update) a bucket/container with a WORM (write-once-read-many) immutability policy.  
   - Refer to the [Getting Started guide](https://github.com/gardener/etcd-backup-restore/blob/master/docs/usage/enabling_immutable_snapshots.md#configure-bucket-level-immutability) for step-by-step instructions on configuring or updating the immutable bucket across different cloud providers.
   - For AWS S3, for example, you enable Object Lock at bucket creation; for Azure Blob Storage, you configure Immutable Blob Storage at the container scope.

2. **Set `Etcd.spec.backup.store` to Reference This Bucket**  
   - In the `Etcd` custom resource (CR), specify the `store` configuration (e.g., `spec.backup.store`) to point to the bucket name you configured above.

3. **Provide Valid Credentials in a Kubernetes Secret**  
   - The `store` section of the `Etcd` CR must reference a `Secret` containing valid credentials.  
   - Confirm that this secret has the proper permissions to upload and retrieve snapshots from the immutable bucket.  
   - See the [Getting Started guide](https://github.com/gardener/etcd-druid/blob/master/docs/deployment/getting-started-locally/getting-started-locally.md#setting-up-cloud-provider-object-store-secret) for an example.

4. **(Gardener Only) Credential Rotation**  
   - **Note**: In Gardener-based setups, credential rotation is automatically handled by `gardener/gardener`. No manual rotation is required.  
   - In non-Gardener environments, you must manage credential updates and secret rotation yourself.

By following these steps, you will have set up an immutable bucket for storing etcd backups, along with the necessary references in your `Etcd` specification and Kubernetes secret.

### Handling of Hibernated Clusters

When an etcd cluster is hibernated for a period longer than the bucket’s immutability period, backups might become mutable again (depending on the cloud provider, see [Comparison of Storage Provider Properties](#comparison-of-bucket-level-and-object-level-immutability)). This possibility undermines the intended guarantees of immutability and may expose backups to accidental or malicious alterations.

As mentioned in [gardener/etcd-druid#922](https://github.com/gardener/etcd-druid/issues/922), a clear hibernation signal is needed. Since hibernation is not yet supported in `etcd-druid` and is out of scope for this proposal, we only address the method for maintaining immutability.

#### Proposal

To mitigate the risk of backups becoming mutable during extended hibernation under bucket-level immutability, the authors propose the following approach:

1. **Prerequisite: Take a Final Full Snapshot Before Hibernation**  
   - Before scaling the etcd cluster down to zero replicas, the etcd controller triggers an [on-demand full snapshot](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/05-etcd-operator-tasks.md#trigger-on-demand-fulldelta-snapshot). This ensures that the latest state of the etcd cluster is captured and securely stored before hibernation commences.
2. **Periodically Re-Upload the Snapshot**  
   - Re-uploading the latest full snapshot resets its immutability period in the bucket, thereby keeping the backups protected during hibernation.  
   - By default, the re-upload schedule is determined by `etcd.spec.backup.fullSnapshotSchedule`. At present, this interval cannot be customized exclusively for re-uploads; future enhancements may introduce a dedicated configuration parameter.  
   - A new operator task type, `ExtendFullSnapshotImmutabilityTask`, will periodically invoke the `reupload-snapshot` and `garbage-collect` commands.

3. **Enhance `etcd-backup-restore`**  
   - Introduce new CLI commands:
     - **`reupload-snapshot`** for re-uploading snapshots.  
     - **`garbage-collect`** for removing older backups whose immutability period has expired.

By capturing a final full snapshot before hibernation, periodically re-uploading it to preserve immutability, and removing stale backups, etcd backups remain safeguarded against accidental or malicious alterations until the cluster is resumed.

##### Etcd CR API Changes

A new field is introduced in `Etcd.spec.backup.store` to indicate the immutability strategy:

```go
// StoreSpec defines parameters for storing etcd backups.
type StoreSpec struct {
    // ...
    // Immutability configuration for the backup store.
    Immutability *ImmutabilitySpec `json:"immutability,omitempty"`
}

// ImmutabilitySpec defines immutability settings.
type ImmutabilitySpec struct {
    // RetentionType indicates the type of immutability approach. For example, "Bucket".
    RetentionType string `json:"retentionType,omitempty"`
}
```

If `immutability` is not specified, `etcd-druid` will assume that the bucket is mutable, and no immutability-related logic applies.

##### `etcd-backup-restore` Enhancements

The authors propose adding two new commands to the `etcd-backup-restore` CLI (`etcdbrctl`) to maintain immutability during hibernation and to clean up older snapshots:

1. **`reupload-snapshot`**
   - Downloads the latest full snapshot from the object store.
   - Renames the snapshot (for instance, updates its Unix timestamp) to avoid overwriting an existing immutable snapshot.
   - Uploads the renamed snapshot back to object storage, thereby restarting its immutability timer.

2. **`garbage-collect`**
   - Scans the object store for older snapshots and deletes them if their immutability period has expired and they are no longer needed, following the standard [garbage collection policy](https://github.com/gardener/etcd-backup-restore/blob/master/docs/usage/garbage_collection.md).

##### etcd Controller Enhancements

When a hibernation flow is initiated (by external tooling or higher-level operators), the [etcd controller](https://github.com/gardener/etcd-druid/blob/master/docs/development/controllers.md#etcd-controller) can:

1. Remove etcd’s client ports (2379/2380) from the etcd Service to block application traffic.
2. Trigger an [on-demand full snapshot](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/05-etcd-operator-tasks.md#trigger-on-demand-fulldelta-snapshot) via an `EtcdOperatorTask`.
3. Scale down the `StatefulSet` replicas to zero, provided the previous snapshot step is successful.
4. Create the `ExtendFullSnapshotImmutabilityTask` if `etcd.spec.backup.store.immutability.retentionType` is `"Bucket"` and based on `etcd.spec.backup.fullSnapshotSchedule`.

##### Operator Task Enhancements

The `ExtendFullSnapshotImmutabilityTask` will create a cron job that:

- Runs `etcdbrctl reupload-snapshot` to extend the immutability of the most recent snapshot.
- Runs `etcdbrctl garbage-collect --garbage-collection-policy <policy>` to remove old snapshots.

By periodically re-uploading the latest snapshot during hibernation, the authors ensure that the immutability period is extended, and the backups remain **protected throughout the hibernation period**.

###### Example Task Config

```go
type ExtendFullSnapshotImmutabilityTaskConfig struct {
  // Schedule defines a cron schedule (e.g., "0 */6 * * *").
  Schedule *string `json:"schedule,omitempty"`

  // GarbageCollectionConfig specifies the configuration for snapshot GC.
  GarbageCollectionConfig *GarbageCollectionConfig `json:"garbageCollectionConfig,omitempty"`
}

type GarbageCollectionConfig struct {
  // GarbageCollectionPolicy (e.g., "LimitBased" or "Exponential").
  GarbageCollectionPolicy *string `json:"garbageCollectionPolicy,omitempty"`
  // MaxBackupsLimitBasedGC sets the maximum number of full snapshots to keep.
  MaxBackupsLimitBasedGC *int32 `json:"maxBackupsLimitBasedGC,omitempty"`
  // DeltaSnapshotRetentionPeriod indicates how long to keep delta snapshots (e.g., "72h").
  DeltaSnapshotRetentionPeriod *metav1.Duration `json:"deltaSnapshotRetentionPeriod,omitempty"`
}
```

**Sample YAML**:

```yaml
spec:
  config:
    schedule: "0 0 * * *"
    garbageCollectionConfig:
      garbageCollectionPolicy: "LimitBased"
      maxBackupsLimitBasedGC: 5
      deltaSnapshotRetentionPeriod: "72h"
```

## Compatibility

These changes are compatible with existing etcd clusters and current backup processes.

- **Backward Compatibility:**
  - Clusters without immutable buckets continue to function without any changes.
- **Forward Compatibility:**
  - Clusters can opt in to use immutable backups by configuring the bucket accordingly (as described in [Configuring Immutable Backups](#configuring-immutable-backups)) and setting `etcd.spec.backup.store.immutability.retentionType == "Bucket"`.
  - The enhanced hibernation logic in the etcd controller is additive, meaning it does not interfere with existing workflows.

### Impact for Operators

In scenarios where you want to exclude certain snapshots from an etcd restore, you previously could simply delete them from object storage. However, when bucket-level immutability is enabled, deleting existing immutable snapshots is no longer possible. To address this need, most cloud providers allow adding **custom annotations or tags** to objects—even immutable ones—so they can be logically excluded without physically removing them.

`etcd-backup-restore` supports ignoring snapshots based on annotations or tags, rather than deleting them. Operators can add the following key-value pair to any snapshot object to exclude it from future restores:

- **Key:** `x-etcd-snapshot-exclude`  
- **Value:** `true`  

Because these tags or annotations do not modify the underlying snapshot data, they are permissible even for immutable objects. Once these annotations are in place, `etcd-backup-restore` will detect them and skip the tagged snapshots during restoration, thus preventing unwanted snapshots from being used. For more details, see the [Ignoring Snapshots during Restoration](https://github.com/gardener/etcd-backup-restore/blob/master/docs/usage/enabling_immutable_snapshots.md#ignoring-snapshots-during-restoration).

## References

- [GCS Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)  
- [AWS S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)  
- [Azure Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)  
- [etcd-backup-restore Documentation](https://github.com/gardener/etcd-backup-restore/blob/master/README.md)  
- [Gardener Issue: 10866](https://github.com/gardener/gardener/issues/10866)  

---
