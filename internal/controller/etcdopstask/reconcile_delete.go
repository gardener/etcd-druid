package etcdopstask

import (
	"context"
	"fmt"

	"github.com/gardener/etcd-druid/api/core/v1alpha1"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	opsTask "github.com/gardener/etcd-druid/internal/task"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// triggerDeletionFlow handles the deletion flow for EtcdOpsTask resources.
func (r *Reconciler) triggerDeletionFlow(ctx context.Context, taskHandler opsTask.Handler, task *v1alpha1.EtcdOpsTask) ctrlutils.ReconcileStepResult {
	logger := taskHandler.Logger()
	logger.Info("Triggering deletion flow", "completed", task.IsCompleted(), "markedForDeletion", task.IsMarkedForDeletion())

	// If task is completed but not marked for deletion, wait for TTL expiry
	if task.IsCompleted() {

		if !task.HasTTLExpired() {
			logger.Info("Task completed but TTL not expired yet, will requeue after TTL", "ttlSeconds", task.Spec.TTLSecondsAfterFinished)
			return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), "Task completed, waiting for TTL to expire")
		}
		logger.Info("Task TTL expired, proceeding with deletion")
	}

	deletionStepFns := []StepFunction{
		{
			StepName: "CleanupTaskResources",
			StepFunc: r.cleanupTaskResources,
		},
		{
			StepName: "RemoveTaskFinalizer",
			StepFunc: r.removeTaskFinalizer,
		},
		{
			StepName: "RemoveTask",
			StepFunc: r.removeTask,
		},
	}
	for _, step := range deletionStepFns {
		logger.Info("Executing deletion step", "stepName", step.StepName)
		result := step.StepFunc(ctx, client.ObjectKeyFromObject(task), taskHandler)
		if ctrlutils.ShortCircuitReconcileFlow(result) {
			logger.Info("Short-circuiting deletion flow", "stepName", step.StepName, "result", result)
			return result
		}
	}
	logger.Info("Deletion flow completed for EtcdOpsTask")
	return ctrlutils.DoNotRequeue()
}

// cleanupTaskResources calls the Cleanup method of the task handler.
func (r *Reconciler) cleanupTaskResources(ctx context.Context, taskObjKey client.ObjectKey, taskHandler opsTask.Handler) ctrlutils.ReconcileStepResult {
	logger := r.logger.WithValues("namespace", taskObjKey.Namespace, "name", taskObjKey.Name)
	logger.Info("Cleaning up task resources")

	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	if task.Status.State != nil && *task.Status.State == v1alpha1.TaskStateRejected {
		logger.Info("Task is rejected, skipping cleanup")
		// no-op cleanup
		// Cases where task is rejected:
		// 1. Task is not supported by the controller
		// 2. Task admit failed
		// In these cases, we don't want to call the cleanup method of the task handler. Since there is no cleanup to be done, we can skip this step.
		if err := r.recordLastOperation(ctx, taskObjKey, v1alpha1.OperationPhaseCleanup, v1alpha1.OperationStateCompleted, ""); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		return ctrlutils.ContinueReconcile()
	}

	if err := r.recordLastOperation(ctx, taskObjKey, v1alpha1.OperationPhaseCleanup, v1alpha1.OperationStateInProgress, ""); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	result := taskHandler.Cleanup(ctx)
	if result == nil {
		err := druiderr.WrapError(
			fmt.Errorf("cleanup returned nil TaskResult; this is a bug in the TaskHandler implementation"),
			ErrNilResult,
			string(v1alpha1.OperationPhaseCleanup),
			"cleanup returned nil TaskResult; this is a bug in the TaskHandler implementation",
		)
		_ = r.recordLastError(ctx, taskObjKey, err)
		return ctrlutils.ReconcileWithError(err)
	}

	if result.Completed {
		if result.Error != nil {
			err := r.recordLastError(ctx, taskObjKey, result.Error)
			if err != nil {
				return ctrlutils.ReconcileWithError(err)
			}
			err = r.recordLastOperation(ctx, taskObjKey, v1alpha1.OperationPhaseCleanup, v1alpha1.OperationStateFailed, result.Description)
			if err != nil {
				return ctrlutils.ReconcileWithError(err)
			}
			return ctrlutils.ReconcileWithError(result.Error)
		}
		err := r.recordLastOperation(ctx, taskObjKey, v1alpha1.OperationPhaseCleanup, v1alpha1.OperationStateCompleted, result.Description)
		if err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	} else if result.Error != nil {
		err := r.recordLastError(ctx, taskObjKey, result.Error)
		if err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}

// removeTaskFinalizer removes the finalizer from the EtcdOpsTask resource.
func (r *Reconciler) removeTaskFinalizer(ctx context.Context, taskObjKey client.ObjectKey, _ opsTask.Handler) ctrlutils.ReconcileStepResult {
	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if controllerutil.ContainsFinalizer(task, FinalizerName) {
		controllerutil.RemoveFinalizer(task, FinalizerName)
		if err := r.client.Update(ctx, task); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}

// removeTask deletes the EtcdOpsTask resource from the cluster.
func (r *Reconciler) removeTask(ctx context.Context, taskObjKey client.ObjectKey, _ opsTask.Handler) ctrlutils.ReconcileStepResult {
	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if err := r.client.Delete(ctx, task); err != nil {
		return ctrlutils.ReconcileWithError(client.IgnoreNotFound(err))
	}
	return ctrlutils.DoNotRequeue()
}
