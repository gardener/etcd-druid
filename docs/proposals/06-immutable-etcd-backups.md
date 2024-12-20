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

Currently, `etcd-druid` can provision etcd clusters and manage their lifecycle. Additionally, it enables regular backups of the etcd cluster state through the sidecar container `etcd-backup-restore`, which is deployed in each etcd pod running a member of the etcd cluster. This functionality is activated when `spec.backup` is enabled with appropriate values for an etcd cluster.

All actors (with sufficient privileges) in the cluster where `etcd-druid` is deployed, and in the etcd clusters it provisions, have access to the `Secret` that holds the credentials used to upload snapshots of the etcd cluster state. These credentials are used by system actors and human operators—typically to perform various maintenance and recovery operations.

To prevent erroneous operations by human operators during maintenance and recovery, or by misbehaving actors in the cluster - which could potentially lead to an unrecoverable restoration failure, the authors propose using write-once-read-many ([WORM](#terminology)) features offered by various cloud providers where available.

This [WORM](#terminology) model will enhance the reliability and integrity of etcd cluster state backups created by `etcd-backup-restore` in etcd clusters managed and operated by `etcd-druid` by ensuring that the backups are [*immutable*](#terminology) for a specific period from the time they are uploaded, thereby preventing any unintended modifications.

`etcd-druid` and `etcd-backup-restore` will be enhanced to achieve the same functionality currently achieved by modifying or deleting backups, but without actually modifying or deleting these backups, since they will now be immutable for a set duration. This approach eliminates the possibility of potential data loss. `etcd-druid` will provide an end-to-end solution for achieving this functionality, as relying solely on `etcd-backup-restore` is insufficient given the scope and possible approaches to achieving this.

Additionally, handling [hibernation](#terminology) for immutable backups presents a unique challenge. When an `etcd` cluster is hibernated for a duration exceeding the immutability period of its backups, the backups may become mutable again, compromising the intended immutability guarantees and exposing the backups to accidental or malicious modifications. To address this, the authors propose a solution to maintain the immutability of snapshots during extended [hibernation](#terminology) periods.

## Terminology

- **etcd-druid:** A controller that manages etcd clusters, including provisioning and lifecycle handling.
- **etcd-backup-restore:** A sidecar container that manages backups and restores of etcd cluster state.
- **WORM (Write Once, Read Many):** A storage model where data, once written, cannot be modified or deleted until certain conditions are met.
- **Immutability:** The property of an object being unmodifiable after creation.
- **Immutability Period:** The duration for which data must remain immutable in object storage before it can be modified or deleted.
- **Bucket-Level Immutability:** A policy that applies a uniform immutability period to all objects within a bucket.
- **Object-Level Immutability:** A policy that allows setting non-uniform immutability periods for individual objects within a bucket, providing more granular control.
- **Garbage Collection:** The process of deleting old or unnecessary snapshot data to free up storage space.
- **Hibernation:** The state in which an etcd cluster is scaled down to zero replicas, effectively pausing its operations. This is typically done to save resources when the cluster is not needed for an extended period. During hibernation, the cluster's data remains intact, and it can be resumed to its previous state when required.

## Motivation

Backups are stored in object storage, which is accessible to both `etcd-backup-restore` and human operators of these clusters who have access to the credentials stored in `Secret`s. This, however, is a double-edged sword. On one hand, it offers operators the capability to intervene in situations where restoration of the etcd cluster fails due to a multitude of reasons, like a potential bug in the sidecar `etcd-backup-restore`, an unlikely bug in etcd's Snapshot API, and so on.

There have been instances previously where such human operator intervention was necessary, as reported in [gardener/etcd-backup-restore#763](https://github.com/gardener/etcd-backup-restore/issues/763). Such situations can be resolved by human operators through manual intervention, by either modifying or deleting erroneous snapshots.

Manual intervention is quite helpful in cases where restoration fails, but there is a *glaring flaw* with this method—operators have full access to all backups: `GET`, `PUT`, and `DELETE` calls. This leaves backups vulnerable to potential erroneous operations from human operators, which could lead to a disastrous loss of backup data, which cannot be recovered from.

Ensuring the integrity and availability of etcd cluster state backups is crucial for the ability to restore an etcd cluster when it has become non-functional or inoperable. Making the backups immutable protects against any unintended or malicious modifications post-creation, thereby enhancing the overall security posture.

### Goals

- Secure backup data against unintended modifications after creation through bucket-level immutability policies with the storage providers that support such features.
- Ensure a one-to-one mapping of functionality exists for recovery operations in special circumstances which require human operator intervention, where recovery involved direct manipulation of data in the object store.

### Non-Goals

- Implementing the hibernation support via `etcd.spec` or annotations on the `Etcd` CR  (i.e., specifying an intent for hibernation) as mentioned in [gardener/etcd-druid#922](https://github.com/gardener/etcd-druid/issues/922).
- Securing backup data through object-level immutability policies.
- Supporting immutable backups on storage providers that do not offer immutability features (e.g., OpenStack Swift).

## Proposal

### Overview

The authors propose introducing immutability in backup storage by leveraging cloud provider features that support a write-once-read-many (WORM) model. This approach will prevent data alterations post-creation, enhancing data integrity and security.

There are two types of immutability options to consider:

1. **Bucket-Level Immutability:** Applies a uniform immutability period to all objects within a bucket. This is widely supported and easier to manage across different cloud providers.
2. **Object-Level Immutability:** Applies a non-uniform immutability period to the objects in the bucket, allowing setting immutability periods on a per-object basis, offering more granular control but with increased complexity and varying support across providers.

Bucket-level immutability policies will be the focus of this proposal due to their broader support and simpler management, as mentioned in the [Non-Goals](#non-goals).

### Configuring Immutable Backups

The bucket immutability feature configures an immutability policy for a cloud storage bucket, dictating how long objects in the bucket must remain immutable. It also allows for locking the bucket's immutability policy, permanently preventing the policy from being reduced or removed.

- **Supported by Major Providers:**

  - **Google Cloud Storage (GCS):** [Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
  - **Amazon S3 (S3):** [Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
  - **Azure Blob Storage (ABS):** [Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)

- **Not Supported:**

  - **OpenStack Swift**

Currently, configuring immutable buckets is not handled directly within `etcd-druid`.

By configuring an immutability policy on your storage bucket/container, you ensure that all snapshots are stored in an immutable (WORM) state for a specified duration. This prevents snapshots from being modified or deleted until they reach the end of the immutability period.

This immutability policy can be set for a specific duration, ensuring that snapshots are not altered during this period, providing a safeguard against accidental or malicious modifications. The immutability policy configuration can be applied via cloud provider consoles, APIs, or CLI tools. Once the policy is set, it becomes an inherent protection mechanism for `etcd` backups.

Immutable buckets are configured differently depending on the consumer:

- **Large-Scale Consumers (e.g., Gardener):**
  - Have automated the configuration of immutability for existing or new buckets, as discussed [here](https://github.com/gardener/gardener/issues/10866).
- **Standalone Consumers of `etcd-druid`:**
  - Need to handle the immutability configuration manually using the respective cloud provider's CLI tools or console.

Detailed documentation on configuring the backup bucket for immutable etcd snapshots uploaded by `etcd-backup-restore` can be found [here](https://github.com/gardener/etcd-backup-restore/blob/master/docs/usage/enabling_immutable_snapshots.md).

### Handling of Hibernated Clusters

#### What is Hibernation?

Hibernation typically involves scaling down an etcd cluster or other resources to conserve costs or resources during inactive periods. The specific implementation and processes can vary based on the setup and tools used. However, the primary objective is to pause operations while maintaining the state for future resumption.

When an etcd cluster is hibernated for a duration exceeding the immutability period of its backups, the backups may become mutable again (this behavior depends on the cloud provider; refer to [Comparison of Storage Provider Properties](#comparison-of-storage-provider-properties-for-bucket-level-and-object-level-immutability)). This compromises the intended immutability guarantees and may expose the backups to accidental or malicious modifications.

To address this issue, the authors propose a solution that maintains the immutability of snapshots during extended hibernation periods.

#### Proposed Solution: Re-uploading the Latest Snapshot

The authors propose a mechanism to periodically extend the immutability period of the latest snapshot during hibernation. This is achieved by re-uploading the latest snapshot, which resets the immutability period because the immutability countdown starts from the time of upload.

This handling of hibernated clusters is a scenario where the [etcd operator-tasks](./05-etcd-operator-tasks.md) framework can be effectively utilized. Therefore, the authors will leverage the operator tasks framework in the design of this solution.

**Implementation Details:**

- **Introduce the `renew-snapshot` Command to `etcdbrctl`:**

  - `etcd-backup-restore` will be enhanced to support a new command `renew-snapshot`, which performs the following steps:
    - Downloads the latest full snapshot from the object store.
    - Renames the snapshot by updating the Unix epoch in the filename to reflect the time of the download completion.
    - Uploads the renamed snapshot back to the object store.
    - Updates the full snapshot lease after the successful upload.

  The immutability period of an object begins from the moment of upload, so re-uploading the snapshot renews its immutability period. Renaming the snapshot is necessary because uploading with the same name would be considered an attempt to modify an existing snapshot, which is disallowed under immutability policies.

- **Introduce the `garbage-collect` Command to `etcdbrctl`:**

  - `etcd-backup-restore` will be enhanced with a new command `garbage-collect`, which:
    - Performs garbage collection of snapshots in the object store according to `etcd-backup-restore`'s [garbage collection policy](https://github.com/gardener/etcd-backup-restore/blob/master/docs/usage/garbage_collection.md#gc-policies).

  This functionality is needed to remove old snapshots whose immutability has expired, preventing storage from growing indefinitely.

- **Update the `Etcd` CRD:**

  - Add a new field `immutability.retentionType` under `etcd.spec.backup.store` to specify the type of immutability config.

- **etcd Controller Logic:**

  - When hibernation is requested:
    - The controller removes the etcd client ports `2380` and `2379` from the `etcd-client` service, leaving only the `etcd-backup-restore` port `8080`, effectively stopping etcd client traffic.
    - The controller creates an `EtcdOperatorTask` to trigger an on-demand full snapshot.
    - The controller scales down the etcd cluster by setting `StatefulSet.spec.replicas` to zero.
    - The controller periodically creates the `ExtendEtcdSnapshotImmutabilityTask` if `etcd.spec.backup.store.immutability.retentionType` is set to `"Bucket"` and based on `etcd.spec.backup.fullSnapshotSchedule`.

- **`ExtendEtcdSnapshotImmutabilityTask` Specification:**

  - Runs `etcdbrctl renew-snapshot` to extend the immutability of the latest snapshot.
  - Runs `etcdbrctl garbage-collect --garbage-collection-policy <policy>` to remove old snapshots.

- **EtcdOperatorTask Controller Logic:**

  - The operator-tasks controller reacts to the creation of the custom resource and deploys a job named `<etcd-name>-extend-immutability`, which is the `ExtendEtcdSnapshotImmutabilityTask` job.
  - The controller reports metrics regarding the `ExtendEtcdSnapshotImmutabilityTask` job, which can be used to raise alerts for operators if immutability has not been extended.

By periodically re-uploading the latest snapshot during hibernation, the authors ensure that the immutability period is extended, and the backups remain **protected throughout the hibernation period**.

## Compatibility

The proposed changes are fully compatible with existing etcd clusters and backup processes.

- **Backward Compatibility:**
  - Existing clusters without immutable buckets will continue to function without change.
  - The introduction of the `ExtendEtcdSnapshotImmutabilityTask` does not affect clusters that are not hibernated.
- **Forward Compatibility:**
  - Clusters can opt-in to use immutable backups by configuring the bucket accordingly.
  - The controller's logic to handle hibernation is additive and does not interfere with existing workflows.

## Risks and Mitigations

- **Increased Storage Costs:**
  - **Risk:** Copying snapshots or frequent snapshots may lead to increased storage usage.
  - **Mitigation:** Since only the latest full snapshot is copied, the additional storage usage is minimal. Garbage collection helps manage storage utilization.

- **Operational Complexity:**
  - **Risk:** Introducing new resources and processes might add complexity.
  - **Mitigation:** The processes are automated within the controller, requiring minimal operator intervention. Clear documentation and tooling support will help manage complexity.

- **Failed Snapshot Before Hibernation:**
  - **Risk:** Failure to take a full snapshot before hibernation could delay the hibernation process and potentially compromise data integrity.
    - **Mitigation:** Implement robust error handling and retry mechanisms to ensure snapshots are taken successfully before hibernation. Notify operators of any failures by updating the `etcd.status.lastErrors` and `etcd.status.lastOperation` fields. Additionally, operators can leverage the metrics provided by `ExtendEtcdSnapshotImmutabilityTask`, which follows the [operator task framework](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/05-etcd-operator-tasks.md#metrics), to trigger alerts. This ensures timely intervention and resolution of issues, maintaining the integrity and availability of the etcd cluster state.

## Alternatives

### Object-Level Immutability Policies vs. Bucket-Level Immutability Policies

An alternative to implementing immutability via bucket-level immutability policies is to use object-level immutability policies. Object-level immutability allows for more granular control over the immutability periods of individual objects within a bucket, whereas bucket-level immutability applies a uniform immutability period to all objects in the bucket.

#### Feasibility Study: Immutable Backups on Cloud Providers

Major cloud storage providers such as Google Cloud Storage (GCS), Amazon S3, and Azure Blob Storage (ABS) support both bucket-level and object-level immutability mechanisms to enforce data immutability.

1. **Bucket-Level Immutability Policies:**
   - **Applies Uniformly:** Applies a uniform immutability period to all objects within a bucket.
   - **Immutable Objects:** Once set, objects cannot be modified or deleted until the immutability period expires.
   - **Simplified Management:** Simplifies management by applying the same policy to all objects.

2. **Object-Level Immutability Policies:**
   - **Granular Control:** Allows setting immutability periods on a per-object basis.
   - **Flexible Immutability Durations:** Offers granular control, enabling different immutability durations for individual objects.
   - **Varying Requirements:** Can accommodate varying immutability requirements for different types of backups.

#### Considerations for Object-Level Immutability

Using object-level immutability provides flexibility in scenarios where certain backups require different immutability periods. However, current limitations and complexities make it less practical for immediate implementation.

- Enabling object-level immutability requires bucket-level immutability to be set first (applicable in S3 and ABS). In GCS, the capability to enable object-level immutability on an existing bucket is not available.

**Advantages:**

- **Granular Control:** Allows setting different immutability periods for different objects.
- **Efficient Resource Utilization:** Prevents unnecessary extension of immutability for all objects.
- **Enhanced Flexibility:** Adjust immutability periods as needed.

**Disadvantages:**

- **Provider Limitations:** Enabling object-level immutability on existing buckets is not universally supported.
- **Increased Complexity:** Requires additional logic in backup processes and tooling.
- **Prerequisites:** Some providers require bucket-level immutability to be set first.

#### Conclusion

**Given the complexities and limitations, the authors recommend using bucket-level immutability in conjunction with the `ExtendEtcdSnapshotImmutabilityTask` approach to manage immutability during hibernation effectively. This approach provides a balance between operational simplicity and meeting immutability requirements.**

##### Comparison of Storage Provider Properties for Bucket-Level and Object-Level Immutability

| Feature                                                                   | GCS | AWS | Azure                         |
|---------------------------------------------------------------------------|-----|-----|-------------------------------|
| Can bucket-level immutability period be increased?                        | Yes | Yes*| Yes (only 5 times)            |
| Can bucket-level immutability period be decreased?                        | No  | Yes*| No                            |
| Is bucket-level immutability a prerequisite for object-level immutability?| No  | Yes | Yes (existing buckets)        |
| Can object-level immutability period be increased?                        | Yes | Yes | Yes                           |
| Can object-level immutability period be decreased?                        | No  | No  | No                            |
| Support for enabling object-level immutability in existing buckets        | No  | Yes | Yes                           |
| Support for enabling object-level immutability in new buckets             | Yes | Yes | Yes                           |
| Precedence between bucket-level and object-level immutability periods     | Max(bucket, object)| Object-level| Max(bucket, object) |

> **Note:** In AWS S3, changes to the bucket-level immutability period can be blocked by adding a specific bucket policy.

---

## References

- [GCS Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
- [AWS S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
- [Azure Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)

---
