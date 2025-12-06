// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// reconcileTask manages the lifecycle of an EtcdOpsTask resource.
// It executes a series of step functions to ensure the task is processed correctly.
func (r *Reconciler) reconcileTask(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("op", "reconcileTask")
	logger.Info("Triggering task execution flow")

	reconcileStepFns := []reconcileFn{
		r.ensureTaskFinalizer,
		r.transitionToPendingState,
		r.admitTask,
		r.transitionToInProgressState,
		r.executeTask,
	}

	for _, fn := range reconcileStepFns {
		if result := fn(ctx, logger, task, taskHandler); ctrlutils.ShortCircuitReconcileFlow(result) {
			return result
		}
	}
	logger.Info("Task execution flow completed")
	return ctrlutils.ContinueReconcile()
}

// ensureTaskFinalizer checks if the EtcdOpsTask has the finalizer.
// If not, it adds the finalizer and updates the task status.
func (r *Reconciler) ensureTaskFinalizer(ctx context.Context, _ logr.Logger, task *druidv1alpha1.EtcdOpsTask, _ handler.Handler) ctrlutils.ReconcileStepResult {
	if controllerutil.ContainsFinalizer(task, druidapicommon.EtcdOpsTaskFinalizerName) {
		return ctrlutils.ContinueReconcile()
	}

	if err := kubernetes.AddFinalizers(ctx, r.client, task, druidapicommon.EtcdOpsTaskFinalizerName); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

// transitionToPendingState sets the task.status.state to Pending if not already set.
func (r *Reconciler) transitionToPendingState(ctx context.Context, _ logr.Logger, task *druidv1alpha1.EtcdOpsTask, _ handler.Handler) ctrlutils.ReconcileStepResult {
	if task.Status.State != nil {
		return ctrlutils.ContinueReconcile()
	}
	// Set the task state to Pending
	// and update the LastTransitionTime.
	pendingState := druidv1alpha1.TaskStatePending
	if err := r.updateTaskStatus(ctx, task, taskStatusUpdate{
		State: &pendingState,
	}); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

// admitTask checks if the task is in a pending state.
// If so, it updates the phase to admit and invokes the task handler's Admit method.
// If the admit operation is in progress, it requeues the task.
// If the admit operation fails, it updates the task state to Rejected.
// If the admit operation succeeds, it updates the task.status.state to InProgress.
// If the task is not in its Pending state, it skips the step.
func (r *Reconciler) admitTask(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "admitTask")

	// If the task is not in Pending state, skip the admit step.
	if task.Status.State != nil && *task.Status.State != druidv1alpha1.TaskStatePending {
		return ctrlutils.ContinueReconcile()
	}

	if err := r.updateTaskStatus(ctx, task, taskStatusUpdate{
		Operation: &druidapicommon.LastOperation{
			Type:        druidv1alpha1.LastOperationTypeAdmit,
			State:       druidv1alpha1.LastOperationStateInProgress,
			Description: "Task Admit phase is in progress",
		},
	}); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	var etcdOpsTaskList druidv1alpha1.EtcdOpsTaskList
	if err := r.client.List(ctx, &etcdOpsTaskList, client.InNamespace(task.Namespace)); err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			wrappedErr := druiderr.WrapError(err, handler.ErrInsufficientPermissions, string(druidv1alpha1.LastOperationTypeAdmit), "insufficient permissions to list EtcdOpsTasks")
			return r.rejectTaskWithError(ctx, task, "Insufficient permissions to list EtcdOpsTasks", wrappedErr)
		}
		return ctrlutils.ReconcileWithError(fmt.Errorf("failed to list EtcdOpsTasks: %w", err))
	}

	for _, existingTask := range etcdOpsTaskList.Items {
		if existingTask.Spec.EtcdName != nil && task.Spec.EtcdName != nil && *existingTask.Spec.EtcdName == *task.Spec.EtcdName {
			if !existingTask.IsCompleted() && existingTask.Name != task.Name {
				wrappedErr := druiderr.WrapError(fmt.Errorf("an EtcdOpsTask for etcd %s/%s is already in progress (task: %s)", task.Namespace, *task.Spec.EtcdName, existingTask.Name), handler.ErrDuplicateTask, string(druidv1alpha1.LastOperationTypeAdmit), "duplicate task found")
				return r.rejectTaskWithError(ctx, task, "EtcdOpsTask for the same etcd is already in progress", wrappedErr)
			}
		}
	}

	result := taskHandler.Admit(ctx)
	return r.handleTaskResult(ctx, logger, task, result, druidv1alpha1.LastOperationTypeAdmit)
}

// transitionToInProgressState sets the task.status.state to InProgress if not already set.
func (r *Reconciler) transitionToInProgressState(ctx context.Context, _ logr.Logger, task *druidv1alpha1.EtcdOpsTask, _ handler.Handler) ctrlutils.ReconcileStepResult {
	if task.Status.State == nil || *task.Status.State != druidv1alpha1.TaskStatePending {
		return ctrlutils.ContinueReconcile()
	}

	if err := r.updateTaskStatus(ctx, task, taskStatusUpdate{
		State: ptr.To(druidv1alpha1.TaskStateInProgress),
	}); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	return ctrlutils.ContinueReconcile()
}

// executeTask executes the task handler's Run method.
// It updates the task status based on the result of the run operation.
// If the task is still in progress, it requeues the task for further processing.
// If the task fails, it updates the task state to failed and sets the LastOperation to error.
// If the task succeeds, it updates the task state to succeeded and sets the LastOperation to succeeded.
func (r *Reconciler) executeTask(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "runTask")

	if err := r.updateTaskStatus(ctx, task, taskStatusUpdate{
		Operation: &druidapicommon.LastOperation{
			Type:        druidv1alpha1.LastOperationTypeExecution,
			State:       druidv1alpha1.LastOperationStateInProgress,
			Description: "Task Execution phase is in Progress",
		},
	}); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	result := taskHandler.Execute(ctx)
	return r.handleTaskResult(ctx, logger, task, result, druidv1alpha1.LastOperationTypeExecution)
}
