# EtcdOperatorTask Controller Design

## Custom Resource Golang API

A new Custom Resource named **EtcdOperatorTask** will be introduced to handle out-of-band tasks as initially proposed by [05-etcd-operator-tasks](https://github.com/gardener/etcd-druid/blob/main/docs/proposals/05-etcd-operator-tasks.md). This resource will be defined under the v1alpha1 API version and will be subject to change. Future updates will adhere to the [Kubernetes Deprecation Policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/).

The authors slightly modified CRD for implementation purposes.

- Spec made immutable. This is to ensure that the spec of the custom resource cannot be changed once it has been created.
- Config field of type `runtime.RawExtension` to handle the values for each of the operator task. 
- Renamed `ownerEtcdReference` to `etcdReference`. 
- Removed fields `Name` and `Reason` from `LastOperation` struct. Instead added `Description` field. 


<details>
<summary>Show EtcdOperatorTask CRD Go Definition</summary>

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=etcdoperatortasks,shortName=eot;eots,scope=Namespaced
// +kubebuilder:subresource:status
type EtcdOperatorTask struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   EtcdOperatorTaskSpec   `json:"spec"`
    
    Status EtcdOperatorTaskStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:XPreserveUnknownFields
// +kubebuilder:validation:Immutable
type EtcdOperatorTaskSpec struct {
    // +required
    Type v1alpha1.EtcdOperatorTaskType `json:"type"`

    // Config is task-specific key/value parameters.
    // +required
    Config runtime.RawExtension `json:"config,omitempty"` 

    // TTLSecondsAfterFinished controls how long the status+pod stays around.
    // +optional
    // +kubebuilder:validation:Minimum=0
    TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

    // +optional
    EtcdReference types.NamespacedName `json:"etcdReference,omitempty"`


}
```
</details>

The authors recommend leveraging the `runtime.RawExtension` type for the `Config` field to flexibly store task-specific configuration parameters. This will then be validated via an `admission webhook`. The `Config` field will be a JSON-encoded string that can be unmarshalled into a specific struct based on the task type.

This approach enables each task type to define and expose its own configuration schema via the API, making it straightforward for API consumers to provide the necessary parameters


#### Status

The authors propose the following fields for the Status (current state) of the `EtcdOperatorTask` custom resource to monitor the progress of the task.

<details>
<summary>Show EtcdOperatorTaskStatus Go Definition</summary>

```go
// EtcdOperatorTaskStatus is the status for a EtcdOperatorTask resource.
type EtcdOperatorTaskStatus struct {
  // State is the last known state of the task.
  State TaskState `json:"state"`
  // Time at which the task has moved from "pending" state to any other state.
  // +optional
  InitiatedAt metav1.Time `json:"initiatedAt"`
  // LastError represents the errors when processing the task. Will have a limit of 10 entries at a time.
  // +optional
  LastErrors []LastError `json:"lastErrors,omitempty"`
  // Captures the last operation status if task involves many stages.
  // +optional
  LastOperation *LastOperation `json:"lastOperation,omitempty"`
}

type LastOperation struct {
  // Status of the last operation, one of pending, progress, completed, failed.
  State OperationState `json:"state"`
  // LastTransitionTime is the time at which the operation state last transitioned from one state to another.
  LastTransitionTime metav1.Time `json:"lastTransitionTime"`
  // A human readable message indicating details about the last operation.
  Description string `json:"description"`
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

// TaskState represents the state of the task ie the status of the CR.
type TaskState string

const (
  TaskStateFailed TaskState = "Failed"
  TaskStatePending TaskState = "Pending"
  TaskStateRejected TaskState = "Rejected"
  TaskStateSucceeded TaskState = "Succeeded"
  TaskStateInProgress TaskState = "InProgress"
)

// OperationState represents the state of last operation run via the task.
type OperationState string

const (
  OperationStateInProgress OperationState = "InProgress"
  OperationStateCompleted OperationState = "Completed"
  OperationStateFailed OperationState = "Failed"
)
```
</details>

### Custom Resource YAML API

<details>
<summary>Show Example EtcdOperatorTask YAML</summary>

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
      description: <meaningful description>
      state: <task state as seen at the completion of last operation>
      lastTransitionTime: <time of transition to this state>
```
</details>

## TaskHandler Interface
Defines the execution contract for all out-of-band task handlers.
Each implementation must specify task admission, execution, and cleanup semantics.

TaskResult will be used to return the results of the methods for the interface implementations which will then be returned to the `Reconcile step functions`.

<details>
<summary>Show TaskHandler Interface Go Definition</summary>

```go
type TaskResult struct {
    Description string
    Error error
    RequeueAfter  time.Duration // Duration to requeue the task
    Completed bool
}

// OperatorTask defines the interface for task execution.
type TaskHandler interface {
    EtcdReference() types.NamespacedName // based on if etcdReference is set in the spec.
    Name() string
    Type() druidv1alpha1.EtcdOperatorTaskType
    Logger() logr.Logger
    // Checks if the task is permitted to run. This is a one-time gate; once passed, it is not checked again for the same task execution.
    Admit(ctx context.Context) *TaskResult  
    Run(ctx context.Context) *TaskResult // The Run method will have to check if the necessary pre conditions hold true upon each call. 
    Cleanup(ctx context.Context) *TaskResult // Will be triggered once the task is in a completed state.
}
```
</details>

- Admit: Checks if the task can run (preconditions). This is a one-time gate; once passed, it is not checked again for the same task execution in case if it is requeued for execution.
- Run: Executes the main logic. This will be called upon each requeue of the task until task is completed.
- Cleanup: Handles any post-completion cleanup. This will be called once the task is in a completed state or deleted after TTL has expired.

- `Name()`, `Type()`, `EtcdReference()`, these functions are used to retrieve the name, type, and etcd reference of the task for collecting the task level metrics.

---

## Creating the TaskHandler Instance

To support new out-of-band tasks, the `createTaskHandlerInstance` function is used to instantiate the appropriate task handler. This function is invoked within the `Reconcile` method of the `TaskReconciler`. The task handler is selected based on the `type` field specified in the `EtcdOperatorTask` custom resource.

<details>
<summary>Show TaskHandler Instance Creation</summary>

```go
// Add a new TaskHandler as shown below:
func (r *Reconciler) createTaskHandlerInstance(task *v1alpha1.EtcdOperatorTask) (tasks.TaskHandler, error) {
    switch task.Spec.Type {
    case v1alpha1.EtcdOperatorTaskTypeOnDemandSnapshot:
        return ondemandsnapshot.New(r.client, r.logger, task) // New() initializes the TaskHandler.
    // Add more cases for other task types
    default:
        return nil, fmt.Errorf("unsupported task type: %s", task.Spec.Type)
    }
}
```
</details>

### Adding New Task Types

To add support for a new task type:
1. Implement the `TaskHandler` interface for the new task.
2. Add a case for the new task type in the `createTaskHandlerInstance` function.


## Reconciliation Flow

The core reconciliation logic:

<details>
<summary>Show TaskReconciler Reconcile Function</summary>

```go
func (r *TaskReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    // The below block checks for the OperatorTask.
    task := &v1alpha1.EtcdOperatorTask{}
	if err := r.client.Get(ctx, req.NamespacedName, task); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	logger := r.logger.WithValues("runId", string(controller.ReconcileIDFromContext(ctx)))
    // We have defined the type of the field spec.config as runtime.RawExtension. If that's invalid, the below flow is triggered.
	if task.Status.State == v1alpha1.TaskStateRejected {
		return r.triggerRejectionDeletionFlow(ctx, task, logger).ReconcileResult()
	}

    // create a instance of TaskHandler
    taskHandlerInstance := r.createTaskHandlerInstance(task)
	
    // Triggers the deletion flow in case the task is in a completed state or if it has been marked for deletion.
	if task.IsCompleted() || task.IsMarkedForDeletion() {
		return r.triggerDeletionFlow(ctx, taskHandlerInstance, task).ReconcileResult()
	}
    // triggers the task execution flow.
	return r.reconcileTask(ctx, client.ObjectKeyFromObject(task), taskHandlerInstance).ReconcileResult()
}
```
> **NOTE**: Invalid Config: Set State: Rejected.

```go
// reconcileTask manages finalizer, execution, and status updates.
func (r *Reconciler) reconcileTask(ctx context.Context, taskObjKey client.ObjectKey, taskHandler tasks.TaskHandler) ctrlutils.ReconcileStepResult {
    reconcileStepFns := []reconcileFn{
        r.recordTaskStartOperation,
        r.ensureTaskFinalizer,
        r.updateStatusObservedGeneration,
        r.transitionToPendingState, // updates the status field. Skipped if already done.
        r.admitTaskPreconditions, // calls the implemented interface method.
        r.transitionToInProgressState, // updates the status. Skipped if already marked.
        r.runTask, // calls the implemented interface method.
    }

    for _, step := range reconcileStepFns {
        ctx.Logger.Info("Executing step", "step", step)
        result := step(ctx, taskObjKey, taskHandler)
        if ctrlutils.ShortCircuitReconcileFlow(result) {
            return result
        }
    }

    return ctrlutils.ReconcileAfter(task.Spec.TTLSecondsAfterFinished, "Task completed, waiting for TTL to expire")
}
```
</details>

### Deletion Flow

<details>
<summary>Show Task Deletion Flow</summary>

```go
func (r *Reconciler) triggerTaskDeletionFlow(
    ctx context.Context,
    logger logr.Logger,
    taskObjKey client.ObjectKey,
    taskHandler tasks.TaskHandler,
) ctrlutils.ReconcileStepResult {
    if task.IsCompleted() && !task.IsMarkedForDeletion() {
        if !task.TTLHasExpired() {
            return ctrlutils.ReconcileAfter(task.Spec.TTLSecondsAfterFinished, "Task completed, waiting for TTL to expire")
        }
    }

    deletionStepFns := []reconcileFn{
        r.recordTaskDeletionStartOperation, // update the status. Skip if already done.
		r.cleanupTaskResources, // Cleanup of any kind of resources used to run the task. Runs the interface method.
		r.recordTaskDeletionSuccessOperation,
		r.removeTaskFinalizer,
		r.removeTask, // Removes the CR.
    }
    for _, fn := range deletionStepFns {
        result := fn(ctx, taskObjKey, taskHandler)
        if ctrlutils.ShortCircuitReconcileFlow(result) {
            return r.recordTaskIncompleteDeletionOperation(ctx, logger, taskObjKey, result)
        }
    }
    return ctrlutils.DoNotRequeue()
}
```
</details>


## Example OperatorTask

<details>
<summary>Show Example TaskHandler Implementation</summary>

```go
// OnDemandSnapshot implements the TaskHandler interface for on-demand snapshots.
type OnDemandSnapshot struct {
	client        client.Client
    httpClient    *http.Client
	logger        logr.Logger
	name          string
	etcdReference types.NamespacedName
	config        *Config
}
// the config provided in the spec will be parsed as follows:
const ERR_PRECONDITION_CHECK_ON_DEMAND_SNAPSHOT druidv1alpha1.ErrorCode = "ERR_PRECONDITION_CHECK_ON_DEMAND_SNAPSHOT"


func New(k8sclient client.Client, logger logr.Logger, task *v1alpha1.EtcdOperatorTask) (operatortask.OperatorTask, error) {
     
    config,err := decodeOnDemandSnapshotConfig(task.Spec.Config)
    if err != nil {
        return nil, druiderr.WrapError(err, operatortask.ERR_INVALID_CONFIG, operationNew, "failed to decode config for OnDemandSnapshot")
    }
    
	return &OnDemandSnapshot{
		client:        k8sclient,
		logger:        logger,
		name:          task.Name,
        httpClient:    &http.Client{Timeout: config.Timeout},
		etcdReference: types.NamespacedName(*task.Spec.EtcdRef),
		config:        config,
	}, nil
}

func (o *OnDemandSnapshot) Run(ctx context.Context) *operatortask.TaskResult {
	// Implement the snapshot logic
	etcd := &v1alpha1.Etcd{}
	if err := o.client.Get(ctx, o.etcdReference, etcd); err != nil {
		return &operatortask.TaskResult{Description: "Failed to get Etcd resource", Error: err}
	}

	snapType := "full"
	if o.config.SnapshotType != nil && *o.config.SnapshotType != "" {
		snapType = *o.config.SnapshotType
	}

	o.logger.Info("Snapshot config", "snapshotType", snapType, "timeout", o.config.Timeout)

	url := fmt.Sprintf("http://%s.%s:%d/snapshot/full?final=true", v1alpha1.GetClientServiceName(etcd.ObjectMeta), etcd.Namespace, ptr.Deref(etcd.Spec.Backup.Port, common.DefaultPortEtcdBackupRestore))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return &operatortask.TaskResult{
			Description: fmt.Sprintf("Failed to create HTTP request for %s", url),
			Error:       err,
		}
	}
	o.logger.Info("Triggering snapshot", "url", url)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return &operatortask.TaskResult{Description: "Snapshot HTTP request failed", Error: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &operatortask.TaskResult{Description: fmt.Sprintf("Snapshot failed, status: %s", resp.Status), Completed: false}
	}
	return &operatortask.TaskResult{Description: "Triggered snapshot on pod", Completed: true}
}

func (o *OnDemandSnapshot) Admit(ctx context.Context) *operatortask.TaskResult {
	// Implement the admission logic (e.g., check if Etcd is ready)
	etcd := &v1alpha1.Etcd{}
	if err := o.client.Get(ctx, o.etcdReference, etcd); err != nil {
		return &operatortask.TaskResult{Description: "Failed to get Etcd resource", Error: err}
	}
	if etcd.Status.ReadyReplicas < 1 {
		return &operatortask.TaskResult{
			Description: "No ready replicas for Etcd",
			Error: druiderr.WrapError(nil,
				ERR_PRECONDITION_CHECK_ON_DEMAND_SNAPSHOT,
				operationAdmit,
				fmt.Sprintf("No ready replicas for Etcd: %v", o.etcdReference)),
			Completed: true,
		}
	}
	return &operatortask.TaskResult{Description: "Admission successful", Completed: true}
}

func (o *OnDemandSnapshot) Cleanup(ctx context.Context) *operatortask.TaskResult {
	// No-op for snapshot tasks
	return &operatortask.TaskResult{Description: "Cleanup done", Completed: true}
}


```
</details>

#### Example YAML file for an on-demand snapshot task:

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: EtcdOperatorTask
metadata:
  name: on-demand-snapshot-task
  namespace: default
spec:
  type: OnDemandSnapshot
  etcdRef:
    name: etcd-test
    namespace: default
  config: '{"snapshotType":"full","timeoutSeconds":60}'
  ttlSecondsAfterFinished: 600
```

### Registering a New OperatorTask
1. Implement the TaskHandler and its factory function (constructor).
2. Add the new TaskHandler instance to the `createTaskHandlerInstance` function in the `Reconcile` method as shown below:
   ```go
    func (r *Reconciler) createTaskHandlerInstance(task *v1alpha1.EtcdOperatorTask) (tasks.TaskHandler) {
         switch task.Spec.Type {
         case v1alpha1.<NewTaskType>:
              return <NewTaskType>.New(r.client, r.logger, task)
         // Add more cases for other task types
         default:
         }
    }
   ```

### Benefits
- **Extensible:** New OperatorTasks can be added without modifying the core creation logic.
- **Decoupled:** OperatorTask implementations are independent from the reconciler logic.


## Next Steps:

Designing the rest HTTP Endpoints for the etcd-backup-restore server to expose the etcd maintenance API.

- With support for sync/async.
- The snapshot API must follow the same pattern.


By this way, EtcdOperatorTask can trigger the etcd maintenance API and wait for the result by requeuing the task.

---