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

This proposal introduces immutable backups for etcd clusters managed by `etcd-druid`. By leveraging cloud provider immutability features, backups taken by `etcd-backup-restore` can neither be modified nor deleted once created for a configurable retention duration (immutability period). This approach strengthens the reliability and fault tolerance of the etcd restoration process.

## Terminology

- **etcd-druid:** An etcd operator that configures, provisions, reconciles, and monitors etcd clusters.
- **etcd-backup-restore:** A sidecar container that manages backups and restores of etcd cluster state. For more information, see the [etcd-backup-restore](https://github.com/gardener/etcd-backup-restore/blob/master/README.md) documentation.
- **WORM (Write Once, Read Many):** A storage model in which data, once written, cannot be modified or deleted until certain conditions are met.
- **Immutability:** The property of an object that prevents it from being modified or deleted after creation.
- **Immutability Period:** The duration for which data must remain immutable before it can be modified or deleted.
- **Bucket-Level Immutability:** A policy that applies a uniform immutability period to all objects within a bucket.
- **Object-Level Immutability:** A policy that allows setting immutability periods individually for objects within a bucket, offering more granular control.
- **Garbage Collection:** The process of deleting old snapshot data that is no longer needed, in order to free up storage space. For more information, see the [garbage collection documentation](https://github.com/gardener/etcd-backup-restore/blob/master/docs/usage/garbage_collection.md).
- **Hibernation:** A state in which an etcd cluster is scaled down to zero replicas, effectively pausing its operations. This is typically done to save costs when the cluster is not needed for an extended period. During hibernation, the cluster's data remains intact, and it can be resumed to its previous state when required.

## Motivation

`etcd-druid` provisions etcd clusters and manages their lifecycle. For every etcd cluster, consumers can enable periodic backups of the cluster state by configuring the `spec.backup` section in an Etcd custom resource. Periodic backups are taken via the `etcd-backup-restore` sidecar container that runs in each etcd member pod.

Periodic backups of an etcd cluster state ensure the ability to recover from a data loss or a quorum loss, enhancing reliability and fault tolerance. It is crucial that these backups, which are vital for restoring the etcd cluster, remain protected from any form of tampering, whether intentional or accidental. To safeguard the integrity of these backups, the authors recommend utilizing `WORM` protection, a feature offered by various cloud providers, to ensure the backups remain immutable and secure.

### Goals

- Protect backup data against modifications and deletions post-creation through immutability policies offered by storage providers.

### Non-Goals

- Implementing a mechanism to signal hibernation intent, such as adding functionality via `etcd.spec` or annotations on the `Etcd` CR, to indicate when an etcd cluster should enter or exit hibernation, as discussed in [gardener/etcd-druid#922](https://github.com/gardener/etcd-druid/issues/922).
- Supporting immutable backups on storage providers that do not offer immutability features (e.g., OpenStack Swift).

## Proposal

This proposal aims to improve backup storage integrity and security by using immutability features available on major cloud providers.

### Supported Cloud Providers

- **Google Cloud Storage (GCS):** [Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
- **Amazon S3 (S3):** [Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
- **Azure Blob Storage (ABS):** [Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)
> [!NOTE]
> Currently, Openstack object storage (swift) doesn't support immutability for objects: https://blueprints.launchpad.net/swift/+spec/immutability-middleware.


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
| Support for enabling bucket-level immutability in existing buckets        | Yes                     | Yes          | Yes                                 |
| Support for enabling bucket-level immutability in new buckets             | Yes                     | Yes          | Yes                                 |
| Precedence between bucket-level and object-level immutability periods     | Max(bucket, object)     | Object-level | Max(bucket, object)                 |

> [!NOTE]
> In AWS S3, it is possible to increase and decrease the bucket-level immutability period; however, this action can be blocked by configuring specific bucket policy settings.  
> For GCS, object-level immutability is not yet supported for existing buckets; see [this issue](https://issuetracker.google.com/issues/346679415?pli=1).

### Recommended Approach

At the time of writing this proposal, these are the current limitations seen across providers:

- **S3 and ABS:** typically require bucket-level immutability as a prerequisite for object-level immutability.
- **GCS** does not currently support object-level immutability in existing buckets.
- **ABS** requires a [migration process](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-version-level-worm-policies#migration) to enable version-level immutability on existing containers.

Consequently, the authors recommend **bucket-level immutability**. This approach simplifies configuration and ensures a uniform immutability policy for all backups in a bucket across all support providers.

### Configuring Immutable Backups

Creating and configuring immutable buckets on providers is not handled by `etcd-druid` and must be done by the consumers. For a large-scale consumer like Gardener, provider extensions are leveraged to automate both the creation and configuration of buckets. For more details, see BackupBucket and refer to [this issue](https://github.com/gardener/gardener/issues/10866).


#### Prerequisites

1. **Configure or Update the Immutable Bucket**  
   - Use your cloud provider’s CLI, SDK, or console to create (or update) a bucket/container with a WORM (write-once-read-many) immutability policy.  
   - Refer to the [Configure Bucket-Level Immutability](https://github.com/gardener/etcd-backup-restore/blob/master/docs/usage/enabling_immutable_snapshots.md#configure-bucket-level-immutability) for step-by-step instructions on configuring or updating the immutable bucket across different cloud providers.

2. **Provide Valid Credentials in a Kubernetes Secret**  
  - The `store` section of the `Etcd` CR must reference a `Secret` containing valid credentials.  
    ```yaml
    apiVersion: druid.gardener.cloud/v1alpha1
    kind: Etcd
    metadata:
     name: example-etcd
    spec:
     backup:
      store:
        prefix: etcd-backups
        container: my-immutable-backups  # Bucket name
        provider: aws                    # Supported: aws, gcp, azure
        secretRef:
         name: etcd-backup-credentials  # Reference to the Secret
        immutability:
         retentionType: bucket          # Enables bucket-level immutability
    ```
   - Confirm that this secret has the proper permissions to upload and retrieve snapshots from the immutable bucket.  
   - See the [Getting Started guide](https://github.com/gardener/etcd-druid/blob/master/docs/deployment/getting-started-locally/getting-started-locally.md#setting-up-cloud-provider-object-store-secret) for an example.
> [!NOTE]
> The `etcd-druid` does not handle the rotation of cloud provider credentials. Credential rotation must be managed by the operator.

By following these steps, you will have set up an immutable bucket for storing etcd backups, along with the necessary references in your `Etcd` specification and Kubernetes secret.

### Handling of Hibernated Clusters

When an etcd cluster remains hibernated beyond the bucket’s immutability period, backups might become mutable again, depending on the cloud provider (see [Comparison of Storage Provider Properties](#comparison-of-bucket-level-and-object-level-immutability)). This could compromise the intended guarantees of immutability, exposing backups to accidental or malicious alterations.

As mentioned in [gardener/etcd-druid#922](https://github.com/gardener/etcd-druid/issues/922), a clear hibernation signal is needed. Since `etcd-druid` does not currently support hibernation natively and addressing that is out of scope for this proposal, we focus solely on maintaining immutability.

#### Proposal  

To mitigate the risk of backups becoming mutable during extended hibernation under bucket-level immutability, the authors propose the following approach:

1. **Prerequisite: Cut-off Traffic and Take a Final Full Snapshot Before Hibernation**  
   - Before scaling the etcd cluster down to zero replicas, the etcd controller removes etcd’s client ports (2379/2380) from the etcd Service to block application traffic.
   - The etcd controller then triggers an [on-demand full snapshot](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/05-etcd-operator-tasks.md#trigger-on-demand-fulldelta-snapshot). This ensures that the latest state of the etcd cluster is captured and securely stored before hibernation begins.  

2. **Periodically Re-Upload the Snapshot**  
   - Re-uploading the latest full snapshot resets its immutability period in the bucket, ensuring backups remain protected during hibernation.  
   - By default, the re-upload schedule follows `etcd.spec.backup.fullSnapshotSchedule`. Currently, this interval cannot be customized exclusively for re-uploads; future enhancements may introduce a dedicated configuration parameter.  
   - A new operator task type, **`ExtendFullSnapshotImmutabilityTask`**, periodically calls a new CLI command, `extend-snapshot-immutability`, to re-upload the snapshot and extend its immutability.  
   - **`ExtendFullSnapshotImmutabilityTask` also manages garbage collection**, ensuring that **only the latest immutable snapshots are retained** while deleting older snapshots created by the task itself.

By capturing a final full snapshot before hibernation, periodically re-uploading it to preserve immutability, and removing stale backups, etcd backups remain safeguarded against accidental or malicious alterations until the cluster is resumed.

> [!IMPORTANT]
> **Limitation:** There is a potential edge case where a snapshot might become corrupted **before hibernation** or during the **re-upload process by `ExtendFullSnapshotImmutabilityTask`**. If this happens, the process could repeatedly re-upload the same corrupted snapshot, failing to ensure a reliable backup.

An alternative solution could be to perform [compaction](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md), which involves:

1. Starting an embedded etcd instance from the latest full snapshot in the snapstore.
2. Performing an etcd compaction operation to remove old revisions and free up space.
3. Taking a fresh snapshot of the compacted data and re-uploading it to the snapstore.

While this approach ensures that only valid snapshots are re-uploaded, it is **resource-intensive**, requiring an operational etcd instance even in hibernation. Given the high cost in terms of compute and memory, the authors **recommend** the snapshot re-upload approach as a more practical solution.

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
    // RetentionType indicates the type of immutability approach. For example, "bucket".
    RetentionType string `json:"retentionType,omitempty"`
}
```

If `immutability` is not specified, `etcd-druid` will assume that the bucket is mutable, and no immutability-related logic applies. We have defined a new type to allow us future enhancements to the immutability specification.

##### `etcd-backup-restore` Enhancements

The authors propose adding new sub-command to the `etcd-backup-restore` CLI (`etcdbrctl`) to maintain immutability during hibernation and to clean up snapshots created by `ExtendFullSnapshotImmutabilityTask`:

- **`extend-snapshot-immutability`**
  - Downloads the latest full snapshot from the object store.
  - Renames the snapshot (for instance, updates its Unix timestamp) to avoid overwriting an existing immutable snapshot.
  - Uploads the renamed snapshot back to object storage, thereby **restarting** its immutability timer.  
  - Introduces the `--gc-from-timestamp=<timestamp>` parameter, allowing users to specify a starting point from which garbage collection should be performed.
>[!NOTE]  
>As an alternative to the download/upload approach, the authors document the possibility of using provider APIs to perform a server-side object copy. This method could significantly reduce network costs and latency by directly copying the snapshot within the cloud provider's infrastructure. While this option is not implemented in the current version, let's explore its feasibility for adoption in etcd-backup-restore to enable server-side copying in [snapstore](https://github.com/gardener/etcd-backup-restore/blob/master/pkg/types/snapstore.go#L74-L86) during implementation.

##### etcd Controller Enhancements

When a hibernation flow is initiated (by external tooling or higher-level operators), the [etcd controller](https://github.com/gardener/etcd-druid/blob/master/docs/development/controllers.md#etcd-controller) can:

1. Remove etcd’s client ports (2379/2380) from the etcd Service to block application traffic.
2. Trigger an [on-demand full snapshot](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/05-etcd-operator-tasks.md#trigger-on-demand-fulldelta-snapshot) via an `EtcdOperatorTask`.
3. Scale down the `StatefulSet` replicas to zero, provided the previous snapshot step is successful.
4. Create the `ExtendFullSnapshotImmutabilityTask` if `etcd.spec.backup.store.immutability.retentionType` is `"bucket"` and based on `etcd.spec.backup.fullSnapshotSchedule`.

##### Operator Task Enhancements

The `ExtendFullSnapshotImmutabilityTask` will create a cron job that:

- Runs `etcdbrctl extend-snapshot-immutability --gc-from-timestamp=<creation timestamp of task>` to preserve the immutability period of the most recent snapshot. This command re-uploads the latest snapshot, effectively resetting its immutability period. Additionally, it removes any snapshots that have become mutable after the creation timestamp of the task.

By periodically re-uploading (extending) the latest snapshot during hibernation, the authors ensure that the immutability period is extended, and the backups remain **protected throughout the hibernation period**.

###### Lifecycle of `ExtendFullSnapshotImmutabilityTask`

The `ExtendFullSnapshotImmutabilityTask` is **active during hibernation** and is automatically managed by the `etcd-controller`. Its lifecycle is tied to the cluster’s hibernation state:

1. **Task Creation**  
   - When the etcd cluster enters hibernation (e.g., scaling down to zero replicas), the etcd controller:  
     - Triggers a final full snapshot.  
     - Creates the `ExtendFullSnapshotImmutabilityTask` to run `extend-snapshot-immutability --gc-from-timestamp=<creation timestamp of this task>` 

2. **Task Deletion**  
   - When the cluster resumes from hibernation (scales up to non-zero replicas), the controller:  
     - Deletes the `ExtendFullSnapshotImmutabilityTask` to stop extending snapshots.  
     - Resumes the normal backup schedule defined in `spec.backup.fullSnapshotSchedule`.  

###### Example Task Config

```go
type ExtendFullSnapshotImmutabilityTaskConfig struct {
  // Schedule defines a cron schedule (e.g., "0 0 * * *").
  Schedule *string `json:"schedule,omitempty"`
}
```

**Sample YAML**:

```yaml
spec:
  config:
    schedule: "0 0 * * *"
```

##### Disabling Immutability

If you remove the immutability configuration from `etcd.spec.backup.store`, hibernation-based immutability support no longer applies. However, once the bucket itself is locked at the provider level, **it cannot be reverted to a mutable state**. Any objects uploaded by `etcd-backup-restore` are still subject to the existing WORM policy.

If you genuinely require a mutable backup again, the recommended approach is:
1. **Use a new bucket.** In your `Etcd` custom resource, reference a different bucket that does **not** have immutability enabled.  
2. **Use `EtcdCopyBackupTask`.** If you want to start the cluster with a new bucket but retain old data, use the [`EtcdCopyBackupTask`](https://github.com/gardener/etcd-druid/blob/master/config/samples/druid_v1alpha1_etcdcopybackupstask.yaml) to copy existing backups from the old immutable bucket to the new mutable bucket.
3. **Reconcile the `Etcd` CR.** After pointing `etcd.spec.backup.store` to the new bucket, `etcd-druid` will start storing backups there.

> **Note:** Existing snapshots in the old immutable bucket remain locked according to the configured immutability period.

## Compatibility

These changes are compatible with existing etcd clusters and current backup processes.

- **Backward Compatibility:**
  - Clusters without immutable buckets continue to function without any changes.
- **Forward Compatibility:**
  - Clusters can opt in to use immutable backups by configuring the bucket accordingly (as described in [Configuring Immutable Backups](#configuring-immutable-backups)) and setting `etcd.spec.backup.store.immutability.retentionType == "bucket"`.
  - The enhanced hibernation logic in the etcd controller is additive, meaning it does not interfere with existing workflows.

### Impact for Operators

In scenarios where you want to exclude certain snapshots from an etcd restore, you previously could simply delete them from object storage. However, when bucket-level immutability is enabled, deleting existing immutable snapshots is no longer possible. To address this need, most cloud providers allow adding **custom annotations or tags** to objects—even immutable ones—so they can be logically excluded without physically removing them.

`etcd-backup-restore` supports ignoring snapshots based on annotations or tags, rather than deleting them. Operators can add the following key-value pair to any snapshot object to exclude it from future restores:

- **Key:** `x-etcd-snapshot-exclude`  
- **Value:** `true`  

Because these tags or annotations do not modify the underlying snapshot data, they are permissible even for immutable objects. Once these annotations are in place, `etcd-backup-restore` will detect them and skip the tagged snapshots during restoration, thus preventing unwanted snapshots from being used. For more details, see the [Ignoring Snapshots during Restoration](https://github.com/gardener/etcd-backup-restore/blob/master/docs/usage/enabling_immutable_snapshots.md#ignoring-snapshots-during-restoration).

> [!NOTE]
> At the time of writing this proposal, this feature is not supported for AWS S3 buckets.

## References

- [GCS Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
- [AWS S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
- [Azure Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)
- [etcd-backup-restore Documentation](https://github.com/gardener/etcd-backup-restore/blob/master/README.md)
- [Gardener Issue: 10866](https://github.com/gardener/gardener/issues/10866)

---
