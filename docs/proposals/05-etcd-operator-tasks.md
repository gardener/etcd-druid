---
title: operator out-of-band tasks
dep-number: 05
creation-date: 6th Dec'2023
status: implementable
authors:
- "@ishan" 
- "@sesha"
reviewers:
- "@etcd-druid-maintainers"
---

# DEP-05: Operator Out-Of-Band Tasks

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

This DEP proposes to extend the capability of `etcd-druid` to process `out-of-band` tasks which today are either  done manually or are invoked programmatically via sub-optimal APIs. This document proposes creation of a well-defined API to harmonize triggering of any `out-of-band` task and monitor its progress.

## Terminology

* **druid:** [etcd-druid](https://github.com/gardener/etcd-druid) is an operator to manage the etcd clusters.

* **backup-sidecar:** It is the etcd-backup-restore sidecar container running in each etcd-member pod of etcd cluster.

* **leading-backup-sidecar:** A backup-sidecar that is associated to an etcd leader of an etcd cluster.

* **out-of-band task** is any on-demand tasks/operations that can be executed on an etcd cluster without modifying the Etcd custom resource spec (desired state).

## Motivation

`Etcd Druid` is an etcd operator which creates, manages and deletes resources required to realise etcd clusters. Each etcd cluster has a corresponding [Etcd CR](https://github.com/gardener/etcd-druid/blob/master/api/v1alpha1/types_etcd.go#L72-L78) which defines the `to-be` state. Etcd druid reconciles the `as-is` state of an etcd cluster with its respective `to-be` state as defined in the Etcd CR. The current reconciliation ensures that all required resources are provisioned and the status of the etcd custom resource is regularly updated. This helps ensure the health of an etcd cluster to some extent but it is not sufficient. In future, capabilities of etcd-druid will be enhanced via [etcd-member](https://github.com/gardener/etcd-druid/blob/8ac70d512969c2e12e666d923d7d35fdab1e0f8e/docs/proposals/04-etcd-member-custom-resource.md) proposal by providing it access to a much more detailed information about each etcd-member.  While we enhance the reconciliation and monitoring capabilities of etcd-druid, it still lacks an ability to provide an easy-to-use handle for an operator to trigger `out-of-band` task on an existing etcd cluster which is essential for everyday support and recoverability of an etcd cluster.

There are new learnings while operating etcd clusters at scale. It has been observed that we regularly need capabilities to trigger `out-of-band` tasks which are out of the purview of a regular reconciliation run. Some examples of an `on-demand/out-of-band` operation:

* Recover from a permament quorum loss.
* Trigger an on-demand full/delta snapshot.
* Trigger on-demand snapshot compaction.
* Trigger on-demand maintenance of etcd cluster.
* Copy the backups from one object store to another object store.
* Inspect full/delta snapshots to gauge DB size growth and identify potential hot-spots.

The motivation for this proposal is to provide a well defined API to introduce `operator` tasks. We wish to achieve the following:

* Replace manual intervention by an operator to recover etcd clusters, thus alleviating any mistakes. 
* Provide programmatic integeration/orchestration via well-defined API objects (Custom Resources) for any out-of-band task. Ensures easy invocation and traceability of tasks.
* Provide operator ease by eventually creating a CLI wrapper for operator tasks.

## Goals
* Define a new custom resource allowing cosumers to create new `out-of-band` task specifications. The same custom resource's `Status` will be used to monitor the progress of the task.
* Enhance `etcd-druid` by introducing new controller(s) which will be responsible for listening for creation event for an `out-of-band` task and subsequently either reject or process the request.
* Define a contract (in terms of prerequisites) which needs to be adhered to by any task.
* Provide CLI capabilities to operators making it easy to invoke supported `out-of-band` tasks.

## Proposal

Authors propose creation of a new custom resource to represent an `out-of-band` task. Druid will be enhanced to process the task requests and update its status which can then be tracked/observed.

### API
`ETCDOperatorTask` is the new custom resource that will be introduced. This API will be in `v1alpha1` version and will be subject to change. We will be respecting [Kubernetes Deprecation Policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/).

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: EtcdOperatorTasks
metadata:
    name: <name of operator tasks resource>
    namespace: <cluster namespace>
    generation: <specific generation of the desired state>
spec:
    taskType: <type/category of task>
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

```go
// EtcdOperatorTasksSpec is the spec for a EtcdOperatorTasks resource.
type EtcdOperatorTaskSpec struct {
  
  // TaskType is a field that specifies the type of out-of-band operator task to be performed. 
  // It can have values of all supported operation/task type eg. "OnDemandSnapshotTask" etc.
  Type string `json:"taskType"`
  // TaskConfig is a task specific configuration.
  Config *runtime.RawExtension `json:"taskConfig,omitempty"`
  // TTLSecondsAfterFinished is the time-to-live after which the task and related resources will be
  // garbage collected
  TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`
}
```

```go
// TaskState is an alias that represents an overall state of the task.
type TaskState string
const (
  Pending TaskState = "pending"
  InProgress TaskState = "inProgress"
  Completed TaskState = "completed"
  Rejected TaskState = "rejected"
  Failed TaskState = "failed"
)

// EtcdOperatorTasksStatus is the status for a EtcdOperatorTasks resource.
type EtcdOperatorTaskStatus struct {
  // ObservedGeneration is the most recent generation observed for the resource.
  ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
  // Status of the last operation
  State TaskState `json:"taskStatus"`
  // Time at which operation is been triggered.
  InitiatedAt metav1.Time `json:"initiatedAt"`
  // LastError represents the errors when processing the task.
  LastErrors []LastError `json:"lastErrors,omitempty"`
  // Captures the last operation
  LastOperation *LastOperation `json:"lastOperation,omitempty"`
  Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

## Lifecycle

### Creation

Task(s) can be created by creating an instance of the `EtcdOperatorTask` CR specific to a task. 

### Execution

<< TO BE FILLED >> will introduce `pre-conditions` and how does one sequence or concurrently execute tasks.

### Deletion

`etcd-druid` will ensure that it garbage collects the custom resources and any other k8s resources created to realise the task as per `.spec.ttlSecondsAfterFinished` defined (or defaulted) in the task specification.

## Use Cases

### Recovery from permanent quorum loss

Currently identification and recovery from permament quorum loss is done manaually. The current proposal keeps the identification as manual requiring an operator to intervene and confirm that there is indeed a permanent quorum loss and automatic recovery is not possible. Once that is established then the authors now wish to automate the recovery which is currently a [multi-step process](https://github.com/gardener/etcd-druid/blob/master/docs/operations/recovery-from-permanent-quorum-loss-in-etcd-cluster.md) and needs to be done carefully. Automation would ensure that we eliminate any errors from an operator.

#### Task Config

We do not need any config for this task. When creating an instance of `EtcdOperatorTask` for this scenario, `.spec.config` will be set to nil (unset).

#### Pre-Conditions

* There should be a quorum loss in a multi-member etcd cluster. For a single-member etcd cluster running this task will effectively restore the single member from the backup full snapshot.
* There should not already be a permanent-quorum-loss-recovery-task running for the same etcd cluster.

### Trigger on-demand snapshot compaction

`etcd-druid` provides a configurable [etcd-events-threshold](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md#druid-flags). If and when this threshold is breached then a [snapshot compaction](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/02-snapshot-compaction.md) is triggered for this etcd cluster. However, there are scenarios where an ad-hoc full snapshot compaction is required. 

#### Possible scenarios

<< TODO: Ishan to fill in >>

#### Task Config

<< TODO: Ishan to fill in >>

#### Pre-Conditions

<< TODO: Ishan to fill in >>

### Trigger on-demand full/delta snapshots

`Etcd` custom resource provides an ability to set [FullSnapshotSchedule](https://github.com/gardener/etcd-druid/blob/master/api/v1alpha1/types_etcd.go#L158) which is currently defauled to run once in 24 hrs. [DeltaSnapshotPeriod](https://github.com/gardener/etcd-druid/blob/master/api/v1alpha1/types_etcd.go#L171) is also made configurable which defines the duration after which a delta snapshot will be taken. Based on the usage of etcd, it is possible that these durations are insufficient and it is required to trigger an ad-hoc full/delta snapshot. Once the request is triggered it will be serviced by the `leading-backup-restore` container.

#### Possible scenarios

* [Shoot hibernation](https://github.com/gardener/gardener/blob/master/docs/usage/shoot_hibernate.md): Every etcd cluster incurs an inherent cost of preserving the volumes even when the shoot is in hibernation. However it is possible to save costs by invoking this task to take a full snapshot before deleting the volumes.
* [Control Plane Migration](https://github.com/gardener/gardener/blob/master/docs/proposals/07-shoot-control-plane-migration.md): In gardener a seed cluster control plane can be moved to another seed cluster. To prevent data loss and faster restoration of the etcd cluster in the target seed, a full snapshot can be triggered for the etcd cluster in the source seed.
* It is possible that a full snapshot failed to be taken at the scheduled time. To ensure faster restoration one can invoke this task to trigger a full snapshot.

#### Task Config

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

#### Pre-Conditions

* Etcd cluster should have a quorum.
* There should not already be a trigger-ondemand-snapshot task running with the same `SnapshotType` for the same etcd cluster.

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




< --- OLDER VERSION --- >

### Use Cases

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

