# Implementing a New EtcdOpsTask
EtcdOpsTask is designed to be extensible, allowing developers to implement new task types as needed. This guide outlines the steps to create and integrate a new EtcdOpsTask type into the Etcd Druid operator.

## Controller Architecture Overview
- A single reconciler (as defined in `internal/controller/etcdopstask`) drives all EtcdOpsTask types.
- The reconciler selects the concrete handler based on `spec.config` (one-of union) via the task handler registry.
- This keeps lifecycle logic unified while allowing task-specific behavior to live in dedicated handlers.

## Steps to Implement a New EtcdOpsTask
1. **Define the Task Spec and Status**:
   - Extend the `EtcdOpsTaskConfig` under the `EtcdOpsTaskSpec` in [api/etcdopstask.go](https://github.com/gardener/etcd-druid/tree/master/api/core/v1alpha1/etcdopstask.go) to include a new field for your task configuration under the `config` union type.
   - Add the new task’s config type as a dedicated file under [`api/core/v1alpha1/`](https://github.com/gardener/etcd-druid/tree/master/api/core/v1alpha1) (similar to `ondemandsnapshot.go`). This keeps each task’s CRD schema isolated and maintainable.
   - Use either `kubebuilder markers` or `CEL` expressions to enforce immutability and validation rules on the new config fields as per requirements.

2. **Implement the Task Handler**:
    - In `internal/controller/etcdopstask/handler/`, create a new package for your task (e.g., `mynewtask`).
    - Every etcdpstask handler must implement the `Handler` interface as defined in `internal/controller/etcdopstask/handler/types.go`. The interface has three methods which is expected to be implemented:
      - `Admit(ctx context.Context) error`: Validates preconditions for the task.
      - `Execute(ctx context.Context) error`: Contains the core logic to perform the task.
      - `Cleanup(ctx context.Context) error`: Cleans up any resources created during task execution.
    >NOTE: 
    > 1) The `Admit` method is run when the task is created to validate preconditions. If it fails, the task is marked as `Rejected`. If it succeeds, the task moves to `InProgress` state and this method is not called again.
    > 2) The `Execute` method is called repeatedly in case of timeouts/transient errors until the task execution is successful or a non-retryable error occurs. Once this is done the task moves to `Succeeded` or `Failed` state respectively.  
    > 3) The `Cleanup` method is called when the task reaches a terminal state (Succeeded/Failed/Rejected) to clean up any resources created during the task execution. This can be a no-op if there are no resources to clean up. (Refer the [on-demand-snapshot handler implementation](https://github.com/gardener/etcd-druid/tree/master/internal/controller/etcdopstask/handler/ondemandsnapshot/ondemandsnapshot.go) for reference).
    > 4) Refer the etcdopstask [api](https://gitthub.com/gardener/etcd-druid/tree/master/api/core/v1alpha1/etcdopstask.go) for details regarding the etcdopstask state transitions and status fields.
    - Transient error handling is to be done via setting the `requeue` field in the return type from the corresponsing methods with appropriate `Error` and `Description` field clearly indicating the reason. Common error codes are defined in [types.go](https://github.com/gardener/etcd-druid/tree/master/internal/controller/etcdopstask/handler/types.go). Introduce new error codes if necessary.
    This will then be reflected in the `LastOperation` and/or the `LastErrors` field in the `etcdopstask.status` field accordingly.
    - Once the `requeue` field is set to false, the task will move to the next phase or terminal state as applicable.
    - Add a constructor function in the new handler package that returns an instance of the handler. The constructor should accept necessary dependencies like clients, loggers, etc., similar to existing handlers. This will then be used in the next step to register the handler with the registry. Refer the [on-demand-snapshot handler constructor](https://github.com/gardener/etcd-druid/tree/master/internal/controller/etcdopstask/handler/ondemandsnapshot/ondemandsnapshot.go#L56-L65) for reference.
3. **Register the Handler**:
  - The EtcdOpsTask reconciler maintains a task handler registry. Extend `DefaultTaskHandlerRegistry()` in `internal/controller/etcdopstask/reconciler.go` to register your new handler, e.g. `registry.Register("MyNewTask", mynewtask.New)`.
  - Update `getTaskHandler(...)` in the same file to select the new handler based on the corresponding `spec.config` field (one-of union), similar to the `OnDemandSnapshot` case.
