// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// ErrDuplicateTask represents the error in case of a duplicate task for the same etcd.
	ErrDuplicateTask druidv1alpha1.ErrorCode = "ERR_DUPLICATE_TASK"
)

// reconcileTask manages the lifecycle of an EtcdOpsTask resource.
// It executes a series of step functions to ensure the task is processed correctly.
func (r *Reconciler) reconcileTask(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("op", "reconcileTask")
	logger.Info("Triggering task execution flow")

	reconcileStepFns := []reconcileFn{
		r.ensureTaskFinalizer,
		r.transitionToPendingState,
		r.admitTask,
		r.transitionToInProgressState,
		r.runTask,
	}

	for _, fn := range reconcileStepFns {
		if result := fn(ctx, logger, taskObjKey, taskHandler); ctrlutils.ShortCircuitReconcileFlow(result) {
			return result
		}
	}
	logger.Info("Task execution flow completed")
	return ctrlutils.ContinueReconcile()
}

// ensureTaskFinalizer checks if the EtcdOpsTask has the finalizer.
// If not, it adds the finalizer and updates the task status.
func (r *Reconciler) ensureTaskFinalizer(ctx context.Context, _ logr.Logger, taskObjKey client.ObjectKey, _ handler.Handler) ctrlutils.ReconcileStepResult {
	meta := &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EtcdOpsTask",
			APIVersion: druidv1alpha1.SchemeGroupVersion.String(),
		},
	}
	if err := r.client.Get(ctx, taskObjKey, meta); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if controllerutil.ContainsFinalizer(meta, FinalizerName) {
		return ctrlutils.ContinueReconcile()
	}
	if err := kubernetes.AddFinalizers(ctx, r.client, meta, FinalizerName); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

// transitionToPendingState sets the task.status.state to Pending if not already set.
func (r *Reconciler) transitionToPendingState(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, _ handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "transitionToPendingState")
	logger.Info("Executing step: transitionToPendingState")

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
func (r *Reconciler) admitTask(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "admitTask")
	logger.Info("Executing step: admitTask")

	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	// If the task is not in Pending state, skip the admit step.
	if task.Status.State != nil && *task.Status.State != druidv1alpha1.TaskStatePending {
		return ctrlutils.ContinueReconcile()
	}
	
	var etcdOpsTaskList druidv1alpha1.EtcdOpsTaskList
	if err = r.client.List(ctx, &etcdOpsTaskList, client.InNamespace(task.Namespace)); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	for _, existingTask := range etcdOpsTaskList.Items {
		if existingTask.Spec.EtcdName != nil && task.Spec.EtcdName != nil &&
			*existingTask.Spec.EtcdName != "" && *task.Spec.EtcdName != "" &&
			*existingTask.Spec.EtcdName == *task.Spec.EtcdName {
			if !existingTask.IsCompleted() && existingTask.Name != task.Name {
				err := druiderr.WrapError(
					fmt.Errorf("an EtcdOpsTask for etcd %s/%s is already in progress (task: %s)", existingTask.Namespace, *existingTask.Spec.EtcdName, existingTask.Name),
					ErrDuplicateTask,
					string(druidv1alpha1.OperationTypeAdmit),
					"duplicate EtcdOpsTask for the same etcd is already in progress",
				)

				_ = r.recordLastError(ctx, taskObjKey, err)
				_ = r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationTypeAdmit, druidv1alpha1.OperationStateFailed, err.Error())
				// State recording is critical and must succeed
				if err := r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStateRejected); err != nil {
					return ctrlutils.ReconcileWithError(err)
				}
				return ctrlutils.ReconcileAfter(1*time.Second, "Duplicate task found, handing to deletion flow")
			}
		}
	}

	if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationTypeAdmit, druidv1alpha1.OperationStateInProgress, ""); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	result := taskHandler.Admit(ctx)

	if !result.Completed {
		if result.Error != nil {
			if err := r.recordLastError(ctx, taskObjKey, result.Error); err != nil {
				wrappedErr := errors.Wrapf(result.Error, "task admission failed and failed to record error: %v", err)
				return ctrlutils.ReconcileWithError(wrappedErr)
			}
			return ctrlutils.ReconcileWithError(result.Error)
		}
		return ctrlutils.ReconcileAfter(r.config.RequeueInterval.Duration, "Task admit in progress")
	}
	if result.Error != nil {
		// Admission failed, mark as rejected
		if err := r.recordLastError(ctx, taskObjKey, result.Error); err != nil {
			wrappedErr := errors.Wrapf(result.Error, "task admission failed and failed to record error: %v", err)
			return ctrlutils.ReconcileWithError(wrappedErr)
		}
		if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationTypeAdmit, druidv1alpha1.OperationStateFailed, result.Description); err != nil {
			wrappedErr := errors.Wrapf(result.Error, "task admission failed and failed to record operation state: %v", err)
			return ctrlutils.ReconcileWithError(wrappedErr)
		}
		if err := r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStateRejected); err != nil {
			wrappedErr := errors.Wrapf(result.Error, "task admission failed and failed to record task state: %v", err)
			return ctrlutils.ReconcileWithError(wrappedErr)
		}
		return ctrlutils.ReconcileAfter(1*time.Second, "Task rejected, handing to deletion flow")
	}
	return ctrlutils.ContinueReconcile()
}

// transitionToInProgressState sets the task.status.state to InProgress if not already set.
func (r *Reconciler) transitionToInProgressState(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, _ handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "transitionToInProgressState")
	logger.Info("Executing step: transitionToInProgressState")

	if task, err := r.getTask(ctx, taskObjKey); err != nil {
		logger.Error(err, "Failed to get task")
		return ctrlutils.ReconcileWithError(err)
	} else if task.Status.State != nil && *task.Status.State == druidv1alpha1.TaskStatePending {
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
func (r *Reconciler) runTask(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "runTask")
	logger.Info("Executing step: runTask")

	if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationTypeRunning, druidv1alpha1.OperationStateInProgress, ""); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	result := taskHandler.Run(ctx)

	if result.Completed {
		if result.Error != nil {
			// Task failed
			if err := r.recordLastError(ctx, taskObjKey, result.Error); err != nil {
				wrappedErr := errors.Wrapf(result.Error, "task execution failed and failed to record error: %v", err)
				return ctrlutils.ReconcileWithError(wrappedErr)
			}
			if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationTypeRunning, druidv1alpha1.OperationStateFailed, result.Description); err != nil {
				wrappedErr := errors.Wrapf(result.Error, "task execution failed and failed to record operation state: %v", err)
				return ctrlutils.ReconcileWithError(wrappedErr)
			}
			if err := r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStateFailed); err != nil {
				wrappedErr := errors.Wrapf(result.Error, "task execution failed and failed to record task state: %v", err)
				return ctrlutils.ReconcileWithError(wrappedErr)
			}
		} else {
			// Task succeeded
			if err := r.recordLastOperation(ctx, taskObjKey, druidv1alpha1.OperationTypeRunning, druidv1alpha1.OperationStateCompleted, result.Description); err != nil {
				return ctrlutils.ReconcileWithError(err)
			}
			if err := r.recordTaskState(ctx, taskObjKey, druidv1alpha1.TaskStateSucceeded); err != nil {
				logger.Error(err, "Failed to record task state as succeeded")
				return ctrlutils.ReconcileWithError(err)
			}
		}
		return ctrlutils.ReconcileAfter(1*time.Second, "Task completed")
	}

	if result.Error != nil {
		if err := r.recordLastError(ctx, taskObjKey, result.Error); err != nil {
			wrappedErr := errors.Wrapf(result.Error, "task execution failed and failed to record error: %v", err)
			return ctrlutils.ReconcileWithError(wrappedErr)
		}
	}
	return ctrlutils.ReconcileAfter(r.config.RequeueInterval.Duration, "Task in progress")
}
