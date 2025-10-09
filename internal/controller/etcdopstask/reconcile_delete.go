// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"

	"github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// triggerDeletionFlow handles the deletion flow for EtcdOpsTask resources.
func (r *Reconciler) triggerDeletionFlow(ctx context.Context, logger logr.Logger, taskHandler handler.Handler, task *v1alpha1.EtcdOpsTask) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("op", "triggerDeletionFlow")
	logger.Info("Triggering deletion flow", "completed", task.IsCompleted(), "markedForDeletion", task.IsMarkedForDeletion())

	// If task is completed but not marked for deletion, wait for TTL expiry
	if task.IsCompleted() {
		if !task.HasTTLExpired() {
			logger.Info("Task completed but TTL not expired yet, will requeue after TTL", "ttlSeconds", task.Spec.TTLSecondsAfterFinished)
			return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), "Task completed, waiting for TTL to expire")
		}
		logger.Info("Task TTL expired, proceeding with deletion")
	}

	deletionStepFns := []reconcileFn{
		r.cleanupTaskResources,
		r.removeTaskFinalizer,
		r.removeTask,
	}

	for _, fn := range deletionStepFns {
		if result := fn(ctx, logger, client.ObjectKeyFromObject(task), taskHandler); ctrlutils.ShortCircuitReconcileFlow(result) {
			return result
		}
	}
	logger.Info("Deletion flow completed for EtcdOpsTask")
	return ctrlutils.DoNotRequeue()
}

// cleanupTaskResources calls the Cleanup method of the task handler.
func (r *Reconciler) cleanupTaskResources(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, taskHandler handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "cleanupTaskResources")
	logger.Info("Executing step: cleanupTaskResources")

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
		if err := r.recordLastOperation(ctx, taskObjKey, v1alpha1.OperationTypeCleanup, v1alpha1.OperationStateCompleted, "Cleanup skipped for rejected task"); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		return ctrlutils.ContinueReconcile()
	}

	if err := r.recordLastOperation(ctx, taskObjKey, v1alpha1.OperationTypeCleanup, v1alpha1.OperationStateInProgress, ""); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	result := taskHandler.Cleanup(ctx)

	if result.Completed {
		if result.Error != nil {
			logger.Error(result.Error, "Cleanup operation failed", "description", result.Description)

			if err := r.recordLastError(ctx, taskObjKey, result.Error); err != nil {
				wrappedErr := errors.Wrapf(result.Error, "cleanup failed and failed to record error: %v", err)
				return ctrlutils.ReconcileWithError(wrappedErr)
			}
			if err := r.recordLastOperation(ctx, taskObjKey, v1alpha1.OperationTypeCleanup, v1alpha1.OperationStateFailed, result.Description); err != nil {
				wrappedErr := errors.Wrapf(result.Error, "cleanup failed and failed to record operation state: %v", err)
				return ctrlutils.ReconcileWithError(wrappedErr)
			}
			return ctrlutils.ReconcileWithError(result.Error)
		}
		if err := r.recordLastOperation(ctx, taskObjKey, v1alpha1.OperationTypeCleanup, v1alpha1.OperationStateCompleted, result.Description); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	} else if result.Error != nil {
		if err := r.recordLastError(ctx, taskObjKey, result.Error); err != nil {
			wrappedErr := errors.Wrapf(result.Error, "cleanup failed and failed to record error: %v", err)
			return ctrlutils.ReconcileWithError(wrappedErr)
		}
		return ctrlutils.ReconcileWithError(result.Error)
	}
	return ctrlutils.ContinueReconcile()
}

// removeTaskFinalizer removes the finalizer from the EtcdOpsTask resource.
func (r *Reconciler) removeTaskFinalizer(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, _ handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "removeTaskFinalizer")

	logger.Info("Executing step: removeTaskFinalizer")
	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if controllerutil.ContainsFinalizer(task, FinalizerName) {
		patch := client.MergeFrom(task.DeepCopy())
		controllerutil.RemoveFinalizer(task, FinalizerName)
		if err := r.client.Patch(ctx, task, patch); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}

// removeTask deletes the EtcdOpsTask resource from the cluster.
func (r *Reconciler) removeTask(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, _ handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "removeTask")
	logger.Info("Executing step: removeTask")

	if task, err := r.getTask(ctx, taskObjKey); err != nil {
		return ctrlutils.ReconcileWithError(err)
	} else if err := r.client.Delete(ctx, task); err != nil {
		return ctrlutils.ReconcileWithError(client.IgnoreNotFound(err))
	}
	return ctrlutils.DoNotRequeue()
}
