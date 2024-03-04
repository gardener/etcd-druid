---
title: Etcd cluster operator out-of-band tasks
dep-number: 05
creation-date: 6th Dec'2023
status: implementable
authors:
- "@ishan" 
- "@sesha"
reviewers:
- "@etcd-druid-maintainers"
---

# DEP-05: Etcd cluster operator out-of-band tasks

## Table of Contents

* [DEP-05: Etcd cluster operator out-of-band tasks](#dep-05-etcd-cluster-operator-out-of-band-tasks)
  * [Table of Contents](#table-of-contents)
  * [Summary](#summary)
  * [Terminology](#terminology)
  * [Motivation](#motivation)
    * [Definition of "out-of-band" task](#definition-of-out-of-band-task)
    * [Goals](#goals)
      * [Concrete Use Cases](#concrete-use-cases)
  * [Proposal](#proposal)
    * [Operator tasks Custom resources and its controller](#operator-tasks-custom-resource-and-controller)
      * [1 custom resource vs n custom resources](#1-custom-resource-vs-n-custom-resources)
      * [1 contoller vs n-controller](#1-contoller-vs-n-controller)
    * [Custom Resource API](#custom-resource-api)
      * [Spec](#spec)
      * [Status](#status)
      * [API Object](#api-object)
      * [Task specific config](#task-specific-config)
        * [Examples of Task specific config](#examples-of-task-specific-config)
    * [Dealing with multiple operator tasks simultaneously](#dealing-with-multiple-operator-tasks-simultaneously)
    * [Lifecycle of an Operator Tasks custom resources](#lifecycle-of-an-operator-tasks-custom-resources)
    * [Few Implementation Details](#few-implementation-details)
  * [Graduation Criteria](#graduation-criteria)
    * [Alpha](#alpha)
    * [Alpha -> Beta](#alpha---beta)
    * [Beta -> GA](#beta---ga)
  * [Monitoring Requirements](#monitoring-requirements)
  * [Reference](#reference)

## Summary

The proposal suggests the creation of a unified framework for managing "out-of-band" operator tasks in an etcd cluster. It recommends using a single dedicated custom resource for all "out-of-band" operator tasks, simplifying the process of adding new tasks and managing their lifecycle. The proposal also suggests using a single controller to orchestrate these tasks, initiating actions based on the task type specified in the custom resource.
The custom resource(say EtcdOperatorTasks), will have specific fields in its spec (desired state) and status (current state) to capture the type and configuration of each task, as well as their status and any errors.
The proposal also addresses the challenge of dealing with multiple or duplicate tasks triggered simultaneously. It suggests defining pre-conditions for each task, which must be met for the task to be executed. If multiple tasks are pending, they will be executed in a First-In-First-Out (FIFO) manner.

## Terminology

* **druid:** [etcd-druid](https://github.com/gardener/etcd-druid) is an operator to manage the etcd clusters.

* **backup-sidecar:** It is the etcd-backup-restore sidecar container running in each etcd-member pod of etcd cluster.

* **leading-backup-sidecar:** A backup-sidecar that is associated to an etcd leader of an etcd cluster.

* **on-demand**: Refers to any task or operation that is performed outside of the regular schedule.

## Motivation

[Etcd-druid](https://github.com/gardener/etcd-druid) mainly acts as an etcd cluster provisioner(creation, maintenance and deletion), but sometimes operator wants to perform some on-demand operations on etcd cluster. These operations, which can be executed on-demand, are referred to as "out-of-band" operations/tasks. Operations that have nothing to do with the desired state of the etcd cluster.
Today, these "out-of-band" task, operator have to perform via different methods like manual, or via some other ways, but there is no single standard interface available for operator to use that for these "out-of-band" operator task
This proposal proposes a unified framework to perform operator's `out-of-band` operations/tasks on etcd cluster.

Examples of various "out-of-band" operator task are following:

* Recovery from permanent quorum loss.
* Trigger on-demand full/delta snapshots.
* Trigger on-demand snapshot compaction.
* Trigger on-demand maintenance of etcd cluster.
* Copy the backups from one object store to another object store.

## Definition of "out-of-band" task

"out-of-band" task refer to any on-demand tasks/operations that can be executed on an etcd cluster without modifying the Etcd custom resource spec (desired state).

## Goals

* Establish a unified interface for operators to initiate any "out-of-band" operator task at their convenience and monitor their status.
* Facilitate the easy addition of new "out-of-band" task through this framework.

### Concrete Use Cases

1. Recovery from permanent quorum loss.

    Description:

    Automate the handling and recovering from permanent quorum loss of etcd cluster as right now etcd-druid operator don't take any action in case of permanent quorum loss of etcd cluster.
    This operation will affect etcd custom resource spec(desired state) but etcd cluster is no longer in a healthy state. Hence, it can be labelled as "out-of-band" operator task.

    Application:
    * Right now, to recover from permanent quorum loss the human operator needs to follow certain steps mentioned in our [ops-guide document](https://github.com/gardener/etcd-druid/blob/master/docs/operations/recovery-from-permanent-quorum-loss-in-etcd-cluster.md), etcd-druid don't take any action by itself as it involves the deletion of etcd cluster volumes. We still can't let etcd-druid reconciliation to act on permanent quorum loss but [steps to recover]((https://github.com/gardener/etcd-druid/blob/master/docs/operations/recovery-from-permanent-quorum-loss-in-etcd-cluster.md)) from permanent quorum can be automated using a "out-of-band" operator task.

2. Trigger on-demand full/delta snapshots.

    Description:

    If operator don't want to wait for schedule full/delta snapshot, operator can trigger a on-demand(out-of-schedule) full/delta snapshot on etcd cluster which will be taken by backup-restore leader.
    Since this operation will not affect etcd custom resource spec(desired state), it can be labelled as "out-of-band" operator task.

    Applications:
    * A on-demand full snapshot can be triggered before hibernation of cluster, so that PVCs can be deleted.
    * A on-demand full snapshot can be triggered before control plane migration starts.
    * A on-demand full snapshot can be triggered if scheduled snapshot fails due to any reason.

3. Trigger on-demand snapshot compaction.

    Description:

    If operator don't want to wait for [event-threshold](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md#druid-flags) to reach to trigger a [snapshot compaction](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md). Operator can just trigger a on-demand snapshot compaction job to take a compacted full snapshot.
    Since this operation will not affect etcd custom resource spec(desired state), it can be labelled as "out-of-band" operator task.

    Applications:
    * Full snapshot taken via compaction job is better as compared to full snapshot taken directly from etcd cluster as full snapshot taken via snapshot compaction is taken after [defragmentation](https://etcd.io/docs/v3.2/op-guide/maintenance/#defragmentation) called on etcd, hence this full snapshot is much more compacted, hence less in size.
    * Full snapshot triggered on etcd might cause load on running etcd cluster but full snapshot taken by compaction job is developed separately with zero effect on running cluster.

4. Trigger on-demand maintenance of etcd cluster.

    Description:

    Operator can trigger on-demand maintenance of etcd cluster which includes operations like [etcd compaction](https://etcd.io/docs/v3.5/op-guide/maintenance/#history-compaction-v3-api-key-value-database), [etcd defragmentation](https://etcd.io/docs/v3.2/op-guide/maintenance/#defragmentation) etc.
    Since this operation will not affect etcd custom resource spec(desired state), it can be labelled as "out-of-band" operator task.

    Applications:
    * If etcd cluster is heavily loaded which is causing performance degradation of etcd cluster, and operator doesn't want to wait for schedule maintenance window then operator can trigger on-demand maintenance of etcd cluster with the help operations like etcd-compaction, etcd-defragmentation which will make etcd cluster lean and clean, therefore improves cluster performance.

5. Copy the backups from one object store to another object store.

    Description:

    To copy the backups from one object store(source bucket) to another object store(target bucket).
    Since this operation is not affecting etcd custom resource spec(desired state), it can be labelled as "out-of-band" operator task.

    Application:
    * Although this task has already been implemented and currently being used in control plane migration but this can also be done via "out-of-band" operation framework.

    **Note**: Right now, copying backups from one object store to another has already achieved by [EtcdCopyBackupsTask controller](https://github.com/gardener/etcd-druid/blob/master/docs/concepts/controllers.md#etcdcopybackupstask-controller) and its dedicated CRD. But with the help of `out-of-bands operations` framework, we can harmonize the EtcdCopyBackupsTask as well.

## Proposal

### Operator tasks custom resource and controller

#### 1 custom resource vs n custom resources

Whether all "out-of-band" operator tasks should share a single dedicated custom resource, or each "out-of-band" operator task should have its own dedicated custom resource.

In this proposal, authors are proposing that all "out-of-band" operator tasks should have single dedicated custom resource. Due to the following reasons:

1. As we are providing a framework, it makes more sense to define a standard one custom resource for all operator tasks. Having many custom resources could make it more challenging for others to contribute a new operator tasks.

2. The process of adding a new task will be simplified, as there will be no need to introduce a new custom resource for each "out-of-band" task. This also eliminates the need to manage the lifecycle of every newly added custom resource.

3. The disadvantage of having a single custom resource which is to seprately define and capture the intermediate states of each "out-of-band" task in the spec(desired state) and staus(current state) is overcomed by using [runtime.RawExtension](https://github.com/kubernetes/apimachinery/blob/829ed199f4e0454344a5bc5ef7859a01ef9b8e22/pkg/runtime/types.go#L49-L102) which provides flexible data storage in pre-defined schema for each task.

#### 1 contoller vs n-controller

A single custom resource for all operator "out-of-band" tasks can be managed by one single controller or multiple controllers. This decision is mainly left to implementation.

In this proposal, the authors recommend the implementation of a single controller with the ability to orchestrate these "out-of-band" operator tasks. The controller would initiate actions based on the `taskType` specified in the spec(desired state) of the EtcdOperatorTasks custom resource.

### Custom Resource API

The authors propose to add the `EtcdOperatorTasks` custom resource API to etcd-druid APIs and initially introduce it with `v1alpha1` version.

#### Spec

The authors propose that the following fields should be specified in the spec (desired state) of the EtcdOperatorTasks custom resource.

- To capture the type of "out-of-band" operator task to be performed, `taskType` field is defined in spec. It can have values of all supported "out-of-band" operations/tasks eg. "OnDemandSnaphotTask", "QuorumLossRecoveryTask" etc.
- To capture the configuration specific to each task, a `taskConfig` field is defined in spec as type [RawExtension](https://github.com/kubernetes/apimachinery/blob/829ed199f4e0454344a5bc5ef7859a01ef9b8e22/pkg/runtime/types.go#L49-L102). The controller (for instance, the EtcdOperatorTasks Controller) needs to parse this data, which comes as a RawExtension, according to the schema defined for each task.
- `.spec.ttlSecondsAfterFinished` defines a `Time-to-live` to garbage collect the custom resource after completion of task.

```go
// EtcdOperatorTasksSpec is the spec for a EtcdOperatorTasks resource.
type EtcdOperatorTasksSpec struct {

  // TaskType is a field that specifies the type of out-of-band operator task to be performed. 
  // It can have values of all supported operation/task type eg. "OnDemandSnapshotTask" etc.
  TaskType *string `json:"taskType"`

  // TTLSecondsAfterFinished define time-to-libe to garbage collect the custom resource after completion of task. 
  TTLSecondsAfterFinished  int32 `json:"ttlSecondsAfterFinished"`

  // TaskConfig is a task specific configuration.
  TaskConfig *runtime.RawExtension `json:"taskConfig,omitempty"`
}
```

#### Status

The authors propose that the following fields should be specified in the status (current state) of the EtcdOperatorTasks custom resource.

- To capture the Status of the task a `taskStatus` field is define in status.
- If operation involves many stages, so to capture the status of intermediate or any stage, `.status.lastOperations` will be useful.
- `.status.lastError` to capture the last error occured for the task.

```go

// EtcdOperatorTasksStatus is the status for a EtcdOperatorTasks resource.
type EtcdOperatorTasksStatus struct {

  // ObservedGeneration is the most recent generation observed for the resource.
  ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
  
  // Status of the last operation, one of pending, in-progress, completed, rejected or failed.
  TaskStatus TaskState `json:"taskStatus"`
  
  // Time at which operation has been triggered.
  InitiatedAt metav1.Time `json:"initiatedAt"`
  
  // LastError represents the last occurred error.
  LastError *string `json:"lastError,omitempty"`
  
  // To capture status of last operation.
  LastOps []LastOperation `json:"lastOperations"`
}

// TaskState represents the state of the task.
type TaskState string

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

// OperationState represents the state of the operation.
type OperationState string
```

#### API Object

```yaml

apiVersion: druid.gardener.cloud/v1alpha1
kind: EtcdOperatorTasks
metadata:
    name: <name of operator tasks resource>
    namespace: <cluster namespace>
    generation: <specific generation of the desired state>
spec:
    taskType: < QuorumLossRecoveryTask | OnDemandSnapshotTask | OnDemandSnapCompactionTask | OnDemandMaintenanceTask | CopyBackupsTask >
    ttlSecondsAfterFinished: <time-to-live to garbage collect the custom resource>
    taskConfig: <task specific configuration>
status:
    observedGeneration: <specific observedGeneration of the resource>
    taskStatus: <pending | inProgress | completed | rejected | failed>
    initiatedAt: <time of intiation of this operation>
    errors: <last error message if any>
    lastOperations:
    - name: <operation-name>
      state: <pending | inProgress | complete | failed>
      lastTransitionTime: <time of transition to this state>
      reason: <reason/message if any>

```

#### Task specific config

- The task specific configuration in spec of EtcdOperatorTasks custom resource will be define by `TaskConfig *runtime.RawExtension` as mentioned in [spec](#spec)

```go
type EtcdOperatorTasksSpec struct {
  . . .
  // TaskConfig is a task specific configuration.
  TaskConfig *runtime.RawExtension `json:"taskConfig,omitempty"`
}
```

- To add a new task via this framework the user have to define the task specific configuration and Controller (say EtcdOperatorTasks Controller) need to parse this data coming as [RawExtension](https://pkg.go.dev/k8s.io/apimachinery/pkg/runtime#RawExtension) according to [.spec.taskType](#spec)

##### Examples of Task specific config

1. Recovery from permanent quorum loss

```go
type QuorumLossRecoveryTaskConfig struct {
  metav1.TypeMeta

  // Total number of etcd cluster replicas.
  Replicas int32 `json:"replicas"`
}
```

2. On demand full/delta snapshots

```go
// snapshotType can be full or delta snapshot
snapshotType string `json:"snapshotType"`

type OnDemandSnapshotTaskConfig struct {
  metav1.TypeMeta

  // Type of on-demand snapshot.
  SnapshotType snapshotType `json:"snapshotType"`
}
```

3. On demand maintenance of etcd cluster

```go
type OnDemandMaintenanceTaskConfig struct {
  metav1.TypeMeta

  // MaintenanceType defines the maintenance operations need to be perform on etcd cluster.
  MaintenanceType maintenanceOps
}

type maintenanceOps struct {
  EtcdCompaction *bool `json:"etcd-compaction,omitempty"`
  EtcdDefragmentation *bool `json:"defragmentation,omitempty"`
}
```

##### Example to decode a Task specific config

```go
  if err := e.Decode(e.Spec.OperationsConfig.Raw, nil, QuorumLossRecoveryTaskConfig); err != nil {
    return err
  }
```

### Dealing with multiple operator tasks simultaneously

It is quite apparent that multiple as well as duplicate "out-of-band" operator tasks can be triggered simultaneously or just one after other. The challenge then becomes determining the precedence of each operation.
For instance, let's say an operator triggers an `OnDemandSnapshotTask` for full-snapshot that doesn't gets complete. Then, the human operator realizes the cluster has lost its quorum, so they trigger a `QuorumLossRecoveryTask` task.
In this scenario, controller (say EtcdOperatorTasks Controller) will detect two pending tasks at the same time. The question that arises which task should be executed first ?

To deal with such scenario, authors are proposing that each "out-of-bands" operator task can have some pre-defined pre-conditions which should be necessary to perform that operation otherwise controller can simply `reject` the operations.
Moreover, controller (EtcdOperatorTasks Controller) will execute each operator task in a First-In-First-Out (FIFO) fashion.

| Out-of-band Tasks                | Pre-conditions      |  Reasons                                               |
| ---------------------------------|---------------------|--------------------------------------------------------|
| `QuorumLossRecoveryTask`         | `replicas` > 1      | if no. of etcd replicas is one then it's simple restoration case, not a quorum loss recovery.         |
| `OnDemandSnapshotTask`           | cluster should be healhty i.e quorum should be maintained        | taking a on-demand snapshot(full/delta) involves etcd cluster.           |
| `OnDemandSnapCompactionTask`     | NA          | on-demand snapshot-compaction runs in a seprate pod, hence it don't depend on health of etcd cluster members or any other conditions.       |
| `OnDemandMaintenanceTaskConfig`  | cluster should be healhty i.e quorum should be maintained         | on-demand etcd cluster maintenance depends on the health of etcd cluster.              |
| `CopyBackupsTask`                | NA         | copy-backup tasks run seprately without affecting or required help from etcd cluster.                      |

Steps to follow in case of multiple operator tasks:

- Check for duplicate task. If found simply `reject` the task.
- Verify if the pre-conditions of the task are met. If not, `reject` the task.
   * If the pre-conditions are satisfied, add the task to the queue.

- For adding any new operations to "out-of-band" operator tasks, user must have to define the pre-conditions of that task, if any.

### Lifecycle of an Operator Tasks custom resources

#### Creation

* For [alpha feature](#alpha), creation of operator tasks custom resources can be done manually.
* For [GA](#beta---ga), the creation of operator tasks custom resources will done by `druidctl add task <taskName>`.

#### Deletion

To garbage collect the custom resources can be done by another controller (say TTL controller) with help of `.spec.ttlSecondsAfterFinished` specified in each operator tasks custom resource.

### Few Implementation Details

1. In QuorumLossRecoveryTask, recovery controller can't move to next operation until pervious operation moved to `complete` state.
2. If both type of maintenance is requested then controller should trigger etcd-compaction first then defragmentation.

### Graduation Criteria

#### Alpha

- For alpha graduation, creation of operator tasks custom resource can be done manually and once operator tasks get completed then deletion of operator tasks custom resource can also be done manually.
- Unit and e2e tests.

#### Alpha -> Beta

- Introduce appropriate metrics which are agreed on.
- Another controller will be introduced (say TTL controller) which will garbage collect the operator tasks custom resource after waiting for time defined by: `.spec.ttlSecondsAfterFinished`.
- Upgrade/Rollback manually tested.

#### Beta -> GA

- A `druidctl` will implemented to manage the lifecycle of operator tasks custom resource.
- Enabled in Beta for at least two releases without complaints.

#### Monitoring Requirements

* `operation_total_time_taken{kind=operation_type, label=failed/success}`
* `no_of_operator_tasks{kind=operation_type, label=failed/succeeded/rejected}`

## Reference

* [TTL mechanism](https://kubernetes.io/docs/concepts/workloads/controllers/job/#ttl-mechanism-for-finished-jobs)
