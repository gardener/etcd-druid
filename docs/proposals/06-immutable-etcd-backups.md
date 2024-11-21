---
title: Immutable ETCD Backups
dep-number: 06
creation-date: 2024-09-25
status: implementable
authors:
- "@seshachalam-yv"
- "@renormalize"
- "@ishan16696"
reviewers:
- "@unmarshall"
---

# DEP-06: Immutable ETCD Backups

## Summary

This proposal aims to enhance the reliability and integrity of ETCD backups created by `etcd-backup-restore` in ETCD clusters managed by `etcd-druid`, by introducing immutable backups. By leveraging cloud provider features that support a write-once-read-many (WORM) model, unauthorized modifications to backup data are prevented, ensuring that backups remain intact and accessible for restoration.

The proposed solution relies on `etcd-druid` to manage ETCD backups and handle hibernation processes effectively. It leverages one of the suggested approaches to ensure backups remain immutable over extended periods. It is important to note that using `etcd-backup-restore` standalone may not be sufficient to achieve this functionality end-to-end, as the immutability handling (with respect to hibernation) is specifically managed within `etcd-druid`.

## Motivation

Ensuring the integrity and availability of ETCD backups is crucial for the ability to restore an ETCD cluster when it has become non-functional or inoperable. Making the backups immutable protects against any unintended or malicious modifications post-creation, thereby enhancing the overall security posture.

### Goals

- Implement immutable backup support for ETCD clusters.
- Secure backup data against unintended or unauthorized modifications after creation.
- Implement changes required in `etcd-backup-restore` and `etcd-druid` to support this proposal.

### Non-Goals

- Implementing object-level immutability policies at this stage.
- Supporting immutable backups on storage providers that do not offer immutability features (e.g., OpenStack Swift).

## Proposal

### Overview

We propose introducing immutability in backup storage by leveraging cloud provider features that support a write-once-read-many (WORM) model. This approach will prevent data alterations post-creation, enhancing data integrity and security.

There are two types of immutability options to consider:

1. **Bucket-Level Immutability:** Applies a uniform immutability period to all objects within a bucket. This is widely supported and easier to manage across different cloud providers.
2. **Object-Level Immutability:** Allows setting immutability periods on a per-object basis, offering more granular control but with increased complexity and varying support across providers.

In the detailed design, we will focus on bucket-level immutability policies due to their broader support and simpler management.

### Detailed Design

#### Bucket Immutability Mechanism

The bucket immutability feature configures an immutability policy for a cloud storage bucket, dictating how long objects in the bucket must remain immutable. It also allows for locking the bucket's immutability policy, permanently preventing the policy from being reduced or removed.

- **Supported by Major Providers:**
  - **Google Cloud Storage (GCS):** [Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
  - **Amazon S3 (S3):** [Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
  - **Azure Blob Storage (ABS):** [Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)

- **Not Supported:**
  - **OpenStack Swift**

**Implementation Details:**

- Operators are responsible for configuring new or existing buckets with these immutability settings before `etcd-backup-restore` begins uploading snapshots.
- Once configured, snapshots uploaded by `etcd-backup-restore` will also be immutable and cannot be altered or deleted until the immutability period expires.
- No additional configuration needs to be passed to etcd-druid.

#### ETCD Backup Configuration

Operators must ensure that the ETCD backup configuration aligns with the immutability requirements, including setting appropriate immutability periods.

#### Handling of Hibernated Clusters

When an ETCD cluster is hibernated for a duration exceeding the immutability period, backups may become mutable again (this behavior depends on the cloud provider; refer to [Comparison of Storage Provider Properties](#comparison-of-storage-provider-properties-for-bucket-level-and-object-level-immutability)), compromising the intended immutability guarantees.

Such handling of hibernated clusters is the type of scenario which the etcd operator-tasks frameworks lends itself to quite well, and thus for all proposed solutions, the operator tasks framework as defined [here](./05-etcd-operator-tasks.md) will be made use of for the designs of the solutions.

To maintain snapshot immutability during extended hibernation, we propose two approaches:

##### Approach 1: Using the Compaction Job

**Proposed Solution:**

Utilize the compaction job to periodically take fresh snapshots during hibernation. Introduce a new flag `--hibernation-snapshot-interval` to the compaction controller. This flag sets the interval after which a compaction job should be triggered for a hibernated ETCD cluster, based on the time elapsed since `fullLease.Spec.RenewTime.Time` and if `etcd.spec.replicas` is `0` (indicating hibernation). The compaction job uses the [compact command](https://github.com/gardener/etcd-backup-restore/blob/master/cmd/compact.go) to create a new snapshot.

**Implementation Details:**

- **Etcd Druid:**
  - **Compaction Controller:**
    - Introduce a new flag:
      - **Flag:** `--hibernation-snapshot-interval`
        - **Type:** Duration
        - **Default:** `24h`
        - **Description:** Interval after which a new snapshot is taken during hibernation.
    - The compaction job starts an embedded ETCD instance to take snapshots during hibernation.
    - This approach ensures that backups remain within the immutability period and are safeguarded against becoming mutable.

###### Advantages

- **No Change to ETCD Cluster State:** Does not alter the actual ETCD cluster, keeping it in hibernation.
- **Automated Snapshot Creation:** Periodically creates new snapshots to extend immutability by triggering the compaction job.
- **Leverages Existing Mechanism:** Utilizes the compaction job, which is already part of the system.

###### Disadvantages

- **Resource Consumption:** Starting an embedded ETCD instance periodically consumes resources.

##### Approach 2: Re-upload of the latest snapshot

**Proposed Solution:**

A new `EtcdOperatorTask` called `EtcdSnapshotImmutabilityExtension` will be created as defined in the operator tasks framework. This new `EtcdOperatorTask` extends the immutability period by deploying a job which uploads another copy of the latest snapshot to the object store.

A full snapshot is taken before hibernating the ETCD cluster. This is to ensure that no state maintained in the etcd cluster is lost before hibernation.

**Implementation Details:**

- **Introduce the `extend-immutability` command to etcdbrctl**:
  - etcd-backup-restore will be enhanced to support a new command `extend-immutability` which does the following:
    - Downloads the latest full snapshot from the object store.
    - Replaces the Unix epoch in the file-name of the downloaded snapshot to contain the time at which the file completes downloading.
    - Uploads this newly renamed snapshot to the same object store.
    - Renews the full snapshot lease after the upload is successful.

    The immutability period of an object begins from the moment of upload, thus extending the immutability period of the latest snapshot. Renaming the snapshot is necessary since the downloaded snapshot can not simply be re-uploaded as uploading with the same name would be an attempt at modifying an already existing snapshot, which is disallowed.

    This command could either be implemented standalone, or could be implemented as a wrapper over the `copy` command of `etcdbrctl` by extending the functionality of the `copy` command accordingly.
- **Introduce the `garbage-collect` command to etcdbrctl**:
  - etcd-backup-restore will be enhanced to support a new command `garbage-collect` which does the following:
    - Perform garbage collection of the snapshots in the object store according to the policy specified with the `--garbage-collection-policy` flag.  

    This functionality is needed since it would be necessary to garbage collect the (identical final) snapshots that are (re)uploaded in order to ensure that there is always a snapshot which is immutable.
- **Update `Etcd` CRD:**
  - Add `etcd.spec.hibernation`:  
    Since there are situations outside of hibernation where the number of replicas of the statefulset would have to be scaled to zero, there needs to be an explicit way in which it is conveyed to etcd-druid that the etcd cluster is being hibernated. This can be achieved by extending the `Etcd` CRD by including a new field in the `spec` called `hibernated`.

    ```yaml
    hibernation:
      enabled: <bool>
    ```

  - Add `etcd.status.hibernatedAt`:  
    This field conveys information about whether the cluster has been successfully hibernated after a reconciliation, and the time at which it entered hibernation. This field is cleared when the cluster is woken up from hibernation.

    ```yaml
    hibernatedAt: <hibernation-time>
    ```

  - Add `immutableSettings.retentionType` under `etcd.spec.backup.store`.
- **ETCD Controller Logic:**
  - When hibernation is requested, by changing `etcd.spec.hibernated.enabled` to `true`:
    - The controller removes the ETCD client ports `2380` and `2379` from the `etcd-client` service, leaving only the etcd-backup-restore port `8080`. This stops ETCD client traffic.
    - The controller creates a `EtcdOperatorTask` to trigger an on-demand full snapshot.
      - On-demand full snapshot is successful: the controller does no additional handling.
      - On-demand full snapshot fails: the controller triggers the creation of a `EtcdOperatorTask` for an on-demand compaction job that compacts the latest base full snapshot and the corresponding deltas.
    - The controller scales in the ETCD cluster (i.e., sets `StatefulSet.spec.replicas` to zero).
    - The controller creates the `EtcdSnapshotImmutabilityExtension` periodically if `etcd.spec.backup.store.immutableSettings.retentionType` is set to `"Bucket"`.

- **`EtcdSnapshotImmutabilityExtension` specification:**
  - Run `etcdbrctl extend-immutability --bucket-level-immutability` to extend the immutability of the latest snapshot.
  - Run `etcdbrctl garbage-collect --garbage-collection-policy <garbage-collection-policy>` to garbage collect the snapshots that are created during the extension.

- **EtcdOperatorTask Controller Logic:**
  - The operator-tasks controller will react to the creation of the custom resource, and will deploy a job named `<etcd-name>-extend-immutability` which is the `EtcdSnapshotImmutabilityExtension` job.
  - The controller also reports metrics regarding the `EtcdSnapshotImmutabilityExtension` job, which can be used to raise alerts for operators that immutability has not been extended.

###### Advantages

- **Minimal Operational Impact:** Does not alter the ETCD cluster's state during hibernation and respects the operator's intention to hibernate the cluster without unintended changes.
- **Efficient Resource Utilization:** Only the latest snapshot is copied, limiting additional storage usage, and avoids the need to start an embedded ETCD instance.
- **Automated Process:** The process of taking a full snapshot before hibernation and creating the `EtcdSnapshotImmutabilityExtension` is automated within the controller.

###### Disadvantages

- **Additional Complexity:** Requires updates to the etcd controller, introduction of the operator-tasks controller, and introduction of new etcdbrctl commands.
- **Prerequisite Requirement:** Relies on successfully taking a full snapshot before hibernation, which may introduce delays or require handling snapshot failures.

##### Recommendation

After evaluating both approaches, **Approach 2: Re-upload of the latest snapshot** is recommended due to its minimal operational impact and efficient resource utilization. By ensuring that a full snapshot is taken before hibernation, we maintain data consistency and extend the immutability period effectively. This approach respects the operator's intention to keep the ETCD cluster hibernated without introducing significant resource consumption or complexity.

## Compatibility

The proposed changes are fully compatible with existing ETCD clusters and backup processes.

- **Backward Compatibility:**
  - Existing clusters without immutable buckets will continue to function without change.
  - The introduction of the `EtcdSnapshotImmutabilityExtension` does not affect clusters that are not hibernated.
- **Forward Compatibility:**
  - Clusters can opt-in to use immutable backups by configuring the bucket accordingly.
  - The controller's logic to handle hibernation is additive and does not interfere with existing workflows.

## Risks and Mitigations

- **Increased Storage Costs:**

  - **Risk:** Copying snapshots or frequent snapshots may lead to increased storage usage.
  - **Mitigation:** Since only the latest full snapshot is copied in Approach 2, the additional storage usage is minimal. Garbage collection helps manage storage utilization.

- **Operational Complexity:**

  - **Risk:** Introducing new resources and processes might add complexity.
  - **Mitigation:** The processes are automated within the controller, requiring minimal operator intervention. Clear documentation and tooling support will help manage complexity.

- **Failed Snapshot Before Hibernation:**

  - **Risk:** Failure to take a full snapshot before hibernation could delay the hibernation process.
  - **Mitigation:** Implement robust error handling and retries. Notify operators of failures to take corrective action.

- **Failed Operations:**

  - **Risk:** Errors during copy or snapshot operations could lead to incomplete backups.
  - **Mitigation:** Implement robust error handling and retries in the copier and compaction job logic. Ensure proper logging and alerting.

## Operational Considerations

Operators need to:

- **Configure Buckets:**

  - Set up buckets with appropriate immutability settings before deploying ETCD clusters.
  - Ensure immutability periods align with organizational policies.

- **Monitor Hibernation Processes:**

  - Keep track of hibernated clusters and ensure that full snapshots are taken before hibernation.
  - Verify that `EtcdCopyBackupsTask` resources are created and executed as expected.

- **Review Retention Policies:**

  - Set `maxBackups` and `maxBackupAge` in the `EtcdCopyBackupsTask` to manage storage utilization effectively.
  - Configure the `--hibernation-snapshot-interval` for the compaction job if using Approach 1.

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

**Advantages:**

- **Granular Control:** Allows setting different immutability periods for different objects.
- **Efficient Resource Utilization:** Prevents unnecessary extension of immutability for all objects.
- **Enhanced Flexibility:** Adjust immutability periods as needed.

**Disadvantages:**

- **Provider Limitations:** Enabling object-level immutability on existing buckets is not universally supported.
- **Increased Complexity:** Requires additional logic in backup processes and tooling.
- **Prerequisites:** Some providers require bucket-level immutability to be set first.

#### Conclusion

Given the complexities and limitations, we recommend using bucket-level immutability in conjunction with the `EtcdCopyBackupsTask` approach (Approach 2) to manage immutability during hibernation effectively. This approach provides a balance between operational simplicity and meeting immutability requirements. The compaction job approach (Approach 1) is also viable but may introduce more resource consumption and operational overhead.

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

> **Note:** *In AWS S3, changes to the bucket-level immutability period can be blocked by adding a specific bucket policy.

---

## References

- [GCS Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
- [AWS S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
- [Azure Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)
- [etcd-backup-restore PR #776](https://github.com/gardener/etcd-backup-restore/pull/776)
- [EtcdCopyBackupsTask Implementation](https://github.com/gardener/etcd-druid/pull/544)

## Glossary

- **ETCD:** A distributed key-value store used as the backing store for Kubernetes.
- **Etcd Druid:** A Kubernetes operator that manages ETCD clusters for Gardener.
- **EtcdCopyBackupsTask:** A custom resource that defines a task to copy ETCD backups.
- **Compaction Job:** A process that compacts ETCD snapshots to reduce storage size and improve performance.
- **Hibernation:** Shutting down all the processes that correspond to an etcd cluster, while persisting information which can later be used to restart the etcd cluster with the same state.
- **Immutability Period:** The duration for which data must remain immutable in storage before it can be modified or deleted.
- **WORM (Write Once, Read Many):** A storage model where data, once written, cannot be modified or deleted until certain conditions are met.
- **Immutability:** The property of an object being unchangeable after creation.
- **Garbage Collection:** The process of deleting old or unnecessary data to free up storage space.

---
