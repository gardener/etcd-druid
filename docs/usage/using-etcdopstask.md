# Using EtcdOpsTask

`EtcdOpsTask` is a custom resource provided by the etcd-druid to facilitate various out-of-band tasks on Etcd clusters managed by the etcd-druid operator. It was initially proposed in [this design document](https://github.com/gardener/etcd-druid/blob/master/docs/proposals/05-etcdopstask.md).

## Overview

`EtcdOpsTask` allows operators to execute one-time operational tasks on an Etcd cluster. This includes operations like triggering on-demand snapshots (full or delta). The controller manages the task lifecycle, executing the operation and updating the task status to reflect success or failure.

## How Operators Can Use EtcdOpsTask

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
      timeoutSecondsFull: 900
      #timeoutSecondsDelta: 60
   ttlSecondsAfterFinished: 360   
  
```
* The above example creates an `EtcdOpsTask` named `on-demand-snapshot` that triggers an on-demand snapshot for the Etcd cluster referenced by `spec.etcdName` in the `metadata.namespace` namespace where the `EtcdOpstask` is created. Some tasks might not require an etcd reference and hence the field `spec.etcdName` is optional.
* The task will be automatically deleted after a Time-To-Live duration (TTL) completion as defined by `spec.ttlSecondsAfterFinished`. If not specified, the default TTL is 3600 seconds (1 hour).
* The spec field is immutable and is enforced via [`CEL`](https://kubernetes.io/docs/reference/using-api/cel/) expressions.

### Task Lifecycle

Once created, the `EtcdOpsTask` progresses through the following states:

1. **Pending**: Task is accepted but not yet acted upon
2. **InProgress**: Task is currently being executed
3. **Succeeded**: Task completed successfully
4. **Failed**: Task execution failed.
5. **Rejected**: Task was rejected as it failed to fulfill required pre-conditions. This set of pre-conditions varies from task to task.

Once a task reaches a terminal state (Succeeded, Failed, or Rejected), it will be eligible for automatic cleanup based on the specified TTL(`spec.ttlSecondsAfterFinished`).

#### Task Phases

The `Etcdopstask` controller follows a handler-driven flow with three phases:
- **Admit**: Validates preconditions (RBAC, duplicates, config). Task moves `Pending → InProgress` on success, or `Rejected` on failure.
- **Execute**: Performs the requested operation (e.g., on-demand snapshot). Task transitions to `Succeeded` or `Failed`.
- **Cleanup**: Handles cleanup of the task and any deployed resources. `EtcdopsTask` resource is deleted after TTL expiry.

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

`EtcdOpsTask` resources along with any dependent resources (Eg: Jobs, Pods etc.) are automatically cleaned up after a configurable TTL once they reach a terminal state (Succeeded, Failed, or Rejected).

### Supported Task Types

Currently supported task types:

#### OnDemandSnapshot

Triggers an on-demand snapshot outside the regular snapshot schedule.

**Snapshot Types:**
- **Full**: Creates a complete snapshot of the entire Etcd data store.
- **Delta**: Creates an incremental snapshot containing only the changes since the last snapshot.

**Prerequisites:**
- Backup must be enabled for the target Etcd cluster (`spec.backup.store` must be configured in the Etcd resource)
- The Etcd cluster must be in a ready state (all members healthy and running)
- No other `EtcdOpsTask` should be in progress for the same Etcd cluster.

**Configuration Options:**
- `type`: Specifies the snapshot type (`full` or `delta`)
- `isFinal`: Optional boolean flag to mark the snapshot as final (default: `false`). This field is applicable only for full snapshots.
- `timeoutSecondsFull`: Timeout in seconds for full snapshot operations (default: 900)
- `timeoutSecondsDelta`: Timeout in seconds for delta snapshot operations (default: 60)


#### RecoverFromQuorumLoss

Automates recovery of a multi-member etcd cluster that has permanently lost quorum. This task
implements the manual recovery steps documented in [Recovering Etcd Clusters](recovering-etcd-clusters.md):
wait for user confirmation → suspend reconciliation → disable component protection → scale STS
to 0 → delete PVCs → delete member Leases → patch ConfigMap to single-member → scale STS to 1
→ wait for pod ready → re-enable reconciliation (triggers scale-out) → wait for all members to
become ready → re-enable component protection.

> [!WARNING]
> This task deletes all PersistentVolumeClaims for the etcd cluster. All etcd data on those
> volumes is permanently lost. By default a backup must be available for the cluster to restore
> from; the backup-restore sidecar will automatically restore from the latest snapshot when the
> single-member pod starts. Do **not** use this task unless the cluster has truly lost quorum
> and cannot recover on its own.

**Prerequisites:**
- The target etcd cluster must be a multi-member cluster (`spec.replicas > 1`). For single-member
  clusters the backup-restore sidecar handles recovery automatically.
- Backup must be enabled for the target etcd cluster (`spec.backup.store` must be configured),
  **and** the etcd's `BackupReady` condition must currently report `True`. Recovery relies on
  the backup-restore sidecar restoring from the latest snapshot; without a Ready backup, recovery
  may restore stale data or have nothing to restore from at all. The admit check rejects the task
  if either condition is unmet. Both requirements can be explicitly waived with the `allowDataLoss`
  config flag — see below — at which point the cluster comes back up empty (no backup configured)
  or restores from whatever the latest available snapshot happens to be (backup configured but not
  Ready).
- The cluster must currently **not** be in a ready state (quorum loss has occurred).
- No other `EtcdOpsTask` should be in progress for the same etcd cluster.
- **Confirmation:** because this task is destructive, it requires explicit user confirmation
  before any state-mutating step runs. The first execution step blocks until the annotation
  `druid.gardener.cloud/confirm-recovery: "true"` is observed on the EtcdOpsTask. Until then, the task
  sits in `InProgress` state with `status.lastOperation.description` reading `"Awaiting user
  confirmation..."` and no changes are made to the cluster. The annotation may be included in the
  YAML at creation time (for unattended workflows) or applied after reviewing the created task.
  The match is strict: only the literal value `"true"` confirms; `"True"`, `"yes"`, or any other
  value keeps the gate closed.

**Configuration options:**
- `scaleDownTimeout`: Maximum time to wait for the StatefulSet to reach 0 replicas (default: `60s`). Accepts Go duration strings (e.g. `30s`, `2m`).
- `podReadyTimeout`: Maximum time to wait for the single-member pod (`<etcd-name>-0`) to become ready after scale-up (default: `180s`). Accepts Go duration strings.
- `etcdReadyTimeout`: Maximum time to wait for **all** etcd cluster members to become ready (the `AllMembersReady` condition reports `True`) after etcd-druid reconciliation has been re-enabled and the cluster has been scaled back out (default: `300s` / 5 minutes). Accepts Go duration strings.
- `allowDataLoss`: When `true`, permits the recovery to proceed even if no backup store is configured for the referenced Etcd, **or** if the configured backup is not in a Ready state (`BackupReady != True`). Setting this is an explicit acknowledgement that all existing etcd data may be lost or restored from a stale snapshot. When unset (or `false`), the admit check rejects the task if either of these conditions holds. Defaults to `false`.

**Example:**

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: EtcdOpsTask
metadata:
  name: recover-from-quorum-loss
  namespace: default
  # annotations:
  #   # Required: explicit confirmation before the destructive recovery proceeds.
  #   # The task pauses at the first execution step until this annotation is observed with value "true".
  #   # Uncomment when you are ready to run the recovery.
  #   druid.gardener.cloud/confirm-recovery: "true"
spec:
  etcdName: etcd-main
  ttlSecondsAfterFinished: 3600
  config:
    recoverFromQuorumLoss:
      scaleDownTimeout: 60s
      podReadyTimeout: 5m
      etcdReadyTimeout: 5m
```

**Expected behaviour:**
1. The task waits for the `druid.gardener.cloud/confirm-recovery: "true"` annotation to be present on the EtcdOpsTask. Until then, no changes are made to the cluster.
2. Once confirmed, the task suspends etcd-druid reconciliation and disables component protection on the target Etcd resource.
3. The StatefulSet is scaled to 0 and all PVCs and member leases are deleted.
4. The etcd ConfigMap is patched to configure a single-member cluster using member-0 only.
5. The StatefulSet is scaled to 1 and the task waits for the pod to become ready.
6. The backup-restore sidecar restores etcd data from the latest available snapshot (skipped if `allowDataLoss: true` and no backup store is configured — the cluster comes up empty).
7. Once the pod is ready, etcd-druid reconciliation is re-enabled and the cluster is scaled back to the original replica count.
8. The task waits for the `AllMembersReady` condition on the Etcd resource to report `True` — i.e. every replica has rejoined the cluster and reports healthy — before proceeding.
9. Component protection is re-enabled. The task reports `Succeeded` only after the cluster is verified back to a fully ready state.

If the task fails mid-execution, the `Cleanup` phase removes the suspend and disable-protection
annotations so the operator is not permanently locked out.




1. **Unique Names**: Use descriptive, unique names for tasks to avoid conflicts
2. **Monitor Status**: Check task status before creating duplicate operations. This will help avoid duplicate tasks.
