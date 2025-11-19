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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// triggerDeletionFlow handles the deletion flow for EtcdOpsTask resources.
func (r *Reconciler) triggerDeletionFlow(ctx context.Context, logger logr.Logger, taskHandler handler.Handler, task *druidv1alpha1.EtcdOpsTask) ctrlutils.ReconcileStepResult {
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

	if task.Status.State != nil && *task.Status.State == druidv1alpha1.TaskStateRejected {
		logger.Info("Task is rejected, skipping cleanup")
		// no-op cleanup
		// Cases where task is rejected:
		// 1. Task is not supported by the controller
		// 2. Task admit failed
		// In these cases, we don't want to call the cleanup method of the task handler. Since there is no cleanup to be done, we can skip this step.
		if err := r.updateTaskStatus(ctx, task, taskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeDelete,
				State:       druidv1alpha1.LastOperationStateProcessing,
				Description: "Cleanup skipped for rejected task",
			},
			Phase: ptr.To(druidv1alpha1.OperationPhaseCleanup),
		}); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		return ctrlutils.ContinueReconcile()
	}

	if err := r.updateTaskStatus(ctx, task, taskStatusUpdate{
		Operation: &druidv1alpha1.LastOperation{
			Type:        druidv1alpha1.LastOperationTypeDelete,
			State:       druidv1alpha1.LastOperationStateProcessing,
			Description: "Task Cleanup phase is in Progress",
		},
		Phase: ptr.To(druidv1alpha1.OperationPhaseCleanup),
	}); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	result := taskHandler.Cleanup(ctx)
	return r.handleTaskResult(ctx, logger, task, result, druidv1alpha1.OperationPhaseCleanup)
}

// removeTaskFinalizer removes the finalizer from the EtcdOpsTask resource.
func (r *Reconciler) removeTaskFinalizer(ctx context.Context, logger logr.Logger, taskObjKey client.ObjectKey, _ handler.Handler) ctrlutils.ReconcileStepResult {
	logger = logger.WithValues("step", "removeTaskFinalizer")
	logger.Info("Executing step: removeTaskFinalizer")

	task, err := r.getTask(ctx, taskObjKey)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if controllerutil.ContainsFinalizer(task, druidapicommon.EtcdOpsTaskFinalizerName) {
		patch := client.MergeFrom(task.DeepCopy())
		controllerutil.RemoveFinalizer(task, druidapicommon.EtcdOpsTaskFinalizerName)
		if err := r.client.Patch(ctx, task, patch); err != nil {
			if updateErr := r.updateTaskStatus(ctx, task, taskStatusUpdate{
				Operation: &druidv1alpha1.LastOperation{
					Type:        druidv1alpha1.LastOperationTypeDelete,
					State:       druidv1alpha1.LastOperationStateRequeue,
					Description: fmt.Sprintf("removeTaskFinalizer failed to remove finalizer: %v", err),
				},
			}); updateErr != nil {
				return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
			}
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
		if updateErr := r.updateTaskStatus(ctx, task, taskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeDelete,
				State:       druidv1alpha1.LastOperationStateError,
				Description: fmt.Sprintf("removeTask failed to delete task: %v", err),
			},
		}); updateErr != nil {
			return ctrlutils.ReconcileWithError(errors.Wrapf(err, "failed to update status after error: %v", updateErr))
		}
		return ctrlutils.ReconcileWithError(client.IgnoreNotFound(err))
	}
	return ctrlutils.DoNotRequeue()
}
