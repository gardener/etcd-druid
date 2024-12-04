---
title: Immutable etcd cluster backups
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

# DEP-06: Immutable etcd cluster backups

## Summary

Currently, along with being able to provision etcd clusters and handle cluster lifecycle, `etcd-druid` also enables regular backups of the etcd cluster state to be taken, through the side-car container `etcd-backup-restore` that is deployed in each etcd pod running a member of the etcd cluster.
This functionality is toggled on when `spec.backup` is enabled with appropriate values for an etcd cluster.

All actors (with sufficient privileges) in the cluster where `etcd-druid` is deployed, and the etcd clusters it provisions have access to the `Secret` which holds the credentials that are used to upload snapshots of the etcd cluster state. These credentials are used by actors running in the system, and human operators - typically to perform various maintenance and recovery operations.

To ensure erroneous operations do not occur by human operators during such maintenance and recovery operations, or by misbehaving actors in the cluster, and potentially create the scope for a complete restoration failure which can not be recovered from, the authors propose the usage of write-once-read-many (WORM) features offered by several cloud providers, wherever available.
This WORM model will enhance the reliability and integrity of etcd cluster state backups created by `etcd-backup-restore` in etcd clusters managed and operated by `etcd-druid`; by ensuring that the backups are [*immutable*](#terminology) for a specific period of time from the time they are uploaded, thereby preventing all unintended modifications.

`etcd-druid` and `etcd-backup-restore` will be enhanced to achieve the same functionality that currently is being achieved by modifying/deleting backups, without actually modifying/deleting these backups henceforth as the backups will be immutable for a set duration, thereby eliminating the possibility of a potential loss of data. `etcd-druid` will be the end-to-end solution for achieving this functionality, as relying just on `etcd-backup-restore` for such behavior will not be sufficient given the scope and all possible approaches to achieving this.

## Terminology

- **WORM (Write Once, Read Many):** A storage model where data, once written, cannot be modified or deleted until certain conditions are met.
- **Immutability:** The property of an object being unmodifiable after creation.
- **Immutability Period:** The duration for which data must remain immutable in object storage before it can be modified or deleted.
- **Garbage Collection:** The process of deleting old or unnecessary snapshot data to free up storage space.

## Motivation

Backups are stored in object storage, which are accessible to both `etcd-backup-restore`, and human operators of these clusters with access to these credentials stored in `Secret`s.
This however, is a double-edged sword. On one hand, it offers operators the capability to intervene in situations where restoration of the etcd cluster fails due to a multitude of reasons, like a potential bug in the side-car `etcd-backup-restore`, an unlikely bug in etcd's Snapshot API, and so on.
There have been instances previously where such human operator intervention was necessary, as reported in https://github.com/gardener/etcd-backup-restore/issues/763. Such situations can be resolved by human operators through manual intervention, by either modifying or deleting erroneous snapshots.

Manual intervention is quite helpful in cases where restoration fails, but there is *glaring flaw* with this method - operators have full access to all backups: `GET`, `PUT`, and `DELETE` calls.
This leaves backups vulnerable to potential erroneous operations from human operators, which could lead to a disastrous loss of backup data, which can not be recovered from.

Ensuring the integrity and availability of etcd cluster state backups is crucial for the ability to restore an etcd cluster when it has become non-functional or inoperable. Making the backups immutable protects against any unintended or malicious modifications post-creation, thereby enhancing the overall security posture.

### Goals

- Secure backup data against unintended modifications after creation through bucket-level immutability policies with the storage providers that support such features.
- Ensure a one-to-one map of functionality exists for recovery operations in special circumstances which require human operator intervention, where recovery involved direct manipulation of data in the object store.

### Non-Goals

- Secure backup data through object-level immutability policies.
- Supporting immutable backups on storage providers that do not offer immutability features (e.g., OpenStack Swift).

## Proposal

### Overview

The authors propose introducing immutability in backup storage by leveraging cloud provider features that support a write-once-read-many (WORM) model. This approach will prevent data alterations post-creation, enhancing data integrity and security.

There are two types of immutability options to consider:

1. **Bucket-Level Immutability:** Applies a uniform immutability period to all objects within a bucket. This is widely supported and easier to manage across different cloud providers.
2. **Object-Level Immutability:** Applies a non-uniform immutability period to the objects in the bucket, allowing setting immutability periods on a per-object basis, offering more granular control but with increased complexity and varying support across providers.

Bucket-level immutability policies will be focused on in this proposal due to their broader support and simpler management, as mentioned in the [Non-Goals](#non-goals).

### Configuring Immutable Backups

The bucket immutability feature configures an immutability policy for a cloud storage bucket, dictating how long objects in the bucket must remain immutable. It also allows for locking the bucket's immutability policy, permanently preventing the policy from being reduced or removed.

- **Supported by Major Providers:**
  - **Google Cloud Storage (GCS):** [Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
  - **Amazon S3 (S3):** [Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
  - **Azure Blob Storage (ABS):** [Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)

- **Not Supported:**
  - **OpenStack Swift**

To configure immutability with your etcd cluster backups, there will be no changes needed to be performed in your etcd `spec`. `etcd-backup-restore` is designed to handle immutable backup buckets inherently.
It is the responsibility of controllers/operators for configuring existing/new buckets with the necessary immutability settings for `etcd-backup-restore`'s snapshots to be immutable.

Immutable buckets are configured in the following way for each type of consumer of `etcd-druid`:

- For consumers using `gardener-extension-provider-<provider>` to configure their buckets, the following fields have to be added in the `backup.providerConfig` section of your `extensions.gardener.cloud` `BackupBucket` resource.

  ```yaml
  backup:
    providerConfig:
      immutability:
        retentionType: "bucket"
        retentionPeriod: "<time>"
        locked: false|true
  ```

  The extension will act on the changes in the spec of the `BackupBucket` resource in the next reconciliation, and make the corresponding API calls to the cloud provider to make the `BackupBucket` immutable. Once this succeeds, your backups are now immutable and `etcd-backup-restore` reacts to this without any configuration changes or restarts.

- For consumers using `etcd-druid` standalone, the necessary API calls has to be made to make your buckets immutable, either by your controllers which provision and handle infrastructure like buckets, or if the buckets are provisioned by human operators, then the operators can use the corresponding cloud provider CLIs to run the necessary commands to make the buckets immutable. Please check `etcd-backup-restore` [docs](https://github.com/gardener/etcd-backup-restore/blob/master/docs/usage/immutable_snapshots.md) to find example CLI commands for the currently supported providers.

### etcd Backup Configuration

Before configuring backup buckets to be immutable, it is the responsibility of the operators to configure the snapshot and snapshot garbage collection schedules to be meaningful with the context of the immutability duration of the bucket.

It is recommended that your full snapshot schedule enables the triggering of a full snapshot before the previous full snapshot's immutability period expires. This is to ensure that all the corresponding delta snapshots triggered on top of this full snapshot are ensured to use an intact full snapshot.

It is recommended to configure your snapshot garbage collection policies to begin garbage collection only after the bucket immutability period, to avoid unnecessary API calls to the cloud provider.

### Hibernation

A new state for an etcd cluster will be introduced, called `Hibernated`. This state is inspired from a Gardener [Shoot's Hibernation](https://github.com/gardener/gardener/blob/master/docs/usage/shoot/shoot_hibernate.md).
An etcd cluster is hibernated when the intent is for the etcd cluster to stop serving traffic for the foreseeable future.

Since `etcd-druid` enables hosted control planes, if the intent is to bring down the control plane of a cluster completely, then the corresponding etcd cluster is also to be brought down. In such cases, the etcd cluster can be `Hibernated`.

An explicit effort is made to differentiate between hibernating an etcd cluster and scaling-in the number of the replicas of the `StatefulSet` to zero. The replicas being scaled to zero *does not* mean that the cluster is hibernated. Therefore, to make it clear to all entities interacting with the etcd cluster, i.e. `etcd-druid` and human operators, the new field is introduced.

To enable hibernating etcd clusters by `etcd-druid`, the following fields are proposed to be added to the `spec` and `status` of an etcd cluster respectively:

- `spec.hibernation`:

  ```yaml
  spec:
    hibernation:
      enabled: <bool>
  ```

  When the `spec` of the etcd cluster is changed to contain `spec.hibernation.enabled: true`, in the next reconciliation, `etcd-druid` will stop traffic to be served from the etcd cluster by removing the corresponding client `Service`s, perform the necessary maintenance operations, and then scale-in the cluster.  
  Similarly, when the `spec` has `spec.hibernation.enabled: true` removed, or set to `spec.hibernation.enabled: false`, the next reconciliation will scale-out the etcd cluster to `spec.replicas`.

- `status.hibernationTime`:

  ```yaml
  status:
    hibernationTime: <hibernation-UTC-time>
  ```

  This field conveys information about whether the cluster has been successfully hibernated after a reconciliation, and the time at which it entered hibernation. This field is cleared when the cluster is woken up from hibernation.

The following are the implications:

- Gardener consumers: When a Gardener Shoot cluster is hibernated, then the corresponding etcd cluster is also hibernated by `etcd-druid`, and vice versa.
- Standalone: When an etcd cluster is to be hibernated, the spec of the etcd is to be changed by an operator.

### Handling of Hibernated Clusters

When an etcd cluster is hibernated for a duration exceeding the duration for which a backup is immutable, backups may become mutable again (this behavior depends on the cloud provider; refer to [Comparison of Storage Provider Properties](#comparison-of-storage-provider-properties-for-bucket-level-and-object-level-immutability)), compromising the intended immutability guarantees.

Such handling of hibernated clusters is the type of scenario which the [etcd operator-tasks](./05-etcd-operator-tasks.md) framework lends itself to quite well, and thus for all proposed solutions, the operator tasks framework will be made use of for the design of the solution.

To maintain snapshot immutability during extended hibernation, the authors propose two approaches:

#### Approach 1: Using the Compaction Job

**Proposed Solution:**

Utilize the compaction job to periodically take fresh snapshots during hibernation. Introduce a new flag `--hibernation-snapshot-interval` to the compaction controller. This flag sets the interval after which a compaction job should be triggered for a hibernated etcd cluster, based on the time elapsed since `fullLease.Spec.RenewTime.Time` and if `etcd.spec.replicas` is `0` (indicating hibernation). The compaction job uses the [compact command](https://github.com/gardener/etcd-backup-restore/blob/master/cmd/compact.go) to create a new snapshot.

**Implementation Details:**

- **Compaction Controller:**
  - Introduce a new flag:
    - **Flag:** `--hibernation-snapshot-interval`
      - **Type:** Duration
      - **Default:** `24h`
      - **Description:** Interval after which a new snapshot is taken during hibernation.
  - The compaction job starts an embedded etcd instance to take snapshots during hibernation.

##### Advantages

- **No Change to etcd Cluster State:** Does not alter the actual etcd cluster, keeping it in hibernation.
- **Automated Snapshot Creation:** Periodically creates new snapshots to extend immutability by triggering the compaction job.
- **Leverages Existing Mechanism:** Utilizes the compaction job, which is already part of the system.

##### Disadvantages

- **Resource Consumption:** Starting an embedded etcd instance periodically consumes resources.

#### Approach 2: Re-upload of the latest snapshot

**Proposed Solution:**

A new `EtcdOperatorTask` called `ExtendEtcdSnapshotImmutabilityTask` will be created as defined in the operator tasks framework. This new `EtcdOperatorTask` extends the immutability period by deploying a job which uploads another copy of the latest snapshot to the object store.

A full snapshot is taken before hibernating the etcd cluster. This is to ensure that no state maintained in the etcd cluster is lost before hibernation.

**Implementation Details:**

- **Introduce the `renew-snapshot` command to etcdbrctl**:
  - etcd-backup-restore will be enhanced to support a new command `renew-snapshot` which does the following:
    - - Downloads the latest full snapshot from the object store.
    - Replaces the Unix epoch in the file-name of the downloaded snapshot to contain the time at which the file completes downloading.
    - Uploads this newly renamed snapshot to the same object store.
    - Renews the full snapshot lease after the upload is successful.

    The immutability period of an object begins from the moment of upload, thus renewing the immutability period of the snapshot. Renaming the snapshot is necessary since the downloaded snapshot can not simply be re-uploaded as uploading with the same name would be an attempt at modifying an already existing snapshot, which is disallowed.

    This command could either be implemented standalone, or could be implemented as a wrapper over the `copy` command of `etcdbrctl` by extending the functionality of the `copy` command accordingly.
- **Introduce the `garbage-collect` command to etcdbrctl**:
  - etcd-backup-restore will be enhanced to support a new command `garbage-collect` which does the following:
    - Perform garbage collection of the snapshots in the object store according to the policy specified with the `--garbage-collection-policy` flag.  

    This functionality is needed to garbage collect the old snapshots whose immutability has expired but have been renewed as a fresh snapshot via the approach mentioned above.
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
- **etcd Controller Logic:**
  - When hibernation is requested, by changing `etcd.spec.hibernated.enabled` to `true`:
    - The controller removes the etcd client ports `2380` and `2379` from the `etcd-client` service, leaving only the etcd-backup-restore port `8080`. This stops etcd client traffic.
    - The controller creates a `EtcdOperatorTask` to trigger an on-demand full snapshot.
      - On-demand full snapshot is successful: the controller does no additional handling.
      - On-demand full snapshot fails: the controller triggers the creation of a `EtcdOperatorTask` for an on-demand compaction job that compacts the latest base full snapshot and the corresponding deltas.
    - The controller scales in the etcd cluster (i.e., sets `StatefulSet.spec.replicas` to zero).
    - The controller creates the `ExtendEtcdSnapshotImmutabilityTask` periodically if `etcd.spec.backup.store.immutableSettings.retentionType` is set to `"Bucket"`.

- **`ExtendEtcdSnapshotImmutabilityTask` specification:**
  - Run `etcdbrctl renew-snapshot` to extend the immutability of the latest snapshot.
  - Run `etcdbrctl garbage-collect --garbage-collection-policy <garbage-collection-policy>` to garbage collect the snapshots that are created during the extension.

- **EtcdOperatorTask Controller Logic:**
  - The operator-tasks controller will react to the creation of the custom resource, and will deploy a job named `<etcd-name>-extend-immutability` which is the `ExtendEtcdSnapshotImmutabilityTask` job.
  - The controller also reports metrics regarding the `ExtendEtcdSnapshotImmutabilityTask` job, which can be used to raise alerts for operators that immutability has not been extended.

##### Advantages

- **Minimal Operational Impact:** Does not alter the etcd cluster's state during hibernation and respects the operator's intention to hibernate the cluster without unintended changes.
- **Efficient Resource Utilization:** Only the latest snapshot is copied, limiting additional storage usage, and avoids the need to start an embedded etcd instance.
- **Automated Process:** The process of taking a full snapshot before hibernation and creating the `ExtendEtcdSnapshotImmutabilityTask` is automated within the controller.

##### Disadvantages

- **Additional Complexity:** Requires updates to the etcd controller, introduction of the operator-tasks controller, and introduction of new etcdbrctl commands.
- **Prerequisite Requirement:** Relies on successfully taking a full snapshot before hibernation, which may introduce delays or require handling snapshot failures.

#### Recommendation

After evaluating both approaches, **Approach 2: Re-upload of the latest snapshot** is recommended due to its minimal operational impact and efficient resource utilization. By ensuring that a full snapshot is taken before hibernation, data consistency is maintained, and the immutability period is extended effectively. This approach respects the operator's intention to keep the etcd cluster hibernated without introducing significant resource consumption or complexity.

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

  - Set up buckets with appropriate immutability settings before deploying etcd clusters.
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

Given the complexities and limitations, the authors recommend using bucket-level immutability in conjunction with the `EtcdCopyBackupsTask` approach (Approach 2) to manage immutability during hibernation effectively. This approach provides a balance between operational simplicity and meeting immutability requirements. The compaction job approach (Approach 1) is also viable but may introduce more resource consumption and operational overhead.

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

---
