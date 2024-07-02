---
title: operator out-of-band tasks
dep-number: 05
creation-date: 6th Dec'2023
status: implementable
authors:
- "@ishan16696" 
- "@unmarshall"
- "@seshachalam-yv"
reviewers:
- "etcd-druid-maintainers"
---

# DEP-05: Operator Out-of-band Tasks

## Table of Contents

- [DEP-05: Operator Out-of-band Tasks](#dep-05-operator-out-of-band-tasks)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Terminology](#terminology)
  - [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [Custom Resource Golang API](#custom-resource-golang-api)
      - [Spec](#spec)
      - [Status](#status)
    - [Custom Resource YAML API](#custom-resource-yaml-api)
    - [Lifecycle](#lifecycle)
      - [Creation](#creation)
      - [Execution](#execution)
      - [Deletion](#deletion)
    - [Use Cases](#use-cases)
      - [Recovery from permanent quorum loss](#recovery-from-permanent-quorum-loss)
        - [Task Config](#task-config)
        - [Pre-Conditions](#pre-conditions)
      - [Trigger on-demand snapshot compaction](#trigger-on-demand-snapshot-compaction)
        - [Possible scenarios](#possible-scenarios)
        - [Task Config](#task-config-1)
        - [Pre-Conditions](#pre-conditions-1)
      - [Trigger on-demand full/delta snapshot](#trigger-on-demand-fulldelta-snapshot)
        - [Possible scenarios](#possible-scenarios-1)
        - [Task Config](#task-config-2)
        - [Pre-Conditions](#pre-conditions-2)
      - [Trigger on-demand maintenance of etcd cluster](#trigger-on-demand-maintenance-of-etcd-cluster)
        - [Possible Scenarios](#possible-scenarios-2)
        - [Task Config](#task-config-3)
        - [Pre-Conditions](#pre-conditions-3)
      - [Copy Backups Task](#copy-backups-task)
        - [Possible Scenarios](#possible-scenarios-3)
        - [Task Config](#task-config-4)
        - [Pre-Conditions](#pre-conditions-4)
  - [Metrics](#metrics)

## Summary

This DEP proposes an enhancement to `etcd-druid`'s capabilities to handle [out-of-band](#terminology) tasks, which are presently performed manually or invoked programmatically via suboptimal APIs. The document proposes the establishment of a unified interface by defining a well-structured API to harmonize the initiation of any `out-of-band` task, monitor its status, and simplify the process of adding new tasks and managing their lifecycles.

## Terminology

* **etcd-druid:** [etcd-druid](https://github.com/gardener/etcd-druid) is an operator to manage the etcd clusters.

* **backup-sidecar:** It is the etcd-backup-restore sidecar container running in each etcd-member pod of etcd cluster.

* **leading-backup-sidecar:** A backup-sidecar that is associated to an etcd leader of an etcd cluster.

* **out-of-band task:** Any on-demand tasks/operations that can be executed on an etcd cluster without modifying the [Etcd custom resource spec](https://github.com/gardener/etcd-druid/blob/9c5f8254e3aeb24c1e3e88d17d8d1de336ce981b/api/v1alpha1/types_etcd.go#L272-L273) (desired state).

## Motivation

Today, [etcd-druid](https://github.com/gardener/etcd-druid) mainly acts as an etcd cluster provisioner (creation, maintenance and deletion). In future, capabilities of etcd-druid will be enhanced via [etcd-member](https://github.com/gardener/etcd-druid/blob/8ac70d512969c2e12e666d923d7d35fdab1e0f8e/docs/proposals/04-etcd-member-custom-resource.md) proposal by providing it access to much more detailed information about each etcd cluster member. While we enhance the reconciliation and monitoring capabilities of etcd-druid, it still lacks the ability to allow users to invoke `out-of-band` tasks on an existing etcd cluster.

There are new learnings while operating etcd clusters at scale. It has been observed that we regularly need capabilities to trigger `out-of-band` tasks which are outside of the purview of a regular etcd reconciliation run. Many of these tasks are multi-step processes, and performing them manually is error-prone, even if an operator follows a well-written step-by-step guide. Thus, there is a need to automate these tasks.
Some examples of an `on-demand/out-of-band` tasks:

* Recover from a permanent quorum loss of etcd cluster.
* Trigger an on-demand full/delta snapshot.
* Trigger an on-demand snapshot compaction.
* Trigger an on-demand maintenance of etcd cluster.
* Copy the backups from one object store to another object store.

## Goals

* Establish a unified interface for operator tasks by defining a single dedicated custom resource for `out-of-band` tasks.
* Define a contract (in terms of prerequisites) which needs to be adhered to by any task implementation.
* Facilitate the easy addition of new `out-of-band` task(s) through this custom resource.
* Provide CLI capabilities to operators, making it easy to invoke supported `out-of-band` tasks.

## Non-Goals

* In the current scope, capability to abort/suspend an `out-of-band` task is not going to be provided. This could be considered as an enhancement based on pull.
* Ordering (by establishing dependency) of `out-of-band` tasks submitted for the same etcd cluster has not been considered in the first increment. In a future version based on how operator tasks are used, we will enhance this proposal and the implementation.

## Proposal

Authors propose creation of a new single dedicated custom resource to represent an `out-of-band` task. Etcd-druid will be enhanced to process the task requests and update its status which can then be tracked/observed.

### Custom Resource Golang API

`EtcdOperatorTask` is the new custom resource that will be introduced. This API will be in `v1alpha1` version and will be subject to change. We will be respecting [Kubernetes Deprecation Policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/).

```go
// EtcdOperatorTask represents an out-of-band operator task resource.
type EtcdOperatorTask struct {
  metav1.TypeMeta
  metav1.ObjectMeta

  // Spec is the specification of the EtcdOperatorTask resource.
  Spec EtcdOperatorTaskSpec `json:"spec"`
  // Status is most recently observed status of the EtcdOperatorTask resource.
  Status EtcdOperatorTaskStatus `json:"status,omitempty"`
}
```

#### Spec

The authors propose that the following fields should be specified in the spec (desired state) of the `EtcdOperatorTask` custom resource.

* To capture the type of `out-of-band` operator task to be performed, `.spec.type` field should be defined. It can have values from all supported `out-of-band` tasks eg. "OnDemandSnaphotTask", "QuorumLossRecoveryTask" etc.
* To capture the configuration specific to each task, a `.spec.config` field should be defined of type `string` as each task can have different input configuration.

```go
// EtcdOperatorTaskSpec is the spec for a EtcdOperatorTask resource.
type EtcdOperatorTaskSpec struct {
  
  // Type specifies the type of out-of-band operator task to be performed. 
  Type string `json:"type"`

  // Config is a task specific configuration.
  Config string `json:"config,omitempty"`

  // TTLSecondsAfterFinished is the time-to-live to garbage collect the 
  // related resource(s) of task once it has been completed.
  // +optional
  TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

  // OwnerEtcdReference refers to the name and namespace of the corresponding 
  // Etcd owner for which the task has been invoked.
  OwnerEtcdRefrence types.NamespacedName `json:"ownerEtcdRefrence"`
}
```

#### Status

The authors propose the following fields for the Status (current state) of the `EtcdOperatorTask` custom resource to monitor the progress of the task.

```go
// EtcdOperatorTaskStatus is the status for a EtcdOperatorTask resource.
type EtcdOperatorTaskStatus struct {
  // ObservedGeneration is the most recent generation observed for the resource.
  ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
  // State is the last known state of the task.
  State TaskState `json:"state"`
  // Time at which the task has moved from "pending" state to any other state.
  InitiatedAt metav1.Time `json:"initiatedAt"`
  // LastError represents the errors when processing the task.
  // +optional
  LastErrors []LastError `json:"lastErrors,omitempty"`
  // Captures the last operation status if task involves many stages.
  // +optional
  LastOperation *LastOperation `json:"lastOperation,omitempty"`
}

type LastOperation struct {
  // Name of the LastOperation.
  Name opsName `json:"name"`
  // Status of the last operation, one of pending, progress, completed, failed.
  State OperationState `json:"state"`
  // LastTransitionTime is the time at which the operation state last transitioned from one state to another.
  LastTransitionTime metav1.Time `json:"lastTransitionTime"`
  // A human readable message indicating details about the last operation.
  Reason string `json:"reason"`
}

// LastError stores details of the most recent error encountered for the task.
type LastError struct {
  // Code is an error code that uniquely identifies an error.
  Code ErrorCode `json:"code"`
  // Description is a human-readable message indicating details of the error.
  Description string `json:"description"`
  // ObservedAt is the time at which the error was observed.
  ObservedAt metav1.Time `json:"observedAt"`
}

// TaskState represents the state of the task.
type TaskState string

const (
  TaskStateFailed TaskState = "Failed"
  TaskStatePending TaskState = "Pending"
  TaskStateRejected TaskState = "Rejected"
  TaskStateSucceeded TaskState = "Succeeded"
  TaskStateInProgress TaskState = "InProgress"
)

// OperationState represents the state of last operation.
type OperationState string

const (
  OperationStateFailed OperationState = "Failed"
  OperationStatePending OperationState = "Pending"
  OperationStateCompleted OperationState = "Completed"
  OperationStateInProgress OperationState = "InProgress"
)
```

### Custom Resource YAML API

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: EtcdOperatorTask
metadata:
    name: <name of operator task resource>
    namespace: <cluster namespace>
    generation: <specific generation of the desired state>
spec:
    type: <type/category of supported out-of-band task>
    ttlSecondsAfterFinished: <time-to-live to garbage collect the custom resource after it has been completed>
    config: <task specific configuration>
    ownerEtcdRefrence: <refer to corresponding etcd owner name and namespace for which task has been invoked>
status:
    observedGeneration: <specific observedGeneration of the resource>
    state: <last known current state of the out-of-band task>
    initiatedAt: <time at which task move to any other state from "pending" state>
    lastErrors:
    - code: <error-code>
      description: <description of the error>
      observedAt: <time the error was observed>
    lastOperation:
      name: <operation-name>
      state: <task state as seen at the completion of last operation>
      lastTransitionTime: <time of transition to this state>
      reason: <reason/message if any>
```

### Lifecycle

#### Creation

Task(s) can be created by creating an instance of the `EtcdOperatorTask` custom resource specific to a task.

> Note: In future, either a `kubectl` extension plugin or a `druidctl` tool will be introduced. Dedicated sub-commands will be created for each `out-of-band` task. This will drastically increase the usability for an operator for performing such tasks, as the CLI extension will automatically create relevant instance(s) of `EtcdOperatorTask` with the provided configuration.

#### Execution

* Authors propose to introduce a new controller which watches for `EtcdOperatorTask` custom resource.
* Each `out-of-band` task may have some task specific configuration defined in [.spec.config](#spec).
* The controller needs to parse this task specific config, which comes as a [string](#spec), according to the schema defined for each task.
* For every `out-of-band` task, a set of `pre-conditions` can be defined. These pre-conditions are evaluated against the current state of the target etcd cluster. Based on the evaluation result (boolean), the task is permitted or denied execution.
* If multiple tasks are invoked simultaneously or in `pending` state, then they will be executed in a First-In-First-Out (FIFO) manner.

> Note: Dependent ordering among tasks will be addressed later which will enable concurrent execution of tasks when possible.

#### Deletion

Upon completion of the task, irrespective of its final state, `Etcd-druid` will ensure the garbage collection of the task custom resource and any other Kubernetes resources created to execute the task. This will be done according to the `.spec.ttlSecondsAfterFinished` if defined in the [spec](#spec), or a default expiry time will be assumed.

### Use Cases

#### Recovery from permanent quorum loss

Recovery from permanent quorum loss involves two phases - identification and recovery - both of which are done manually today. This proposal intends to automate the latter. Recovery today is a [multi-step process](https://github.com/gardener/etcd-druid/blob/master/docs/operations/recovery-from-permanent-quorum-loss-in-etcd-cluster.md) and needs to be performed carefully by a human operator. Automating these steps would be prudent, to make it quicker and error-free. The identification of the permanent quorum loss would remain a manual process, requiring a human operator to investigate and confirm that there is indeed a permanent quorum loss with no possibility of auto-healing.

##### Task Config

We do not need any config for this task. When creating an instance of `EtcdOperatorTask` for this scenario, `.spec.config` will be set to nil (unset).

##### Pre-Conditions

* There should be a quorum loss in a multi-member etcd cluster. For a single-member etcd cluster, invoking this task is unnecessary as the restoration of the single member is automatically handled by the backup-restore process.
* There should not already be a permanent-quorum-loss-recovery-task running for the same etcd cluster.

#### Trigger on-demand snapshot compaction

`Etcd-druid` provides a configurable [etcd-events-threshold](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md#druid-flags) flag. When this threshold is breached, then a [snapshot compaction](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md) is triggered for the etcd cluster. However, there are scenarios where an ad-hoc snapshot compaction may be required.

##### Possible scenarios

* If an operator anticipates a scenario of permanent quorum loss, they can trigger an `on-demand snapshot compaction` to create a compacted full-snapshot. This can potentially reduce the recovery time from a permanent quorum loss.
* As an additional benefit, a human operator can leverage the current implementation of [snapshot compaction](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md), which internally triggers `restoration`. Hence, by initiating an `on-demand snapshot compaction` task, the operator can verify the integrity of etcd cluster backups, particularly in cases of potential backup corruption or re-encryption. The success or failure of this snapshot compaction can offer valuable insights into these scenarios.

##### Task Config

We do not need any config for this task. When creating an instance of `EtcdOperatorTask` for this scenario, `.spec.config` will be set to nil (unset).

##### Pre-Conditions

* There should not be a `on-demand snapshot compaction` task already running for the same etcd cluster.

> Note: `on-demand snapshot compaction` runs as a separate job in a separate pod, which interacts with the backup bucket and not the etcd cluster itself, hence it doesn't depend on the health of etcd cluster members.

#### Trigger on-demand full/delta snapshot

`Etcd` custom resource provides an ability to set [FullSnapshotSchedule](https://github.com/gardener/etcd-druid/blob/master/api/v1alpha1/etcd.go#L158) which currently defaults to run once in 24 hrs. [DeltaSnapshotPeriod](https://github.com/gardener/etcd-druid/blob/master/api/v1alpha1/types_etcd.go#L171) is also made configurable which defines the duration after which a delta snapshot will be taken.
If a human operator does not wish to wait for the scheduled full/delta snapshot, they can trigger an on-demand (out-of-schedule) full/delta snapshot on the etcd cluster, which will be taken by the `leading-backup-restore`.

##### Possible scenarios

* An on-demand full snapshot can be triggered if scheduled snapshot fails due to any reason.
* [Gardener Shoot Hibernation](https://github.com/gardener/gardener/blob/master/docs/usage/shoot_hibernate.md): Every etcd cluster incurs an inherent cost of preserving the volumes even when a gardener shoot control plane is scaled down, i.e the shoot is in a hibernated state. However, it is possible to save on hyperscaler costs by invoking this task to take a full snapshot before scaling down the etcd cluster, and deleting the etcd data volumes afterwards.
* [Gardener Control Plane Migration](https://github.com/gardener/gardener/blob/master/docs/proposals/07-shoot-control-plane-migration.md): In [gardener](https://github.com/gardener/gardener), a cluster control plane can be moved from one seed cluster to another. This process currently requires the etcd data to be replicated on the target cluster, so a full snapshot of the etcd cluster in the source seed before the migration would allow for faster restoration of the etcd cluster in the target seed.

##### Task Config

```go
// SnapshotType can be full or delta snapshot.
type SnapshotType string

const (
  SnapshotTypeFull SnapshotType = "full"
  SnapshotTypeDelta SnapshotType = "delta"
)

type OnDemandSnapshotTaskConfig struct {
  // Type of on-demand snapshot.
  Type SnapshotType `json:"type"`
}
```

```yaml
spec:
  config: |
    type: <type of on-demand snapshot>
```

##### Pre-Conditions

* Etcd cluster should have a quorum.
* There should not already be a `on-demand snapshot` task running with the same `SnapshotType` for the same etcd cluster.

#### Trigger on-demand maintenance of etcd cluster

Operator can trigger on-demand [maintenance of etcd cluster](https://etcd.io/docs/v3.5/op-guide/maintenance) which includes operations like [etcd compaction](https://etcd.io/docs/v3.5/op-guide/maintenance/#history-compaction-v3-api-key-value-database), [etcd defragmentation](https://etcd.io/docs/v3.5/op-guide/maintenance/#defragmentation) etc.

##### Possible Scenarios

* If an etcd cluster is heavily loaded, which is causing performance degradation of an etcd cluster, and the operator does not want to wait for the scheduled maintenance window then an `on-demand maintenance` task can be triggered which will invoke etcd-compaction, etcd-defragmentation etc. on the target etcd cluster. This will make the etcd cluster lean and clean, thus improving cluster performance.

##### Task Config

```go
type OnDemandMaintenanceTaskConfig struct {
  // MaintenanceType defines the maintenance operations need to be performed on etcd cluster.
  MaintenanceType maintenanceOps `json:"maintenanceType`
}

type maintenanceOps struct {
  // EtcdCompaction if set to true will trigger an etcd compaction on the target etcd.
  // +optional
  EtcdCompaction bool `json:"etcdCompaction,omitempty"`
  // EtcdDefragmentation if set to true will trigger a etcd defragmentation on the target etcd.
  // +optional
  EtcdDefragmentation bool `json:"etcdDefragmentation,omitempty"`
}
```

```yaml
spec:
  config: |
    maintenanceType:
      etcdCompaction: <true/false>
      etcdDefragmentation: <true/false>
```

##### Pre-Conditions

* Etcd cluster should have a quorum.
* There should not already be a duplicate task running with same `maintenanceType`.

#### Copy Backups Task

Copy the backups(full and delta snapshots) of etcd cluster from one object store(source) to another object store(target).

##### Possible Scenarios

* In [Gardener](https://github.com/gardener/gardener), the [Control Plane Migration](https://github.com/gardener/gardener/blob/master/docs/proposals/07-shoot-control-plane-migration.md) process utilizes the copy-backups task. This task is responsible for copying backups from one object store to another, typically located in different regions.

##### Task Config

```go
// EtcdCopyBackupsTaskConfig defines the parameters for the copy backups task.
type EtcdCopyBackupsTaskConfig struct {
  // SourceStore defines the specification of the source object store provider.
  SourceStore StoreSpec `json:"sourceStore"`

  // TargetStore defines the specification of the target object store provider for storing backups.
  TargetStore StoreSpec `json:"targetStore"`

  // MaxBackupAge is the maximum age in days that a backup must have in order to be copied.
  // By default all backups will be copied.
  // +optional
  MaxBackupAge *uint32 `json:"maxBackupAge,omitempty"`

  // MaxBackups is the maximum number of backups that will be copied starting with the most recent ones.
  // +optional
  MaxBackups *uint32 `json:"maxBackups,omitempty"`
}
```

```yaml
spec:
  config: |
    sourceStore: <source object store specification>
    targetStore: <target object store specification>
    maxBackupAge: <maximum age in days that a backup must have in order to be copied>
    maxBackups: <maximum no. of backups that will be copied>
```

> Note: For detailed object store specification please refer [here](https://github.com/gardener/etcd-druid/blob/9c5f8254e3aeb24c1e3e88d17d8d1de336ce981b/api/v1alpha1/types_common.go#L15-L29)

##### Pre-Conditions

* There should not already be a `copy-backups` task running.

> Note: `copy-backups-task` runs as a separate job, and it operates only on the backup bucket, hence it doesn't depend on health of etcd cluster members.

> Note: `copy-backups-task` has already been implemented and it's currently being used in [Control Plane Migration](https://github.com/gardener/gardener/blob/master/docs/proposals/07-shoot-control-plane-migration.md) but `copy-backups-task` will be harmonized with `EtcdOperatorTask` custom resource.

## Metrics

Authors proposed to introduce the following metrics:

* `etcddruid_operator_task_duration_seconds` : Histogram which captures the runtime for each etcd operator task.
  Labels:
  * Key: `type`, Value: all supported tasks
  * Key: `state`, Value: One-Of {failed, succeeded, rejected}
  * Key: `etcd`, Value: name of the target etcd resource
  * Key: `etcd_namespace`, Value: namespace of the target etcd resource

* `etcddruid_operator_tasks_total`: Counter which counts the number of etcd operator tasks.
  Labels:
  * Key: `type`, Value: all supported tasks
  * Key: `state`, Value: One-Of {failed, succeeded, rejected}
  * Key: `etcd`, Value: name of the target etcd resource
  * Key: `etcd_namespace`, Value: namespace of the target etcd resource
