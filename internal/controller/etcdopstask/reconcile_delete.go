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

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// triggerDeletionFlow handles the deletion flow for EtcdOpsTask resources.
func (r *Reconciler) triggerDeletionFlow(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("op", "triggerDeletionFlow")
	logger.Info("Triggering deletion flow", "completed", task.IsCompleted(), "markedForDeletion", druidv1alpha1.IsResourceMarkedForDeletion(task.ObjectMeta))

	if task.IsCompleted() && !task.HasTTLExpired() {
		timeToExpiry := task.GetTimeToExpiry()
		logger.Info("Task completed but TTL not expired yet, will requeue after TTL", "ttlSeconds", timeToExpiry.Seconds())
		return ctrlutils.ReconcileAfter(timeToExpiry, "Task completed, waiting for TTL to expire")
	}

	logger.Info("Task TTL expired, proceeding with deletion")

	deletionStepFns := []reconcileFn{
		r.cleanupTaskResources,
		r.removeTaskFinalizer,
		r.removeTask,
	}

	for _, fn := range deletionStepFns {
		if result := fn(ctx, logger, task, taskHandler); ctrlutils.ShortCircuitReconcileFlow(result) {
			return result
		}
	}
	logger.Info("Deletion flow completed for EtcdOpsTask")
	return ctrlutils.DoNotRequeue()
}

// cleanupTaskResources calls the Cleanup method of the task handler.
func (r *Reconciler) cleanupTaskResources(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "cleanupTaskResources")

	if err := r.updateTaskStatus(ctx, task, taskStatusUpdate{
		Operation: &druidapicommon.LastOperation{
			Type:        druidv1alpha1.LastOperationTypeCleanup,
			State:       druidv1alpha1.LastOperationStateInProgress,
			Description: "Task Cleanup phase is in Progress",
		},
	}); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	if task.Status.State != nil && *task.Status.State == druidv1alpha1.TaskStateRejected {
		logger.Info("Task is rejected, skipping cleanup")
		// no-op cleanup
		// Cases where task is rejected:
		// 1. Task is not supported by the controller
		// 2. Task admit failed
		// In these cases, we don't want to call the cleanup method of the task handler. Since there is no cleanup to be done, we can skip this step.
		if err := r.updateTaskStatus(ctx, task, taskStatusUpdate{
			Operation: &druidapicommon.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeCleanup,
				State:       druidv1alpha1.LastOperationStateCompleted,
				Description: "Cleanup skipped for rejected task",
			},
		}); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		return ctrlutils.ContinueReconcile()
	}

	result := taskHandler.Cleanup(ctx)
	return r.handleTaskResult(ctx, logger, task, result, druidv1alpha1.LastOperationTypeCleanup)
}

// removeTaskFinalizer removes the finalizer from the EtcdOpsTask resource.
func (r *Reconciler) removeTaskFinalizer(ctx context.Context, _ logr.Logger, task *druidv1alpha1.EtcdOpsTask, _ handler.Handler) ctrlutils.ReconcileStepResult {
	if controllerutil.ContainsFinalizer(task, druidapicommon.EtcdOpsTaskFinalizerName) {
		patch := client.MergeFrom(task.DeepCopy())
		controllerutil.RemoveFinalizer(task, druidapicommon.EtcdOpsTaskFinalizerName)
		if err := r.client.Patch(ctx, task, patch); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}

// removeTask deletes the EtcdOpsTask resource from the cluster.
func (r *Reconciler) removeTask(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, _ handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "removeTask")

	if err := r.client.Delete(ctx, task); err != nil {
		if !apierrors.IsNotFound(err) {
			updateErr := r.updateTaskStatus(ctx, task, taskStatusUpdate{
				Operation: &druidapicommon.LastOperation{
					Type:        druidv1alpha1.LastOperationTypeCleanup,
					State:       druidv1alpha1.LastOperationStateInProgress,
					Description: "Task Cleanup phase is in progress",
				},
				Error: druiderr.WrapError(err, handler.ErrDeleteEtcdOpsTask, string(druidv1alpha1.LastOperationTypeCleanup), "failed to delete EtcdOpsTask resource"),
			})
			if updateErr != nil {
				return ctrlutils.ReconcileWithError(fmt.Errorf("%w: failed to delete task and update last opertion: %w", err, updateErr))
			}
			return ctrlutils.ReconcileWithError(err)
		}
	}

	logger.Info("EtcdOpsTask resource deleted successfully")
	return ctrlutils.DoNotRequeue()
}
