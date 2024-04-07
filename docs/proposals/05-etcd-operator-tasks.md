---
title: operator out-of-band tasks
dep-number: 05
creation-date: 6th Dec'2023
status: implementable
authors:
- "@ishan" 
- "@madhav"
- "@sesha"
reviewers:
- "@etcd-druid-maintainers"
---

# DEP-05: Operator Out-Of-Band Tasks

## Table of Contents

* [DEP-05: Operator out-of-band tasks](#operator-out-Of-band-tasks)
  * [Table of Contents](#table-of-contents)
  * [Summary](#summary)
  * [Terminology](#terminology)
  * [Motivation](#motivation)
  * [Goals](#goals)
  * [Proposal](#proposal)
    * [API](#api)
    * [Golang API](#golang-api)
      * [Spec](#spec)
      * [Status](#status)
    * [Lifecycle](#lifecycle)
      * [Creation](#creation)
      * [Execution](#execution)
      * [Deletion](#deletion)
    * [Use cases](#use-cases)
      * [Recovery from permanent quorum loss](#recovery-from-permanent-quorum-loss)
      * [Trigger on-demand snapshot compaction](#trigger-on-demand-snapshot-compaction)
      * [Trigger on-demand full/delta snapshot](#trigger-on-demand-fulldelta-snapshot)
      * [Trigger on-demand maintenance of etcd cluster](#trigger-on-demand-maintenance-of-etcd-cluster)
      * [Copy backups from one object store to another object store](#copy-the-backups-from-one-object-store-to-another-object-store)
  * [Metrics](#metrics)

## Summary

This DEP proposes an enhancement to `etcd-druid`'s capabilities to handle [out-of-band](#terminology) tasks, which are presently performed manually or invoked programmatically via sub-optimal APIs. The document proposes the establishment of an unified interface by defining a well-structured API to harmonize the initiation of any `out-of-band` task, monitor its status, and simplify the process of adding new tasks and managing their lifecycle.

## Terminology

* **druid:** [etcd-druid](https://github.com/gardener/etcd-druid) is an operator to manage the etcd clusters.

* **backup-sidecar:** It is the etcd-backup-restore sidecar container running in each etcd-member pod of etcd cluster.

* **leading-backup-sidecar:** A backup-sidecar that is associated to an etcd leader of an etcd cluster.

* **out-of-band task** Any on-demand tasks/operations that can be executed on an etcd cluster without modifying the Etcd custom resource spec (desired state).

## Motivation

Today, [Etcd-druid](https://github.com/gardener/etcd-druid) mainly acts as an etcd cluster provisioner(creation, maintenance and deletion). In future, capabilities of etcd-druid will be enhanced via [etcd-member](https://github.com/gardener/etcd-druid/blob/8ac70d512969c2e12e666d923d7d35fdab1e0f8e/docs/proposals/04-etcd-member-custom-resource.md) proposal by providing it access to a much more detailed information about each etcd cluster member. While we enhance the reconciliation and monitoring capabilities of etcd-druid, it still lacks an ability to provide an easy-to-use hook to trigger `out-of-band` tasks on an existing etcd cluster.

There are new learnings while operating etcd clusters at scale. It has been observed that we regularly need capabilities to trigger `out-of-band` tasks which are out of the purview of a regular etcd reconciliation run. Many of these tasks are a multi-step process and doing it manually is error-prone even if an operator follows a well-written step-by-step guide. Thus there is a need to automate these tasks to alleviate mistakes.
Some examples of an `on-demand/out-of-band` operations:

* Recover from a permament quorum loss.
* Trigger an on-demand full/delta snapshot.
* Trigger an on-demand snapshot compaction.
* Trigger an on-demand [maintenance of etcd cluster](https://etcd.io/docs/v3.4/op-guide/maintenance/).
* Copy the backups from one object store to another object store.

## Goals

* Establish an unified interface for operator tasks by defining a single dedicated custom resource for `out-of-band` tasks.
* Define a contract (in terms of prerequisites) which needs to be adhered to by any task.
* Facilitate the easy addition of new `out-of-band` task(s) through this custom resource.
* Provide CLI capabilities to operators making it easy to invoke supported `out-of-band` tasks.

## Non-Goals
* In the current scope, capability to abort/cancel an out-of-band task is not going to be provided. This could be considered as an enhancement based on pull.
* Ordering (by establishing dependency) of out-of-band tasks submitted for the same etcd cluster is not been considered in the first increment. In a future version based on how operator tasks are used we will enhance this proposal and the implementation.


## Proposal

Authors propose creation of a new single dedicated custom resource to represent an `out-of-band` task. Druid will be enhanced to process the task requests and update its status which can then be tracked/observed.

### API

`ETCDOperatorTask` is the new custom resource that will be introduced. This API will be in `v1alpha1` version and will be subject to change. We will be respecting [Kubernetes Deprecation Policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/).

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: EtcdOperatorTask
metadata:
    name: <name of operator task resource>
    namespace: <cluster namespace>
    generation: <specific generation of the desired state>
spec:
    taskType: <type/category of supported out-of-band task>
    ttlSecondsAfterFinished: <time-to-live to garbage collect the custom resource>
    taskConfig: <task specific configuration>
status:
    observedGeneration: <specific observedGeneration of the resource>
    taskStatus: < overall status of the task >
    initiatedAt: <time of intiation of this operation>
    lastErrors:
      - code: <error-code>
        description: <description of the error>
        lastUpdateTime: <time the error was reported>
    lastOperation:
      name: <operation-name>
      state: <task state as seen at the completion of last operation>
      lastTransitionTime: <time of transition to this state>
      reason: <reason/message if any>
    conditions:
      - type: <type of condition>
        status: <status of the condition, one of True, False, Unknown>
        lastTransitionTime: <last time the condition transitioned from one status to another>
        reason: <programmatic identifier indicating the reason for the condition's last transition>
        message: <human readable message indicating details about the transition>
```

> NOTE: The above custom resource YAML serves as a template which will be used to further create a CRD and Golang APIs.

### Golang API

```go
// EtcdOperatorTask represents an out-of-band operator task.
type EtcdOperatorTask struct {
  metav1.TypeMeta
  metav1.ObjectMeta

  // Spec is the specification of the task.
  Spec EtcdOperatorTaskSpec
  // Status is most recently observed status of the task.
  Status EtcdOperatorTaskStatus
}
```

#### Spec

The authors propose that the following fields should be specified in the spec (desired state) of the `EtcdOperatorTask` custom resource.

* To capture the type of `out-of-band` operator task to be performed, `spec.Type` field should be defined. It can have values from any supported "out-of-band" operations/tasks eg. "OnDemandSnaphotTask", "QuorumLossRecoveryTask" etc.
* To capture the configuration specific to each task, a `spec.Config` field is defined of type [RawExtension](https://github.com/kubernetes/apimachinery/blob/829ed199f4e0454344a5bc5ef7859a01ef9b8e22/pkg/runtime/types.go#L49-L102) as each task can have different input configuration.

```go
// EtcdOperatorTaskSpec is the spec for a EtcdOperatorTask resource.
type EtcdOperatorTaskSpec struct {
  
  // Type specifies the type of out-of-band operator task to be performed. 
  Type string `json:"taskType"`

  // Config is a task specific configuration.
  Config *runtime.RawExtension `json:"taskConfig,omitempty"`

  // TTLSecondsAfterFinished is the time-to-live after which the task and 
  // related resources will be garbage collected.
  // +optional
  TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`
}
```

#### Status

The authors propose that the following fields should be specified in the `Status` (current state) of the EtcdOperatorTask custom resource as the custom resource's `Status` will be used to monitor the progress of the task.

* To capture the Status of a task, a field `taskStatus` is defined in status.
* If operation involves many stages, so to capture the status of intermediate or any stage, `.status.lastOperation` will be useful.

```go
// TaskState represents the state of the task.
type TaskState string

const (
  Failed TaskState = "failed"
  Pending TaskState = "pending"
  Rejected TaskState = "rejected"
  Completed TaskState = "completed"
  InProgress TaskState = "inProgress"
)

// EtcdOperatorTaskStatus is the status for a EtcdOperatorTask resource.
type EtcdOperatorTaskStatus struct {
  // ObservedGeneration is the most recent generation observed for the resource.
  ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
  // State of the task is the last known state of the task.
  State TaskState `json:"taskStatus"`
  // Time at which operation has been triggered.
  InitiatedAt metav1.Time `json:"initiatedAt"`
  // LastError represents the errors when processing the task.
  LastErrors []LastError `json:"lastErrors,omitempty"`
  // Captures the last operation
  // +optional
  LastOperation *LastOperation `json:"lastOperation,omitempty"`
  // Conditions represents the latest available observations of an object's current state.
  // +optional
  Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type LastOperation struct {
  // Name of the LastOperation.
  Name opsName `json:"name"`
  // Status of the last operation, one of pending, progress, completed, failed.
  State OperationState `json:"state"`
  // Last time the operation state transitioned from one to another.
  LastTransitionTime metav1.Time `json:"lastTransitionTime"`
  // A human readable message indicating details about the last operation.
  Reason string `json:"reason"`
}

```

### Lifecycle

#### Creation

Task(s) can be created by creating an instance of the `EtcdOperatorTask` custom resource specific to a task.

> Note: In future, either a `kubectl` extension plugin or a `druidctl` will be introduced. Dedicated sub-commands will be created for each `out-of-band` task. This will drastically increase the usability for an operator for such tasks as the CLI extension will automatically create relevant instance of `EtcdOperatorTask` with the provided configuration.

#### Execution

* Authors propose to introduce a new controller (let's call it `operator-task-controller`) which watches for `EtcdOperatorTask` custom resource specific to a task defined by [.spec.taskType](#spec).
* Each `out-of-band` task may have some task specific configuration defined in [.spec.taskConfig](#spec).
* The controller (`operator-task-controller`) needs to parse this task specific config, which comes as a RawExtension, according to the schema defined for each task.
* Moreover, all tasks have to adhere to some prerequisites(a.k.a `pre-conditions`) which will be necessary to execute the task. Authors propose to define pre-conditions for each task, which must be met for the task to be eligible for execution otherwise that task should be rejected.
* If multiple tasks are invoked simultaneously or in `pending` state, then they will be executed in a First-In-First-Out (FIFO) manner.

> Note: Dependent ordering among tasks will be addresed later which may enable concurrent execution of tasks.

#### Deletion

`Etcd-druid` will ensure that it garbage collects the custom resources and any other k8s resources created to realise the task as per `.spec.ttlSecondsAfterFinished` if defined in the [spec](#spec) or it assumes a default expiry time.

### Use Cases

#### Recovery from permanent quorum loss

Currently identification and recovery from permament quorum loss is done manaually. The current proposal keeps the identification as manual, requiring an human operator to intervene and confirm that there is indeed a permanent quorum loss with no auto healing possibility. Once that is established then the next step to recover the cluster is now proposed to be automated. Recovery today is a [multi-step process](https://github.com/gardener/etcd-druid/blob/master/docs/operations/recovery-from-permanent-quorum-loss-in-etcd-cluster.md) and needs to be done carefully. Automation would ensure that we eliminate any errors from an operator.

##### Task Config

We do not need any config for this task. When creating an instance of `EtcdOperatorTask` for this scenario, `.spec.config` will be set to nil (unset).

##### Pre-Conditions

* There should be a quorum loss in a multi-member etcd cluster. For a single-member etcd cluster, invoking this task is unnecessary as the restoration of the single member is automatically handled by the backup-restore process.
* There should not already be a permanent-quorum-loss-recovery-task running for the same etcd cluster.

#### Trigger on-demand snapshot compaction

`etcd-druid` provides a configurable [etcd-events-threshold](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md#druid-flags). If and when this threshold is breached then only a [snapshot compaction](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md) is triggered for etcd cluster. However, there are scenarios where an ad-hoc snapshot compaction is required.

##### Possible scenarios

* Full snapshot taken via compaction job is better as compared to full snapshot taken directly from etcd cluster as full snapshot taken via snapshot compaction is taken after [defragmentation](https://etcd.io/docs/v3.2/op-guide/maintenance/#defragmentation) which makes it smaller (more compacted) in size.
* Full snapshot triggered on etcd cluster might cause load on running etcd cluster but full snapshot taken by compaction job is developed separately with zero effect on the running cluster.

##### Task Config

We do not need any config for this task. When creating an instance of `EtcdOperatorTask` for this scenario, `.spec.config` will be set to nil (unset).

##### Pre-Conditions

* There should not be a `on-demand snapshot-compaction` task already running for the same etcd cluster.

>Note: `on-demand snapshot-compaction` runs as a separate job in a seprate pod, hence it doesn't depends on health of etcd cluster members or any other conditions.

#### Trigger on-demand full/delta snapshot

`Etcd` custom resource provides an ability to set [FullSnapshotSchedule](https://github.com/gardener/etcd-druid/blob/master/api/v1alpha1/types_etcd.go#L158) which is currently defauled to run once in 24 hrs. [DeltaSnapshotPeriod](https://github.com/gardener/etcd-druid/blob/master/api/v1alpha1/types_etcd.go#L171) is also made configurable which defines the duration after which a delta snapshot will be taken.
If operator does not wish to wait for the scheduled full/delta snapshot, he/she can trigger an on-demand(out-of-schedule) full/delta snapshot on etcd cluster which will be taken by `leading-backup-restore`.

##### Possible scenarios

* [Shoot hibernation](https://github.com/gardener/gardener/blob/master/docs/usage/shoot_hibernate.md): Every etcd cluster incurs an inherent cost of preserving the volumes even when the shoot is in hibernation state. However it is possible to save costs by invoking this task to take a full snapshot before deleting the volumes.
* [Control Plane Migration](https://github.com/gardener/gardener/blob/master/docs/proposals/07-shoot-control-plane-migration.md): In [gardener](https://github.com/gardener/gardener) a seed cluster control plane can be moved to another seed cluster. To prevent data loss and faster restoration of the etcd cluster in the target seed, a full snapshot can be triggered for the etcd cluster in the source seed.
* A on-demand full snapshot can be triggered if scheduled snapshot fails due to any reason.

##### Task Config

```go
// SnapshotType can be full or delta snapshot
SnapshotType string `json:"snapshotType"`
const (
  FullSnapshot SnapshotType = "full-snapshot"
  DeltaSnapshot SnapshotType = "delta-snapshot"
)

type OnDemandSnapshotTaskConfig struct {
  // Type of on-demand snapshot.
  SnapshotType snapshotType `json:"snapshotType"`
}
```

##### Pre-Conditions

* Etcd cluster should have a quorum.
* There should not already be a `on-demand snapshot` task running with the same `SnapshotType` for the same etcd cluster.

#### Trigger on-demand maintenance of etcd cluster

Operator can trigger on-demand maintenance of etcd cluster which includes operations like [etcd compaction](https://etcd.io/docs/v3.5/op-guide/maintenance/#history-compaction-v3-api-key-value-database), [etcd defragmentation](https://etcd.io/docs/v3.2/op-guide/maintenance/#defragmentation) etc.

##### Possible Scenarios

* If an etcd cluster is heavily loaded which is causing performance degradation of an etcd cluster and the operator does not want to wait for the scheduled maintenance window then an on-demand maintenance task can be triggered which will invoke etcd-compaction, etcd-defragmentation etc. on the target etcd cluster. This will make etcd cluster lean and clean, thus improving cluster performance.

##### Task Config

```go
type OnDemandMaintenanceTaskConfig struct {
  // MaintenanceType defines the maintenance operations need to be performed on etcd cluster.
  MaintenanceType maintenanceOps
}

type maintenanceOps struct {
  EtcdCompaction *bool `json:"etcd-compaction,omitempty"`
  EtcdDefragmentation *bool `json:"defragmentation,omitempty"`
}
```

##### Pre-Conditions

* Etcd cluster should have a quorum.
* There should not already be a duplicate task running with same `maintenanceType`.

#### Copy the backups from one object store to another object store

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

##### Pre-Conditions

* There should not already be a duplicate task running.

> Note: `copy-backup-task` runs as a seprate job, hence it doesn't depends on health of etcd cluster members or any other conditions.

> Note: CopyBackupTask has already been implemented and it's currently being used in [Control Plane Migration](https://github.com/gardener/gardener/blob/master/docs/proposals/07-shoot-control-plane-migration.md) but CopyBackupTask can be harmonize with `EtcdOperatorTask` custom resource.

## Metrics

Authors proposed to introduce the following metrics:

* `etcd_operator_task_duration_seconds` : Histogram which captures the runtime for each etcd operator task.
  Labels:
  * Key : `type`, Value: task type
  * Key: `state` Value: One-Of {failed, success, rejected}
* `etcd_operator_tasks_total`: Counter which counts the number of etcd operator tasks.
  Labels:
  * Key : `type`, Value: task type
  * Key: `state` Value: One-Of {failed, success, rejected}
