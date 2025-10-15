// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"

	druidapiconstants "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
		if updateErr := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateRequeue,
				Description: fmt.Sprintf("ensureTaskFinalizer failed to get task: %v", err),
			},
		}); updateErr != nil {
			return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
		}
		return ctrlutils.ReconcileWithError(err)
	}
	if controllerutil.ContainsFinalizer(meta, druidapiconstants.EtcdOpsTaskFinalizerName) {
		return ctrlutils.ContinueReconcile()
	}
	if err := kubernetes.AddFinalizers(ctx, r.client, meta, druidapiconstants.EtcdOpsTaskFinalizerName); err != nil {
		if updateErr := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateRequeue,
				Description: fmt.Sprintf("ensureTaskFinalizer failed to add finalizer: %v", err),
			},
		}); updateErr != nil {
			return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
		}
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
		if updateErr := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateRequeue,
				Description: fmt.Sprintf("transitionToPendingState failed to get task: %v", err),
			},
		}); updateErr != nil {
			return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
		}
		return ctrlutils.ReconcileWithError(err)
	}

	if task.Status.State != nil {
		return ctrlutils.ContinueReconcile()
	}
	// Set the task state to Pending
	// and update the LastTransitionTime.
	pendingState := druidv1alpha1.TaskStatePending
	if err := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
		State: &pendingState,
	}); err != nil {
		if updateErr := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateRequeue,
				Description: fmt.Sprintf("transitionToPendingState failed to update task state: %v", err),
			},
		}); updateErr != nil {
			return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
		}
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

// admitTask checks if the task is in a pending state.
// If so, it updates the LastOperation to admit and invokes the task handler's Admit method.
// If the admit operation is in progress, it requeues the task.
// If the admit operation fails, it updates the task state to Rejected.
// If the admit operation succeeds, it updates the task.status.state to InProgress.
// If the task is already in a completed state, it skips the admit operation.
func (r *Reconciler) admitTask(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "admitTask")
	logger.Info("Executing step: admitTask")

	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		if updateErr := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateRequeue,
				Description: fmt.Sprintf("admitTask failed to get task: %v", err),
			},
		}); updateErr != nil {
			return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
		}
		return ctrlutils.ReconcileWithError(err)
	}
	// If the task is not in Pending state, skip the admit step.
	if task.Status.State != nil && *task.Status.State != druidv1alpha1.TaskStatePending {
		return ctrlutils.ContinueReconcile()
	}

	var etcdOpsTaskList druidv1alpha1.EtcdOpsTaskList
	if err = r.client.List(ctx, &etcdOpsTaskList, client.InNamespace(task.Namespace)); err != nil {
		if updateErr := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateRequeue,
				Description: fmt.Sprintf("admitTask failed to list task: %v", err),
			},
		}); updateErr != nil {
			return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
		}
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
					string(druidv1alpha1.OperationPhaseAdmit),
					"duplicate EtcdOpsTask for the same etcd is already in progress",
				)

				if statusErr := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
					Operation: &druidv1alpha1.LastOperation{
						Type:        druidv1alpha1.LastOperationTypeReconcile,
						State:       druidv1alpha1.LastOperationStateError,
						Description: err.Error(),
					},
					Phase: ptr.To(druidv1alpha1.OperationPhaseAdmit),
					State: ptr.To(druidv1alpha1.TaskStateRejected),
					Error: err,
				}); statusErr != nil {
					return ctrlutils.ReconcileWithError(statusErr)
				}
				return ctrlutils.ReconcileWithError(fmt.Errorf("duplicate task found, handing to deletion flow"))
			}
		}
	}

	if err := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
		Operation: &druidv1alpha1.LastOperation{
			Type:        druidv1alpha1.LastOperationTypeReconcile,
			State:       druidv1alpha1.LastOperationStateProcessing,
			Description: "Task Admit phase is in progress",
		},
		Phase: ptr.To(druidv1alpha1.OperationPhaseAdmit),
	}); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	result := taskHandler.Admit(ctx)
	return r.handleTaskResult(ctx, taskObjKey, result, druidv1alpha1.OperationPhaseAdmit)
}

// transitionToInProgressState sets the task.status.state to InProgress if not already set.
func (r *Reconciler) transitionToInProgressState(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, _ handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "transitionToInProgressState")
	logger.Info("Executing step: transitionToInProgressState")

	if task, err := r.getTask(ctx, taskObjKey); err != nil {
		if updateErr := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateRequeue,
				Description: fmt.Sprintf("transitionToInProgressState failed to get task: %v", err),
			},
		}); updateErr != nil {
			return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
		}
		return ctrlutils.ReconcileWithError(err)

	} else if task.Status.State != nil && *task.Status.State == druidv1alpha1.TaskStatePending {
		inProgressState := druidv1alpha1.TaskStateInProgress
		if err := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
			State: &inProgressState,
		}); err != nil {
			if updateErr := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
				Operation: &druidv1alpha1.LastOperation{
					Type:        druidv1alpha1.LastOperationTypeReconcile,
					State:       druidv1alpha1.LastOperationStateRequeue,
					Description: fmt.Sprintf("transitionToInProgressState failed to update task state: %v", err),
				},
			}); updateErr != nil {
				return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
			}
			return ctrlutils.ReconcileWithError(err)
		}
	} else {
		logger.Info("Task state is not Pending, skipping transition", "currentState", task.Status.State)
	}

	return ctrlutils.ContinueReconcile()
}

// runTask executes the task handler's Run method.
// It updates the task status based on the result of the run operation.
// If the task is still in progress, it requeues the task for further processing.
// If the task fails, it updates the task state to failed and sets the LastOperation to error.
// If the task succeeds, it updates the task state to succeeded and sets the LastOperation to succeeded.
func (r *Reconciler) runTask(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "runTask")
	logger.Info("Executing step: runTask")

	if err := r.updateTaskStatus(ctx, taskObjKey, TaskStatusUpdate{
		Operation: &druidv1alpha1.LastOperation{
			Type:        druidv1alpha1.LastOperationTypeReconcile,
			State:       druidv1alpha1.LastOperationStateProcessing,
			Description: "Task Running phase is in Progress",
		},
		Phase: ptr.To(druidv1alpha1.OperationPhaseRunning),
	}); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	result := taskHandler.Run(ctx)
	return r.handleTaskResult(ctx, taskObjKey, result, druidv1alpha1.OperationPhaseRunning)
}
