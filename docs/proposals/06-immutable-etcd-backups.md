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

## Table of Contents

- [DEP-06: Immutable ETCD Backups](#dep-06-immutable-etcd-backups)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [Overview](#overview)
    - [Detailed Design](#detailed-design)
      - [Bucket Immutability Mechanism](#bucket-immutability-mechanism)
      - [ETCD Backup Configuration](#etcd-backup-configuration)
      - [Handling of Hibernated Clusters](#handling-of-hibernated-clusters)
    - [New Compaction Controller Flag](#new-compaction-controller-flag)
    - [Garbage Collection during Compaction for Hibernated ETCD Clusters](#garbage-collection-during-compaction-for-hibernated-etcd-clusters)
    - [Excluding Snapshots Under Specific Circumstances](#excluding-snapshots-under-specific-circumstances)
  - [Compatibility](#compatibility)
  - [Implementation Steps](#implementation-steps)
  - [Risks and Mitigations](#risks-and-mitigations)
  - [Operational Considerations](#operational-considerations)
  - [Alternatives](#alternatives)
    - [Object-Level Immutability Policies vs. Bucket-Level Immutability Policies](#object-level-immutability-policies-vs-bucket-level-immutability-policies)
      - [Feasibility Study: Immutable Backups on Cloud Providers](#feasibility-study-immutable-backups-on-cloud-providers)
      - [Considerations for Object-Level Immutability](#considerations-for-object-level-immutability)
      - [Conclusion](#conclusion)
        - [Comparison of Storage Provider Properties for Bucket-Level and Object-Level Immutability](#comparison-of-storage-provider-properties-for-bucket-level-and-object-level-immutability)
  - [References](#references)
  - [Glossary](#glossary)

## Summary

This proposal aims to enhance the reliability and integrity of ETCD backups in ETCD Druid by introducing immutable backups. By leveraging cloud provider features that support a write-once-read-many (WORM) model, this approach prevents unauthorized modifications to backup data, ensuring that backups remain available and intact for restoration.

## Motivation

Ensuring the integrity and availability of ETCD backups is crucial for the ability to restore an ETCD cluster after it has irrecoverably gone down. Making backups immutable, protects against unintended or malicious modifications post-creation, thereby enhancing the overall security posture.

### Goals

- Implement immutable backup support for ETCD clusters.
- Secure backup data against unintended or unauthorized modifications after creation.
- Ensure backups are consistently available and intact for restoration purposes.

### Non-Goals

- Altering existing backup processes beyond what's necessary for immutability.
- Implementing object-level immutability policies at this stage.
- Supporting immutable backups on storage providers that do not offer immutability features (e.g., OpenStack Swift).

## Proposal

### Overview

Introduce immutability in backup storage by leveraging cloud provider features that support a write-once-read-many (WORM) model. This will prevent data alterations after backup creation, enhancing data integrity and security. The implementation will focus on bucket-level immutability policies, as they are widely supported and easier to manage across different cloud providers.

### Detailed Design

#### Bucket Immutability Mechanism

The Bucket Immutability feature configures an immutability policy for a cloud storage bucket, governing how long objects in the bucket must be retained in an immutable state. It also allows for locking the bucket's immutability policy, permanently preventing the policy from being reduced or removed.

- **Supported by Major Providers:**
  - **Google Cloud Storage (GCS):** [Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
  - **Amazon S3 (S3):** [Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
  - **Azure Blob Storage (ABS):** [Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)

- **Not Supported:**
  - **OpenStack Swift**

**Implementation Details:**

- Operators are responsible for creating the new buckets or updating the existing buckets with these immutability settings before `etcd-backup-restore` starts uploading snapshots.
- Once the bucket is configured to be immutable, snapshots uploaded by `etcd-backup-restore` are also immutable and cannot be altered or deleted until the immutability period expires.
- No additional configuration needs to be passed to etcd-druid.

#### ETCD Backup Configuration

Operators need to ensure that the ETCD backup configuration aligns with the immutability requirements. This includes setting appropriate immutability periods.


#### Handling of Hibernated Clusters

In scenarios where an ETCD cluster has been hibernated (scaled down to zero replicas) for a duration that exceeds the immutability period, backups may become mutable again (behaviour depends on cloud provider [refer](#comparison-of-storage-provider-properties-for-bucket-level-and-object-level-immutability)), compromising the intended immutability guarantees.

There are two ways to maintain the immutability of snapshots:

1. **Extend the Immutability of Snapshots**

   Extending the immutability period at the bucket level to cover the extended hibernation period might seem like a solution. However, this approach is **not feasible** due to several significant drawbacks:

   - **Global Impact on All Objects:** Since bucket-level immutability settings apply to all objects within the bucket, extending the immutability period would affect not only the existing snapshots but also all future snapshots uploaded to the bucket.
   - **Increased Storage Costs:** Prolonging the immutability period means that snapshots cannot be deleted or modified until the extended period expires. This leads to the accumulation of old backups, consuming more storage space and significantly driving up storage costs over time.
   - **Inefficient Resource Utilization:** Retaining all snapshots for a longer period than necessary is an inefficient use of storage resources, especially when only the latest snapshots are typically needed for restoration purposes.

   Given these considerable disadvantages, extending the bucket-level immutability period is impractical for maintaining the immutability of snapshots during extended hibernation periods.

2. **Take a New Snapshot**

   By creating a new snapshot, immutability is retained for the new snapshot without affecting the immutability settings of the entire bucket or other snapshots.

   Approach 2 is feasible and can be achieved in two ways:

   - **Option A:** Bring the ETCD cluster back up by increasing the replicas, take a snapshot, and then hibernate the ETCD by setting the replicas back to zero **without the operator's intention**.
     - **Challenges:**
       - **Unintended State Changes:** Increasing the ETCD replicas to bring the cluster back up modifies the cluster's state without the operator's explicit intention, which could lead to unexpected behavior or conflicts with operational policies.
       - **Operational Overhead:** Automating the process of scaling the ETCD cluster up and down adds complexity and potential risks, especially if the scaling operations interfere with other scheduled tasks or maintenance windows.

   - **Option B:** Start an embedded ETCD instance and take a snapshot.
     - **Advantages:**
       - **Respects Operator Intentions:** Does not alter the state of the actual ETCD cluster, keeping it in hibernation as intended by the operator.
       - **No Impact on Existing Setup:** Avoids modifying the ETCD cluster's replicas, preventing any unintended consequences.
       - **Automated Process:** Can be integrated into existing maintenance jobs without manual intervention.

**Proposed Solution:**

*Option B is recommended* because it respects the operator's intention to keep the ETCD cluster hibernated and avoids the complexities and risks associated with modifying the cluster's state.

Leverage the compaction job to take fresh snapshots periodically during hibernation. This ensures that:

- **Immutability is Maintained:** New snapshots have a fresh immutability period, keeping backups protected.
- **Operator Intentions are Respected:** The ETCD cluster remains in its hibernated state, as per the operator's intention.
- **Storage Costs are Controlled:** Avoids unnecessary extension of immutability for all snapshots, preventing increased storage costs.

### New Compaction Controller Flag

**Flag:** `--hibernation-snapshot-interval`

- **Type:** Duration
- **Default:** `24h`
- **Description:** This flag sets the period after which a compaction job should be triggered for a hibernated ETCD cluster, based on the time since the last renewal of the full snapshot lease. If the time since `fullLease.Spec.RenewTime.Time` exceeds the duration specified by this flag, and `etcd.spec.replicas` is `0` (indicating hibernation), the compaction job will automatically trigger to create a new snapshot. This approach ensures that backups remain within the immutability period and are safeguarded against becoming mutable.

### Garbage Collection during Compaction for Hibernated ETCD Clusters

If an ETCD cluster is hibernated for a long duration, there is a chance that backup storage will accumulate a large number of snapshots. Since the ETCD cluster is not running, the `etcd-backup-restore` component is also not running to perform garbage collection of the snapshots.

To address this, the compaction job should be enhanced to handle garbage collection during hibernation. This ensures that old snapshots are cleaned up appropriately, considering the immutability constraints.

### Excluding Snapshots Under Specific Circumstances

Given that immutable backups cannot be deleted until the immutability period expires, there are scenarios, such as corrupted snapshots or other anomalies, where certain snapshots must be skipped during the restoration process. To facilitate this:

- **Custom Metadata Tags:** Utilize custom metadata to mark specific objects (snapshots) that should be bypassed. To exclude a snapshot from the restoration process, attach custom metadata to it with the key `x-etcd-snapshot-exclude` and value `true`. This method is officially supported, as demonstrated in the [etcd-backup-restore PR](https://github.com/gardener/etcd-backup-restore/pull/776) for storage provider: GCS.

## Compatibility

The proposed changes are fully compatible with existing ETCD clusters and backup processes. Operators are responsible for creating or updating the immutability settings on the backup storage buckets, but no changes are required to the ETCD clusters themselves.

- **Backward Compatibility:** Existing clusters without immutable buckets will continue to function without change.
- **Forward Compatibility:** Clusters can opt-in to use immutable backups by configuring the bucket accordingly.

## Implementation Steps

1. **Enhance the Trigger of Compaction Job:**
     - Modify the compaction job in `etcd-backup-restore` to manage backup processes for hibernated clusters, including snapshot creation and garbage collection, while considering the new immutability constraints.
     - Implement the `--hibernation-snapshot-interval` flag in the compaction controller. Ensure that the compaction job can start an embedded ETCD instance to take snapshots during hibernation.

2. **Update Documentation:**
     - Revise the documentation to reflect the changes and guide operators in effectively using the new immutability features.
     - Provide detailed guidelines on configuring buckets with immutability settings.
     - Document procedures for excluding snapshots when necessary.

## Risks and Mitigations

- **Increased Storage Costs:**

  - **Risk:** Introducing immutability could lead to increased storage costs due to the inability to delete backups before the immutability period ends.
  - **Mitigation:** Operators should carefully configure immutability periods and monitor storage utilization. Garbage collection will help mitigate long-term storage growth.

- **Backup Gaps During Hibernation:**

  - **Risk:** Hibernated clusters might not receive necessary backups, potentially leading to compliance issues.
  - **Mitigation:** The compaction job's enhancement ensures that backups are taken during hibernation at configured intervals.

- **Excluding Critical Snapshots:**

  - **Risk:** Excluding snapshots during restoration might be misused or lead to incomplete data restoration.
  - **Mitigation:** Restrict the ability to tag snapshots for exclusion to authorized personnel. Implement audit logging for actions that tag snapshots.

## Operational Considerations

Operators need to:

- **Bucket Configuration:**

  - Configure buckets with appropriate immutability settings before deploying ETCD clusters.
  - Ensure that the immutability periods align with organizational policies.

- **Compaction Job Configuration:**

  - Set the `--hibernation-snapshot-interval` flag according to the desired snapshot frequency during hibernation.
  - Monitor compaction jobs and logs for any issues.

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

Using object-level immutability provides flexibility in scenarios where certain backups require different immutability periods. For example, in the case of hibernated clusters where the ETCD cluster may not be running and backups are not being updated, object-level immutability allows extending the immutability of the latest snapshots without affecting the immutability of older backups.

**Advantages:**

- **Granular Control:** Allows setting different immutability periods for different objects, accommodating varying requirements.
- **Efficient Resource Utilization:** Prevents unnecessary extension of immutability for all objects, potentially reducing storage costs.
- **Enhanced Flexibility:** Can adjust immutability periods for specific backups as needed.

**Disadvantages:**

- **Provider Limitations:** Not all providers support enabling object-level immutability on existing buckets without additional steps. For instance, in GCS, enabling object-level immutability on existing buckets is currently not supported. This limitation necessitates creating new buckets or waiting for the feature to become available.
- **Prerequisite Requirements:** In some providers, object-level immutability requires bucket-level immutability to be set first (e.g., in Amazon S3 and Azure Blob Storage), adding complexity to the configuration.
- **Increased Complexity:** Managing immutability policies at the object level requires additional logic in backup processes and tooling.

#### Conclusion

While object-level immutability offers greater flexibility and control, current provider limitations and operational complexities make it less practical for immediate implementation. Specifically, the inability to enable object-level immutability on existing buckets in GCS and the prerequisite of bucket-level immutability in some providers are significant factors.

Given these considerations, we propose starting with bucket-level immutability to achieve immediate enhancement of backup immutability with minimal changes to existing processes. This approach allows us to implement immutability features across all providers consistently.

Once provider support for object-level immutability on existing buckets improves and operational complexities are addressed, we can consider adopting object-level immutability in the future to address specific requirements, such as varying immutability periods for different backups.

These are the reasons why we are initially opting for bucket-level immutability, with the possibility of transitioning to object-level immutability when it becomes more feasible.

##### Comparison of Storage Provider Properties for Bucket-Level and Object-Level Immutability


| Feature                                                                 | GCS | AWS | Azure |
|-------------------------------------------------------------------------|-----|-----|-------|
| Can bucket-level immutability period be increased?                      | Yes | Yes* | Yes (only 5 times) |
| Can bucket-level immutability period be decreased?                      | No  | Yes* | No    |
| Is bucket-level immutability a prerequisite for object-level immutability? | No  | Yes | Yes (for existing buckets), No (for new buckets) |
| Can object-level immutability period be increased?                      | Yes | Yes | Yes   |
| Can object-level immutability period be decreased?                      | No  | No  | No    |
| Support for enabling object-level immutability in existing buckets      | No (planned support soon) | Yes (only new objects will have immutability) | Yes (Azure handles the migration) |
| Support for enabling object-level immutability in new buckets           | Yes | Yes | Yes   |
| Precedence between bucket-level and object-level immutability periods   | Maximum of bucket or object-level immutability | Object-level immutability has precedence | Maximum of bucket or object-level immutability |

> **Note:** *In AWS S3, changes to the bucket-level immutability period can be blocked by adding a specific bucket policy.

</details>

## References

- [GCS Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
- [AWS S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
- [Azure Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)
- [etcd-backup-restore PR #776](https://github.com/gardener/etcd-backup-restore/pull/776)

## Glossary

- **ETCD:** A distributed key-value store used as the backing store for Kubernetes.
- **Compaction Job:** A process that compacts ETCD snapshots to reduce storage size and improve performance.
- **Hibernation:** Scaling down a cluster (or ETCD) to zero replicas to save resources.
- **Immutability Period:** The duration for which data must remain immutable in storage before it can be modified or deleted.
- **WORM (Write Once, Read Many):** A storage model where data, once written, cannot be modified or deleted until certain conditions are met.
- **Immutability:** The property of an object being unchangeable after creation.
- **Garbage Collection:** The process of deleting old or unnecessary data to free up storage space.

---