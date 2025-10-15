// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
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

// TaskStatusUpdate represents a status update for a task
type TaskStatusUpdate struct {
	// Operation details to update (nil to skip operation update)
	Operation *druidv1alpha1.LastOperation
	// Task state to set (nil to skip state update)
	State *druidv1alpha1.TaskState
	// Phase to set (nil to skip phase update)
	Phase *druidv1alpha1.OperationPhase
	// Error to record (nil to skip error recording)
	Error error
}

// setLastOperation updates the LastOperation field in the task status
func setLastOperation(task *druidv1alpha1.EtcdOpsTask, opType druidv1alpha1.LastOperationType, state druidv1alpha1.LastOperationState, description string, runID string) {
	now := metav1.Time{Time: time.Now().UTC()}
	if description == "" {
		description = fmt.Sprintf("%s is in state %s for task %s", opType, state, task.Name)
	}

	if task.Status.LastOperation == nil {
		task.Status.LastOperation = &druidv1alpha1.LastOperation{
			Type:           opType,
			State:          state,
			LastUpdateTime: now,
			Description:    description,
			RunID:          runID,
		}
		return
	}

	task.Status.LastOperation.Type = opType
	task.Status.LastOperation.State = state
	task.Status.LastOperation.LastUpdateTime = now
	task.Status.LastOperation.Description = description
	task.Status.LastOperation.RunID = runID
}

// setTaskState updates the task's State field
func setTaskState(task *druidv1alpha1.EtcdOpsTask, state druidv1alpha1.TaskState) bool {
	stateChanged := task.Status.State == nil || *task.Status.State != state
	if !stateChanged {
		return false
	}

	now := &metav1.Time{Time: time.Now().UTC()}
	if state == druidv1alpha1.TaskStateInProgress && task.Status.StartedAt == nil {
		task.Status.StartedAt = now
	}
	task.Status.State = &state
	task.Status.LastTransitionTime = now
	return true
}

// setPhase updates the task's Phase field
func setPhase(task *druidv1alpha1.EtcdOpsTask, phase druidv1alpha1.OperationPhase) bool {
	phaseChanged := task.Status.Phase == nil || *task.Status.Phase != phase
	if !phaseChanged {
		return false
	}

	task.Status.Phase = &phase
	return true
}

// setLastError adds an error to the LastErrors field
func setLastError(task *druidv1alpha1.EtcdOpsTask, err error) {
	now := metav1.Time{Time: time.Now().UTC()}

	newErrors := druiderr.MapToLastErrors([]error{err})
	if len(newErrors) == 0 {
		newErrors = []druidv1alpha1.LastError{{
			Description: err.Error(),
			ObservedAt:  now,
		}}
	} else {
		newErrors[0].ObservedAt = now
	}

	lastErrors := task.Status.LastErrors
	if lastErrors == nil {
		lastErrors = make([]druidv1alpha1.LastError, 0, 3)
	}
	if len(lastErrors) >= 3 {
		lastErrors = lastErrors[1:]
	}

	task.Status.LastErrors = append(lastErrors, newErrors[0])
}

// updateTaskStatus updates operation, state, and errors in a single call.
func (r *Reconciler) updateTaskStatus(ctx context.Context, taskObjKey client.ObjectKey, update TaskStatusUpdate) error {
	runID := string(controller.ReconcileIDFromContext(ctx))

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		task, err := r.getTask(ctx, taskObjKey)
		if err != nil {
			return err
		}

		hasChanges := false

		if update.Operation != nil {
			setLastOperation(task, update.Operation.Type, update.Operation.State, update.Operation.Description, runID)
			hasChanges = true
		}

		if update.State != nil {
			if setTaskState(task, *update.State) {
				hasChanges = true
			}
		}

		if update.Phase != nil {
			if setPhase(task, *update.Phase) {
				hasChanges = true
			}
		}

		if update.Error != nil {
			setLastError(task, update.Error)
			hasChanges = true
		}

		if hasChanges {
			return r.client.Status().Update(ctx, task)
		}

		return nil
	})
}

// handleTaskResult is a common helper to handle task handler results with status updates
func (r *Reconciler) handleTaskResult(ctx context.Context, taskObjKey client.ObjectKey, result handler.Result, phase druidv1alpha1.OperationPhase) ctrlutils.ReconcileStepResult {
	if result.Requeue {
		return r.handleRequeue(ctx, taskObjKey, result, phase)
	}

	if result.Error != nil {
		return r.handleError(ctx, taskObjKey, result, phase)
	}

	return r.handleSuccess(ctx, taskObjKey, result, phase)
}

// handleRequeue handles requeue scenarios (common for admit, run, and cleanup)
func (r *Reconciler) handleRequeue(ctx context.Context, taskObjKey client.ObjectKey, result handler.Result, phase druidv1alpha1.OperationPhase) ctrlutils.ReconcileStepResult {
	var opType druidv1alpha1.LastOperationType

	if phase == druidv1alpha1.OperationPhaseCleanup {
		opType = druidv1alpha1.LastOperationTypeDelete
	} else {
		opType = druidv1alpha1.LastOperationTypeReconcile
	}

	statusUpdate := TaskStatusUpdate{
		Operation: &druidv1alpha1.LastOperation{
			Type:        opType,
			State:       druidv1alpha1.LastOperationStateRequeue,
			Description: result.Description,
		},
		Error: result.Error,
	}

	if err := r.updateTaskStatus(ctx, taskObjKey, statusUpdate); err != nil {
		if result.Error != nil {
			return ctrlutils.ReconcileWithError(errors.Wrapf(result.Error, "failed to record error: %v", err))
		}
		return ctrlutils.ReconcileWithError(err)
	}

	if result.Error != nil {
		return ctrlutils.ReconcileWithError(result.Error)
	}

	message := fmt.Sprintf("Task %s in progress", strings.ToLower(string(phase)))
	return ctrlutils.ReconcileAfter(r.config.RequeueInterval.Duration, message)
}

// handleError handles error scenarios with phase-specific behavior
func (r *Reconciler) handleError(ctx context.Context, taskObjKey client.ObjectKey, result handler.Result, phase druidv1alpha1.OperationPhase) ctrlutils.ReconcileStepResult {
	var taskState *druidv1alpha1.TaskState
	var errorMessage string
	var opType druidv1alpha1.LastOperationType

	switch phase {
	case druidv1alpha1.OperationPhaseAdmit:
		taskState = ptr.To(druidv1alpha1.TaskStateRejected)
		errorMessage = "task rejected, handing to deletion flow"
		opType = druidv1alpha1.LastOperationTypeReconcile
	case druidv1alpha1.OperationPhaseRunning:
		taskState = ptr.To(druidv1alpha1.TaskStateFailed)
		errorMessage = "task completed"
		opType = druidv1alpha1.LastOperationTypeReconcile
	case druidv1alpha1.OperationPhaseCleanup:
		errorMessage = "cleanup failed"
		opType = druidv1alpha1.LastOperationTypeDelete
	}

	statusUpdate := TaskStatusUpdate{
		Operation: &druidv1alpha1.LastOperation{
			Type:        opType,
			State:       druidv1alpha1.LastOperationStateError,
			Description: result.Description,
		},
		State: taskState,
		Error: result.Error,
	}

	if err := r.updateTaskStatus(ctx, taskObjKey, statusUpdate); err != nil {
		return ctrlutils.ReconcileWithError(errors.Wrapf(result.Error, "failed to record error: %v", err))
	}

	// For admit and run failed, get task and requeue after TTL
	if phase == druidv1alpha1.OperationPhaseAdmit || phase == druidv1alpha1.OperationPhaseRunning {
		task, err := r.getTask(ctx, taskObjKey)
		if err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), fmt.Sprintf("Task %s, waiting for TTL to expire", strings.ToLower(string(*task.Status.State))))
	}

	return ctrlutils.ReconcileWithError(fmt.Errorf("%s", errorMessage))
}

// handleSuccess handles success scenarios with phase-specific behavior
func (r *Reconciler) handleSuccess(ctx context.Context, taskObjKey client.ObjectKey, result handler.Result, phase druidv1alpha1.OperationPhase) ctrlutils.ReconcileStepResult {
	var statusUpdate TaskStatusUpdate

	switch phase {
	case druidv1alpha1.OperationPhaseAdmit:
		statusUpdate = TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateProcessing,
				Description: result.Description,
			},
		}
	case druidv1alpha1.OperationPhaseRunning:
		statusUpdate = TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateSucceeded,
				Description: result.Description,
			},
			State: ptr.To(druidv1alpha1.TaskStateSucceeded),
		}
	case druidv1alpha1.OperationPhaseCleanup:
		statusUpdate = TaskStatusUpdate{
			Operation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeDelete,
				State:       druidv1alpha1.LastOperationStateProcessing,
				Description: result.Description,
			},
		}
	}

	if err := r.updateTaskStatus(ctx, taskObjKey, statusUpdate); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}

	if phase == druidv1alpha1.OperationPhaseRunning {
		task, err := r.getTask(ctx, taskObjKey)
		if err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		return ctrlutils.ReconcileAfter(task.GetTimeToExpiry(), "Task succeeded, waiting for TTL to expire")
	}

	return ctrlutils.ContinueReconcile()
}
