// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// getTask fetches the EtcdOpsTask resource for the given object key.
// It returns the task or an error if the resource is not found or the
// retrieval fails. Callers should use client.IgnoreNotFound where
// appropriate to handle deleted resources gracefully.
func (r *Reconciler) getTask(ctx context.Context, taskObjKey client.ObjectKey) (*druidv1alpha1.EtcdOpsTask, error) {
	task := &druidv1alpha1.EtcdOpsTask{}
	err := r.client.Get(ctx, taskObjKey, task)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// rejectTaskWithError updates the task status to Rejected state with the provided error.
func (r *Reconciler) rejectTaskWithError(ctx context.Context, task *druidv1alpha1.EtcdOpsTask, description string, wrappedErr error) ctrlutils.ReconcileStepResult {
	if updateErr := r.updateTaskStatus(ctx, task, taskStatusUpdate{
		Operation: &druidapicommon.LastOperation{
			Type:        druidv1alpha1.LastOperationTypeAdmit,
			State:       druidv1alpha1.LastOperationStateFailed,
			Description: description,
		},
		State: ptr.To(druidv1alpha1.TaskStateRejected),
		Error: wrappedErr,
	}); updateErr != nil {
		return ctrlutils.ReconcileWithError(fmt.Errorf("failed to update status: %v: %w", updateErr, wrappedErr))
	}

	return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), "Task rejected, waiting for TTL to expire before deletion")
}

// taskStatusUpdate represents a status update for a task
type taskStatusUpdate struct {
	// Operation details to update (nil to skip operation update)
	Operation *druidapicommon.LastOperation
	// Task state to set (nil to skip state update)
	State *druidv1alpha1.TaskState
	// Error to record (nil to skip error recording)
	Error error
}

// setLastOperation updates the LastOperation field in the task status
func setLastOperation(task *druidv1alpha1.EtcdOpsTask, opType druidapicommon.LastOperationType, state druidapicommon.LastOperationState, description string, runID string) {
	now := metav1.Time{Time: time.Now().UTC()}
	if description == "" {
		description = fmt.Sprintf("%s is in state %s for task %s", opType, state, task.Name)
	}

	if task.Status.LastOperation == nil {
		task.Status.LastOperation = &druidapicommon.LastOperation{}
	}

	task.Status.LastOperation.Type = opType
	task.Status.LastOperation.State = state
	task.Status.LastOperation.LastUpdateTime = now
	task.Status.LastOperation.Description = description
	task.Status.LastOperation.RunID = runID
}

// setTaskState updates the task's State field
func setTaskState(task *druidv1alpha1.EtcdOpsTask, newState druidv1alpha1.TaskState) {
	if task.Status.State != nil && *task.Status.State == newState {
		return
	}

	now := &metav1.Time{Time: time.Now().UTC()}
	if newState == druidv1alpha1.TaskStateInProgress && task.Status.StartedAt == nil {
		task.Status.StartedAt = now
	}
	task.Status.State = &newState
	task.Status.LastTransitionTime = now
}

// setLastError adds an error to the LastErrors field
func setLastError(task *druidv1alpha1.EtcdOpsTask, err error) {
	now := metav1.Time{Time: time.Now().UTC()}

	newErrors := druiderr.MapToLastErrors([]error{err})
	if len(newErrors) == 0 {
		newErrors = []druidapicommon.LastError{{
			Description: err.Error(),
			ObservedAt:  now,
		}}
	} else {
		newErrors[0].ObservedAt = now
	}

	lastErrors := task.Status.LastErrors
	if lastErrors == nil {
		lastErrors = make([]druidapicommon.LastError, 0, 3)
	}
	if len(lastErrors) >= 3 {
		lastErrors = lastErrors[1:]
	}

	task.Status.LastErrors = append(lastErrors, newErrors[0])
}

// updateTaskStatus updates operation, state, and errors in a single call.
func (r *Reconciler) updateTaskStatus(ctx context.Context, task *druidv1alpha1.EtcdOpsTask, update taskStatusUpdate) error {
	runID := string(controller.ReconcileIDFromContext(ctx))
	originalStatus := task.Status.DeepCopy()

	if update.Operation != nil {
		setLastOperation(task, update.Operation.Type, update.Operation.State, update.Operation.Description, runID)
	}

	if update.State != nil {
		setTaskState(task, *update.State)
	}

	if update.Error != nil {
		setLastError(task, update.Error)
	}

	if !reflect.DeepEqual(originalStatus, &task.Status) {
		if err := r.client.Status().Update(ctx, task); err != nil {
			return err
		}
	}

	return nil

}

// handleTaskResult is a common helper to handle task handler results with status updates
func (r *Reconciler) handleTaskResult(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, result handler.Result, phase druidapicommon.LastOperationType) ctrlutils.ReconcileStepResult {
	if result.Requeue {
		return r.handleRequeue(ctx, logger, task, result, phase)
	}

	if result.Error != nil {
		return r.handleError(ctx, logger, task, result, phase)
	}

	return r.handleSuccess(ctx, logger, task, result, phase)
}

// handleRequeue handles requeue scenarios (common for admit, run, and cleanup)
func (r *Reconciler) handleRequeue(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, result handler.Result, phase druidapicommon.LastOperationType) ctrlutils.ReconcileStepResult {
	statusUpdate := taskStatusUpdate{
		Operation: &druidapicommon.LastOperation{
			Type:        phase,
			State:       druidv1alpha1.LastOperationStateInProgress,
			Description: result.Description,
		},
		Error: result.Error,
	}

	if err := r.updateTaskStatus(ctx, task, statusUpdate); err != nil {
		if result.Error != nil {
			return ctrlutils.ReconcileWithError(fmt.Errorf("failed to record error: %v: %w", err, result.Error))
		}
		return ctrlutils.ReconcileWithError(err)
	}

	if result.Error != nil {
		return ctrlutils.ReconcileWithError(result.Error)
	}

	message := fmt.Sprintf("Task %s in progress", strings.ToLower(string(phase)))
	logger.Info("Requeuing task", "phase", phase, "requeueInterval", r.config.RequeueInterval.Duration)
	return ctrlutils.ReconcileAfter(r.config.RequeueInterval.Duration, message)
}

// handleError handles error scenarios with phase-specific behavior
func (r *Reconciler) handleError(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, result handler.Result, phase druidapicommon.LastOperationType) ctrlutils.ReconcileStepResult {
	var taskState *druidv1alpha1.TaskState
	var errorMessage string

	switch phase {
	case druidv1alpha1.LastOperationTypeAdmit:
		taskState = ptr.To(druidv1alpha1.TaskStateRejected)
		errorMessage = "task rejected, handing to deletion flow"
	case druidv1alpha1.LastOperationTypeExecution:
		taskState = ptr.To(druidv1alpha1.TaskStateFailed)
		errorMessage = "task completed with failure"
	case druidv1alpha1.LastOperationTypeCleanup:
		errorMessage = "cleanup failed"
	}

	statusUpdate := taskStatusUpdate{
		Operation: &druidapicommon.LastOperation{
			Type:        phase,
			State:       druidv1alpha1.LastOperationStateFailed,
			Description: result.Description,
		},
		State: taskState,
		Error: result.Error,
	}

	if err := r.updateTaskStatus(ctx, task, statusUpdate); err != nil {
		return ctrlutils.ReconcileWithError(fmt.Errorf("failed to record error: %v: %w", err, result.Error))
	}

	if phase == druidv1alpha1.LastOperationTypeAdmit || phase == druidv1alpha1.LastOperationTypeExecution {
		logger.Info("Task cleanup after TTL", "state", *task.Status.State, "ttlSeconds", task.Spec.TTLSecondsAfterFinished)
		return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), fmt.Sprintf("Task %s, waiting for TTL to expire", strings.ToLower(string(*task.Status.State))))
	}

	return ctrlutils.ReconcileWithError(fmt.Errorf("%s", errorMessage))
}

// handleSuccess handles success scenarios with phase-specific behavior
func (r *Reconciler) handleSuccess(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, result handler.Result, phase druidapicommon.LastOperationType) ctrlutils.ReconcileStepResult {
	operationState := druidv1alpha1.LastOperationStateCompleted
	if phase == druidv1alpha1.LastOperationTypeCleanup {
		operationState = druidv1alpha1.LastOperationStateInProgress
	}

	statusUpdate := taskStatusUpdate{
		Operation: &druidapicommon.LastOperation{
			Type:        phase,
			State:       operationState,
			Description: result.Description,
		},
	}

	if phase == druidv1alpha1.LastOperationTypeExecution {
		statusUpdate.State = ptr.To(druidv1alpha1.TaskStateSucceeded)
	}

	if err := r.updateTaskStatus(ctx, task, statusUpdate); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	if phase == druidv1alpha1.LastOperationTypeExecution {
		logger.Info("Task completed successfully and will requeue after TTL", "state", task.Status.State, "ttlSeconds", task.Spec.TTLSecondsAfterFinished)
		return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), "Task succeeded, waiting for TTL to expire")
	}

	return ctrlutils.ContinueReconcile()
}
