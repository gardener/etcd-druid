# Using EtcdOpsTask

`EtcdOpsTask` is a custom resource provided by the Etcd Druid project to facilitate various out-of-band tasks on Etcd clusters managed by the Druid operator. It was initially proposed in [this design document](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/05-etcdopstask.md).

## Overview

`EtcdOpsTask` allows operators to execute one-time operational tasks on an Etcd cluster. This includes operations like triggering on-demand snapshots (full or delta). The controller manages the task lifecycle, executing the operation and updating the task status to reflect success or failure.

## How Operators Can Use EtcdOpsTask
> [!NOTE] 
> As of v0.34.0, `EtcdOpsTask` primarily supports on-demand snapshot operations. Future versions may introduce additional task types.

### Creating an EtcdOpsTask

To trigger an operational task, create an `EtcdOpsTask` resource in the same namespace as the target Etcd cluster. The `spec.config` field specifies the type of operation to perform. This field is of union type, allowing one and only one operation to be defined per task:

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: EtcdOpsTask
metadata:
  name: on-demand-snapshot
  namespace: default
spec:
  etcdName: etcd-main
  config:
    onDemandSnapshot:
      type: full  # or delta
      #isFinal: false
      timeoutSecondsFull: 480
      #timeoutSecondsDelta: 60
   ttlSecondsAfterFinished: 360   
  
```
* The above example creates an `EtcdOpsTask` named `on-demand-snapshot` that triggers an on-demand snapshot for the Etcd cluster referenced by `spec.etcdName` in the `metadata.namespace` namespace where the etcdopstask is created. Some tasks might not require an etcd reference and hence the field `spec.etcdName` is optional.
* The task will be automatically deleted after a Time-To-Live duration (TTL) completion as defined by `spec.ttlSecondsAfterFinished`. If not specified, the default TTL is 3600 seconds (1 hour).
* The spec field is immutable and is enforced via [`CEL`](https://kubernetes.io/docs/reference/using-api/cel/) expressions.

### Task Lifecycle

Once created, the EtcdOpsTask progresses through the following states:

1. **Pending**: Task is accepted but not yet acted upon
2. **InProgress**: Task is currently being executed
3. **Succeeded**: Task completed successfully
4. **Failed**: Task execution failed.
5. **Rejected**: Task was rejected as it failed to fulfill required pre-conditions. This set of pre-conditions varies from task to task.

Once a task reaches a terminal state (Succeeded, Failed, or Rejected), it will be eligible for automatic cleanup based on the specified TTL(`spec.ttlSecondsAfterFinished`).

#### Task Phases

The etcdopstask controller follows a handler-driven flow with three phases:
- **Admit**: Validates preconditions (RBAC, duplicates, config). Task moves `Pending → InProgress` on success, or `Rejected` on failure.
- **Execute**: Performs the requested operation (e.g., on-demand snapshot). Task transitions to `Succeeded` or `Failed`.
- **Cleanup**: Handles cleanup of the task and any deployed resources. EtcdopsTask resource is deleted after TTL expiry.
### Monitoring Task Status

Check the task status to monitor progress:

```bash
kubectl get etcdopstask <etcdopstask name> -o yaml
```

The status section provides:
- **State**: Current task [state](#task-lifecycle)
- **LastOperation**: Last reconciliation operation performed
    - **Type**: Type of operation i.e whether it is in the Admit phase or Execute phase or Cleanup phase
    - **State**: State of the last operation type. A combination of Type and State can help identify the status of the operation. Eg:
        - Type: Execute, State: Failed indicates that the task execution has failed
        - Type: Admit, State: InProgress indicates that the task is running the pre-conditions.
    - **RunID**: Unique identifier for the operation run
    - **LastUpdateTime**: Timestamp of the last update to the LastOperation field
- **LastError**: Error details if any while running the LastOperation.
    - **Code**: Error code
    - **Description**: Detailed error message
    - **ObservedAt**: Timestamp when the error was observed
- **LastTransitionTime**: Timestamp of the last state transition
- **StartedAt**: Timestamp when the task execution started


Example status:

```yaml
status:
  state: InProgress
  lastTransitionTime: "2025-12-03T23:31:38Z"
  startedAt: "2025-12-03T23:31:32Z"
  lastOperation:
    description: Task Execution phase is in Progress
    lastUpdateTime: "2025-12-03T23:31:50Z"
    runID: e094cd10-e756-4132-b52a-50c164d1df2a
    state: InProgress
    type: Execute
   lastErrors:
   - code: ERR_EXECUTE_HTTP_REQUEST
     description: '[Operation: Execution, Code: ERR_EXECUTE_HTTP_REQUEST] message:
        failed to execute HTTP request, cause: Post "http://etcd-test-client.default:8080/snapshot/full":
        dial tcp 10.96.99.172:8080: connect: connection refused'
     observedAt: "2025-12-03T23:31:53Z"
  
```

### Task Cleanup

EtcdOpsTask resources along with any dependent resources (Eg: Jobs, Pods etc.) are automatically cleaned up after a configurable TTL once they reach a terminal state (Succeeded, Failed, or Rejected).

### Supported Task Types

Currently supported task types:

- **OnDemandSnapshot**: Triggers an on-demand backup snapshot
  - `Full`: Creates a complete snapshot
  - `Delta`: Creates an incremental snapshot

### Best Practices

1. **Unique Names**: Use descriptive, unique names for tasks to avoid conflicts
2. **Monitor Status**: Check task status before creating duplicate operations. This will help avoid duplicate tasks.
