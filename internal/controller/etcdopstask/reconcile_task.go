package etcdopstask

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	opsTask "github.com/gardener/etcd-druid/internal/task"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// ErrDuplicateTask represents the error in case of a duplicate task for the same etcd.
	ErrDuplicateTask druidv1alpha1.ErrorCode = "ERR_DUPLICATE_TASK"
	// ErrNilResult represents the error in case a task handler returns a nil result.
	ErrNilResult druidv1alpha1.ErrorCode = "ERR_NIL_RESULT"
)

// reconcileTask manages the lifecycle of an EtcdOperatorTask resource.
// It executes a series of step functions to ensure the task is processed correctly.
func (r *Reconciler) reconcileTask(ctx context.Context, taskObjKey client.ObjectKey, taskHandler opsTask.Handler) ctrlutils.ReconcileStepResult {
	logger := taskHandler.Logger().WithValues("op", "reconcileTask")
	logger.Info("Triggering task execution flow")
	steps := []StepFunction{
		{StepName: "ensureTaskFinalizer", StepFunc: r.ensureTaskFinalizer},
		{StepName: "transitionToPendingState", StepFunc: r.transitionToPendingState},
		{StepName: "admitTask", StepFunc: r.admitTask},
		{StepName: "transitionToInProgressState", StepFunc: r.transitionToInProgressState},
		{StepName: "runTask", StepFunc: r.runTask},
	}
	for _, step := range steps {
		logger.Info("Executing step", "step", step.StepName)
		result := step.StepFunc(ctx, taskObjKey, taskHandler)
		if ctrlutils.ShortCircuitReconcileFlow(result) {
			logger.Info("Short-circuiting reconciliation", "step", step.StepName, "result", result)
			return result
		}
	}
	logger.Info("Task execution flow completed")
	return ctrlutils.ContinueReconcile()
}

// ensureTaskFinalizer checks if the EtcdOperatorTask has the finalizer.
// If not, it adds the finalizer and updates the task status.
func (r *Reconciler) ensureTaskFinalizer(ctx context.Context, taskObjKey client.ObjectKey, _ opsTask.Handler) ctrlutils.ReconcileStepResult {
	meta := &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EtcdOpsTask",
			APIVersion: druidv1alpha1.SchemeGroupVersion.String(),
		},
	}
	meta.SetNamespace(taskObjKey.Namespace)
	meta.SetName(taskObjKey.Name)
	if err := r.client.Get(ctx, taskObjKey, meta); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if controllerutil.ContainsFinalizer(meta, FinalizerName) {
		return ctrlutils.ContinueReconcile()
	}
	patch := client.MergeFrom(meta.DeepCopy())
	controllerutil.AddFinalizer(meta, FinalizerName)
	if err := r.client.Patch(ctx, meta, patch); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

// transitionToPendingState sets the task.status.state to Pending if not already set.
func (r *Reconciler) transitionToPendingState(ctx context.Context, taskObjKey client.ObjectKey, _ opsTask.Handler) ctrlutils.ReconcileStepResult {
	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	if task.Status.State != nil {
		return ctrlutils.ContinueReconcile()
	}
	// Set the task state to Pending
	// and update the LastTransitionTime.
	if err := r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStatePending); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

// admitTask checks if the task is in a pending state.
// If so, it updates the LastOperation to admit and invokes the task handler's Admit method.
// If the admit operation is in progress, it requeues the task.
// If the admit operation fails, it updates the task state to Rejected and sets the LastOperation to failed.
// If the admit operation succeeds, it updates the task.status.state to InProgress.
// If the task is already in a completed state, it skips the admit operation.
func (r *Reconciler) admitTask(ctx context.Context, taskObjKey client.ObjectKey, taskHandler opsTask.Handler) ctrlutils.ReconcileStepResult {
	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	// check if there is a duplicate task present for the same etcd and same namespace:
	// Filter based on state of the resource
	var etcdOpsTaskList druidv1alpha1.EtcdOpsTaskList
	if err = r.client.List(ctx, &etcdOpsTaskList, client.InNamespace(taskHandler.EtcdReference().Namespace)); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	for _, existingTask := range etcdOpsTaskList.Items {
		if existingTask.Spec.EtcdRef != nil && task.Spec.EtcdRef != nil &&
			existingTask.Spec.EtcdRef.Name == task.Spec.EtcdRef.Name &&
			existingTask.Spec.EtcdRef.Namespace == task.Spec.EtcdRef.Namespace {
			if !existingTask.IsCompleted() && existingTask.Name != task.Name {
				err := druiderr.WrapError(
					fmt.Errorf("an EtcdOpsTask for etcd %s/%s is already in progress (task: %s)", existingTask.Spec.EtcdRef.Namespace, existingTask.Spec.EtcdRef.Name, existingTask.Name),
					ErrDuplicateTask,
					string(druidv1alpha1.OperationPhaseAdmit),
					"duplicate EtcdOpsTask for the same etcd is already in progress",
				)
				_ = r.recordLastError(ctx, taskObjKey, err)
				_ = r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationPhaseAdmit, druidv1alpha1.OperationStateFailed, err.Error())
				_ = r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStateRejected)
				return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), "Duplicate task found, requeueing for cleanup")
			}
		}
	}

	if task.Status.State != nil && *task.Status.State != druidv1alpha1.TaskStatePending {
		return ctrlutils.ContinueReconcile()
	}
	if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationPhaseAdmit, druidv1alpha1.OperationStateInProgress, ""); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	result := taskHandler.Admit(ctx)
	if result == nil {
		err := druiderr.WrapError(
			fmt.Errorf("admit returned nil TaskResult; this is a bug in the TaskHandler implementation"),
			ErrNilResult,
			string(druidv1alpha1.OperationPhaseAdmit),
			"admit returned nil TaskResult; this is a bug in the TaskHandler implementation",
		)
		_ = r.recordLastError(ctx, taskObjKey, err)
		return ctrlutils.ReconcileWithError(err)
	}
	if !result.Completed {
		if result.Error != nil {
			err = r.recordLastError(ctx, taskObjKey, result.Error)
			if err != nil {
				return ctrlutils.ReconcileWithError(err)
			}
			return ctrlutils.ReconcileWithError(result.Error)
		}
		requeue := result.RequeueAfter
		if requeue == 0 {
			requeue = r.config.RequeueInterval.Duration
		}
		return ctrlutils.ReconcileAfter(requeue, "Task admit in progress")
	}
	if result.Error != nil {
		// Admission failed, mark as rejected
		err = r.recordLastError(ctx, taskObjKey, result.Error)
		if err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationPhaseAdmit, druidv1alpha1.OperationStateFailed, result.Description); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		if err := r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStateRejected); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), "Task failed to admit")
	}
	return ctrlutils.ContinueReconcile()
}

// transitionToInProgressState sets the task.status.state to InProgress if not already set.
func (r *Reconciler) transitionToInProgressState(ctx context.Context, taskObjKey client.ObjectKey, taskHandler opsTask.Handler) ctrlutils.ReconcileStepResult {
	logger := taskHandler.Logger().WithValues("op", "transitionToInProgressState")
	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		logger.Error(err, "Failed to get task")
		return ctrlutils.ReconcileWithError(err)
	}

	if task.Status.State != nil && *task.Status.State == druidv1alpha1.TaskStatePending {
		logger.Info("Transitioning task state from Pending to InProgress")
		if err := r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStateInProgress); err != nil {
			logger.Error(err, "Failed to record task state as InProgress")
			return ctrlutils.ReconcileWithError(err)
		}
	} else {
		logger.Info("Task state is not Pending, skipping transition", "currentState", task.Status.State)
	}
	return ctrlutils.ContinueReconcile()
}

// runTask executes the task handler's Run method.
// It updates the task status based on the result of the run operation.
// If the task is completed, it updates the LastOperation to completed or failed.
// If the task is still in progress, it requeues the task for further processing.
// If the task fails, it updates the task status to failed and sets the LastOperation to failed.
// If the task succeeds, it updates the task status to succeeded and sets the LastOperation to completed.
// If the task is already in a completed state, it skips the run operation.
func (r *Reconciler) runTask(ctx context.Context, taskObjKey client.ObjectKey, taskHandler opsTask.Handler) ctrlutils.ReconcileStepResult {
	_, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationPhaseRunning, druidv1alpha1.OperationStateInProgress, ""); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	result := taskHandler.Run(ctx)
	if result == nil {
		err := druiderr.WrapError(
			fmt.Errorf("run returned nil TaskResult; this is a bug in the TaskHandler implementation"),
			ErrNilResult,
			string(druidv1alpha1.OperationPhaseRunning),
			"run returned nil TaskResult; this is a bug in the TaskHandler implementation",
		)
		_ = r.recordLastError(ctx, taskObjKey, err)
		return ctrlutils.ReconcileWithError(err)
	}

	if result.Completed {
		if result.Error != nil {
			// Task failed
			err = r.recordLastError(ctx, taskObjKey, result.Error)
			if err != nil {
				return ctrlutils.ReconcileWithError(err)
			}
			if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationPhaseRunning, druidv1alpha1.OperationStateFailed, result.Description); err != nil {
				return ctrlutils.ReconcileWithError(err)
			}
			if err := r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStateFailed); err != nil {
				return ctrlutils.ReconcileWithError(err)
			}
		} else {
			// Task succeeded
			if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationPhaseRunning, druidv1alpha1.OperationStateCompleted, result.Description); err != nil {
				return ctrlutils.ReconcileWithError(err)
			}
			if err := r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStateSucceeded); err != nil {
				return ctrlutils.ReconcileWithError(err)
			}
		}

		task, err := r.getTask(ctx, taskObjKey)
		if err != nil {
			return ctrlutils.ReconcileWithError(err)
		}

		if task.HasTTLExpired() {
			return ctrlutils.ReconcileWithRequeue("Task completed, TTL expired, Requeueuing for cleanup")
		}

		return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), "Task completed, waiting for TTL to expire")
	}

	if result.Error != nil {
		err = r.recordLastError(ctx, taskObjKey, result.Error)
		if err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	}

	requeue := result.RequeueAfter
	if requeue == 0 {
		requeue = r.config.RequeueInterval.Duration
	}
	return ctrlutils.ReconcileAfter(requeue, "Task in progress")
}
