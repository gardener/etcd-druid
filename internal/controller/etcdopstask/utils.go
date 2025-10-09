// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"
	"time"

	"github.com/gardener/etcd-druid/api/core/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// getTask fetches the EtcdOpsTask resource for the given object key.
// It returns the task or an error if the resource is not found or the
// retrieval fails. Callers should use client.IgnoreNotFound where
// appropriate to handle deleted resources gracefully.
func (r *Reconciler) getTask(ctx context.Context, taskObjKey client.ObjectKey) (*v1alpha1.EtcdOpsTask, error) {
	task := &v1alpha1.EtcdOpsTask{}
	err := r.client.Get(ctx, taskObjKey, task)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// recordLastOperation sets or updates the LastOperation field in the task status.
//
// If LastOperation is nil, it initializes it with the given phase and state.
// If the phase or state changes, it updates them and sets LastTransitionTime to now.
// The Description is always updated to reflect the current operation.
//
// Returns an error if the status update fails.
func (r *Reconciler) recordLastOperation(ctx context.Context, taskObjKey client.ObjectKey, opType v1alpha1.LastOperationType, state v1alpha1.LastOperationState, description string) error {
	runID := string(controller.ReconcileIDFromContext(ctx))
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		task, err := r.getTask(ctx, taskObjKey)
		if err != nil {
			return err
		}
		now := metav1.Time{Time: time.Now().UTC()}
		if description == "" {
			description = fmt.Sprintf("%s is in state %s for task %s", opType, state, taskObjKey.Name)
		}

		if task.Status.LastOperation == nil {
			// Initialize LastOperation if not present
			task.Status.LastOperation = &v1alpha1.LastOperation{
				Type:           opType,
				State:          state,
				LastUpdateTime: now,
				Description:    description,
				RunID:          runID,
			}
			return r.client.Status().Update(ctx, task)
		}

		phaseChanged := task.Status.LastOperation.Type != opType
		stateChanged := task.Status.LastOperation.State != state
		if phaseChanged {
			task.Status.LastOperation.Type = opType
			task.Status.LastOperation.LastUpdateTime = now
		}
		if stateChanged {
			task.Status.LastOperation.State = state
		}
		if phaseChanged || stateChanged {
			// Only update status if there was a transition
			task.Status.LastOperation.Description = description
			task.Status.LastOperation.RunID = runID
			return r.client.Status().Update(ctx, task)
		}
		return nil
	})
}

// recordTaskState sets the task's status.State to the given state and updates LastTransitionTime if the state changes.
//
// If transitioning to InProgress, sets InitiatedAt if not already set.
// Returns an error if the status update fails, or nil if no change is needed.
func (r *Reconciler) recordTaskState(ctx context.Context, taskObjKey client.ObjectKey, state v1alpha1.TaskState) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		task, err := r.getTask(ctx, taskObjKey)
		if err != nil {
			return err
		}

		stateChanged := task.Status.State == nil || *task.Status.State != state
		if !stateChanged {
			// No update needed if state is unchanged
			return nil
		}

		now := &metav1.Time{Time: time.Now().UTC()}
		if state == v1alpha1.TaskStateInProgress && task.Status.StartedAt == nil {
			// Set InitiatedAt only on first transition to InProgress
			task.Status.StartedAt = now
		}
		task.Status.State = &state
		task.Status.LastTransitionTime = now
		return r.client.Status().Update(ctx, task)
	})
}

// recordLastError appends an error to the LastErrors field in the task status.
//
// Maintains a maximum of 10 most recent errors (FIFO order: oldest errors are dropped).
// Returns an error if the status update fails.
func (r *Reconciler) recordLastError(ctx context.Context, taskObjKey client.ObjectKey, err error) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		task, getErr := r.getTask(ctx, taskObjKey)
		if getErr != nil {
			return getErr
		}
		now := metav1.Time{Time: time.Now().UTC()}

		// Use the existing MapToLastErrors utility to convert the error
		newErrors := druiderr.MapToLastErrors([]error{err})
		if len(newErrors) == 0 {
			// Fallback for non-DruidError types
			newErrors = []v1alpha1.LastError{{
				Description: err.Error(),
				ObservedAt:  now,
			}}
		} else {
			// Set ObservedAt to current time for consistency
			newErrors[0].ObservedAt = now
		}

		lastErrors := task.Status.LastErrors
		if lastErrors == nil {
			lastErrors = make([]v1alpha1.LastError, 0, 3)
		}
		if len(lastErrors) >= 3 {
			lastErrors = lastErrors[1:]
		}

		task.Status.LastErrors = append(lastErrors, newErrors[0])
		return r.client.Status().Update(ctx, task)
	})
}
