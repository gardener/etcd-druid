// SPDX-FileCopyrightcext: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/test/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// TestGetTask tests the getTask function.
func TestGetTask(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name        string
		taskName    string
		taskNS      string
		expectError bool
	}{
		{
			name:        "Should successfully get task with valid name and namespace",
			taskName:    "test-task",
			taskNS:      "test-ns",
			expectError: false,
		},
		{
			name:        "Should return error when task name is invalid",
			taskName:    "",
			taskNS:      "test-ns",
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
			task := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build()
			r := newTestReconciler(t, cl)

			err := cl.Create(ctx, task)
			g.Expect(err).NotTo(HaveOccurred())

			taskKey := client.ObjectKey{
				Name:      tc.taskName,
				Namespace: tc.taskNS,
			}
			taskObj, err := r.getTask(ctx, taskKey)
			if tc.expectError {
				g.Expect(err).To(HaveOccurred(), fmt.Sprintf("Expected error for task %s in namespace %s", tc.taskName, tc.taskNS))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(taskObj.Name).To(Equal(tc.taskName))
				g.Expect(taskObj.Namespace).To(Equal(tc.taskNS))
			}

		})

	}
}

func TestRejectTaskWithError(t *testing.T) {
	g := NewGomegaWithT(t)
	defaultTTL := int32(30)
	tests := []struct {
		name           string
		task           *druidv1alpha1.EtcdOpsTask
		description    string
		wrappedErr     error
		expectedResult ctrlutils.ReconcileStepResult
	}{
		{
			name:           "Should reject task and update status successfully",
			task:           utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithTTLSecondsAfterFinished(defaultTTL).Build(),
			description:    "Rejection due to test error",
			wrappedErr:     errors.New("test error"),
			expectedResult: ctrlutils.ReconcileAfter(time.Duration(defaultTTL)*time.Second, "Task rejected, waiting for TTL to expire before deletion"),
		},
	}

	for _, tc := range tests {
		ctx := context.Background()

		cl := utils.NewTestClientBuilder().
			WithScheme(kubernetes.Scheme).
			WithStatusSubresource(tc.task).
			Build()
		err := cl.Create(ctx, tc.task)
		g.Expect(err).NotTo(HaveOccurred())

		r := newTestReconciler(t, cl)

		result := r.rejectTaskWithError(ctx, tc.task, tc.description, tc.wrappedErr)
		updatedTask := &druidv1alpha1.EtcdOpsTask{}
		err = cl.Get(ctx, client.ObjectKeyFromObject(tc.task), updatedTask)
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(result.GetResult().RequeueAfter).To(BeNumerically("~", tc.expectedResult.GetResult().RequeueAfter, time.Second))

	}
}

// TestSetLastOperation tests the setLastOperation helper function
func TestSetLastOperation(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name                  string
		existingLastOperation bool
		expectedLastOperation *druidapicommon.LastOperation
	}{
		{
			name:                  "Should set new LastOperation when none exists",
			existingLastOperation: false,
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeAdmit,
				State:       druidv1alpha1.LastOperationStateInProgress,
				Description: "Starting reconciliation",
				RunID:       "test-run-123",
			},
		},
		{
			name:                  "Should overwrite existing LastOperation",
			existingLastOperation: true,
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeExecution,
				State:       druidv1alpha1.LastOperationStateInProgress,
				Description: "Deletion completed",
				RunID:       "new-run-456",
			},
		},
		{
			name:                  "Should generate default description when description is empty",
			existingLastOperation: false,
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeExecution,
				State:       druidv1alpha1.LastOperationStateInProgress,
				Description: "Running is in state Error for task test-task",
				RunID:       "test-run-789",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			task := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build()

			if tc.existingLastOperation {
				task.Status.LastOperation = &druidapicommon.LastOperation{
					Type:           druidv1alpha1.LastOperationTypeExecution,
					State:          druidv1alpha1.LastOperationStateCompleted,
					Description:    "Old description",
					RunID:          "old-run-id",
					LastUpdateTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				}
			}
			timeBefore := time.Now().UTC()

			setLastOperation(task, tc.expectedLastOperation.Type, tc.expectedLastOperation.State, tc.expectedLastOperation.Description, tc.expectedLastOperation.RunID)

			g.Expect(task.Status.LastOperation).ToNot(BeNil())
			g.Expect(task.Status.LastOperation.Type).To(Equal(tc.expectedLastOperation.Type))
			g.Expect(task.Status.LastOperation.State).To(Equal(tc.expectedLastOperation.State))
			g.Expect(task.Status.LastOperation.Description).To(Equal(tc.expectedLastOperation.Description))
			g.Expect(task.Status.LastOperation.RunID).To(Equal(tc.expectedLastOperation.RunID))

			actualTime := task.Status.LastOperation.LastUpdateTime.Time
			g.Expect(actualTime).To(BeTemporally(">=", timeBefore))
		})
	}
}

// TestUpdateTaskState tests the updateTaskState helper function
func TestSetTaskState(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		task           *druidv1alpha1.EtcdOpsTask
		newState       druidv1alpha1.TaskState
		expectedChange bool
		expectStarted  bool
	}{
		{
			name:           "Should set initial task state when none exists",
			task:           utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			newState:       druidv1alpha1.TaskStatePending,
			expectedChange: true,
			expectStarted:  false,
		},
		{
			name:           "Should transition state from Pending to InProgress and set StartedAt",
			task:           utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStatePending).Build(),
			newState:       druidv1alpha1.TaskStateInProgress,
			expectedChange: true,
			expectStarted:  true,
		},
		{
			name: "Should transition state from InProgress to Succeeded without modifying StartedAt",
			task: func() *druidv1alpha1.EtcdOpsTask {
				task := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build()
				task.Status.StartedAt = &metav1.Time{Time: time.Now().Add(-5 * time.Minute)}
				return task
			}(),
			newState:       druidv1alpha1.TaskStateSucceeded,
			expectedChange: true,
			expectStarted:  false,
		},
		{
			name:           "Should not change state when new state is same as current",
			task:           utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build(),
			newState:       druidv1alpha1.TaskStateInProgress,
			expectedChange: false,
			expectStarted:  false,
		},
		{
			name: "Should not modify existing StartedAt when transitioning to InProgress",
			task: func() *druidv1alpha1.EtcdOpsTask {
				task := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStatePending).Build()
				task.Status.StartedAt = &metav1.Time{Time: time.Now().Add(-10 * time.Minute)}
				return task
			}(),
			newState:       druidv1alpha1.TaskStateInProgress,
			expectedChange: true,
			expectStarted:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			originalStartedAt := tc.task.Status.StartedAt

			setTaskState(tc.task, tc.newState)

			g.Expect(tc.task.Status.State).ToNot(BeNil())
			g.Expect(*tc.task.Status.State).To(Equal(tc.newState))

			if tc.expectedChange {
				g.Expect(tc.task.Status.LastTransitionTime).ToNot(BeNil())
			}

			if tc.expectStarted {
				g.Expect(tc.task.Status.StartedAt).ToNot(BeNil())
				if originalStartedAt == nil {
					g.Expect(tc.task.Status.StartedAt).ToNot(BeZero())
				}
			} else if originalStartedAt != nil {
				g.Expect(tc.task.Status.StartedAt).To(Equal(originalStartedAt))
			}
		})
	}
}

// TestSetLastError tests the setLastError helper function
func TestSetLastError(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name                string
		task                *druidv1alpha1.EtcdOpsTask
		initialErrorSize    int
		error               error
		expectedErrorsCount int
	}{
		{
			name:                "Should add first error when no initial errors exist",
			task:                utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			initialErrorSize:    0,
			error:               druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error"),
			expectedErrorsCount: 1,
		},
		{
			name:                "Should add error when total count is below maximum",
			task:                utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			initialErrorSize:    1,
			error:               druiderr.WrapError(fmt.Errorf("another test error"), "AnotherTestError", "TestOperation", "This is another test error"),
			expectedErrorsCount: 2,
		},
		{
			name:                "Should add error when reaching maximum limit",
			task:                utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			initialErrorSize:    2,
			error:               druiderr.WrapError(fmt.Errorf("yet another test error"), "YetAnotherTestError", "TestOperation", "This is yet another test error"),
			expectedErrorsCount: 3,
		},
		{
			name:                "Should remove oldest error when exceeding maximum limit",
			task:                utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			initialErrorSize:    3,
			error:               druiderr.WrapError(fmt.Errorf("fourth test error"), "FourthTestError", "TestOperation", "This is the fourth test error"),
			expectedErrorsCount: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.initialErrorSize > 0 {
				for i := 0; i < tc.initialErrorSize; i++ {
					tc.task.Status.LastErrors = append(tc.task.Status.LastErrors, druidapicommon.LastError{
						Code:        druidapicommon.ErrorCode(fmt.Sprintf("InitialError%d", i)),
						Description: fmt.Sprintf("initial error %d", i),
						ObservedAt:  metav1.Time{Time: time.Now().Add(-time.Duration(i) * time.Minute)},
					})
				}
			}

			setLastError(tc.task, tc.error)

			g.Expect(tc.task.Status.LastErrors).ToNot(BeEmpty())
			g.Expect(tc.task.Status.LastErrors).To(HaveLen(tc.expectedErrorsCount))
			g.Expect(len(tc.task.Status.LastErrors)).To(BeNumerically("<=", 3))

			lastErrorIndex := len(tc.task.Status.LastErrors) - 1
			lastError := tc.task.Status.LastErrors[lastErrorIndex]

			g.Expect(lastError.ObservedAt).ToNot(BeZero())

			if druidErr, ok := tc.error.(*druiderr.DruidError); ok {
				expectedLastErrors := &druidapicommon.LastError{
					Code: druidapicommon.ErrorCode(druidErr.Code),
				}
				utils.CheckLastErrors(g, tc.task.Status.LastErrors, expectedLastErrors)
			} else {
				g.Expect(lastError.Description).To(Equal(tc.error.Error()))
			}

			if tc.initialErrorSize >= 2 {
				size := len(tc.task.Status.LastErrors)
				g.Expect(size).To(Equal(3))
			}
		})
	}
}

// TestUpdateTaskStatus tests the updateTaskStatus method
func TestUpdateTaskStatus(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name                string
		initialTask         *druidv1alpha1.EtcdOpsTask
		update              taskStatusUpdate
		expectError         bool
		expectedOpType      *druidapicommon.LastOperationType
		expectedOpState     *druidapicommon.LastOperationState
		expectedTaskState   *druidv1alpha1.TaskState
		expectedErrorsCount int
	}{
		{
			name:        "Should update last operation only when operation is provided",
			initialTask: utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			update: taskStatusUpdate{
				Operation: &druidapicommon.LastOperation{
					Type:        druidv1alpha1.LastOperationTypeReconcile,
					State:       druidv1alpha1.LastOperationStateRequeue,
					Description: "Starting admission",
				},
			},
			expectError:       false,
			expectedOpType:    ptr.To(druidv1alpha1.LastOperationTypeReconcile),
			expectedOpState:   ptr.To(druidv1alpha1.LastOperationStateRequeue),
			expectedTaskState: nil,
		},
		{
			name:        "Should update task state only when state is provided",
			initialTask: utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			update: taskStatusUpdate{
				State: ptr.To(druidv1alpha1.TaskStateInProgress),
			},
			expectError:       false,
			expectedOpType:    nil,
			expectedOpState:   nil,
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateInProgress),
		},
		{
			name:        "Should update error only when error is provided",
			initialTask: utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			update: taskStatusUpdate{
				Error: druiderr.WrapError(errors.New("test error"), "TEST_ERROR", "TestOp", "Test error"),
			},
			expectError:         false,
			expectedOpType:      nil,
			expectedOpState:     nil,
			expectedTaskState:   nil,
			expectedErrorsCount: 1,
		},
		{
			name:        "Should update all fields when all are provided",
			initialTask: utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			update: taskStatusUpdate{
				Operation: &druidapicommon.LastOperation{
					Type:        druidv1alpha1.LastOperationTypeReconcile,
					State:       druidv1alpha1.LastOperationStateError,
					Description: "Task failed",
				},
				State: ptr.To(druidv1alpha1.TaskStateFailed),
				Error: druiderr.WrapError(errors.New("execution failed"), "EXEC_ERROR", "RunOp", "Execution failed"),
			},
			expectError:         false,
			expectedOpType:      ptr.To(druidv1alpha1.LastOperationTypeReconcile),
			expectedOpState:     ptr.To(druidv1alpha1.LastOperationStateError),
			expectedTaskState:   ptr.To(druidv1alpha1.TaskStateFailed),
			expectedErrorsCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			cl := utils.NewTestClientBuilder().
				WithScheme(kubernetes.Scheme).
				WithStatusSubresource(tc.initialTask).
				Build()
			err := cl.Create(ctx, tc.initialTask)
			g.Expect(err).NotTo(HaveOccurred())

			r := newTestReconciler(t, cl)
			taskKey := client.ObjectKey{
				Name:      tc.initialTask.Name,
				Namespace: tc.initialTask.Namespace,
			}

			err = r.updateTaskStatus(ctx, tc.initialTask, tc.update)

			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(ctx, taskKey, updatedTask)
			g.Expect(err).NotTo(HaveOccurred())

			if tc.expectedOpType != nil {
				g.Expect(updatedTask.Status.LastOperation).ToNot(BeNil())
				g.Expect(updatedTask.Status.LastOperation.Type).To(Equal(*tc.expectedOpType))
			}
			if tc.expectedOpState != nil {
				g.Expect(updatedTask.Status.LastOperation).ToNot(BeNil())
				g.Expect(updatedTask.Status.LastOperation.State).To(Equal(*tc.expectedOpState))
			}

			if tc.expectedTaskState != nil {
				g.Expect(updatedTask.Status.State).ToNot(BeNil())
				g.Expect(*updatedTask.Status.State).To(Equal(*tc.expectedTaskState))
			}

			if tc.expectedErrorsCount > 0 {
				g.Expect(updatedTask.Status.LastErrors).To(HaveLen(tc.expectedErrorsCount))
			}
		})
	}
}

// TestHandleTaskResult tests the handleTaskResult method
func TestHandleTaskResult(t *testing.T) {
	tests := []struct {
		name     string
		result   handler.Result
		phase    druidapicommon.LastOperationType
		expected string
	}{
		{
			name:     "Should requeue when handler returns requeue with error",
			result:   handler.Result{Requeue: true, Error: fmt.Errorf("test error"), Description: "test description"},
			phase:    druidv1alpha1.LastOperationTypeExecution,
			expected: "requeue",
		},
		{
			name:     "Should requeue when handler returns requeue without error",
			result:   handler.Result{Requeue: true, Description: "test description"},
			phase:    druidv1alpha1.LastOperationTypeAdmit,
			expected: "requeue",
		},
		{
			name:     "Should handle error when handler returns error without requeue",
			result:   handler.Result{Error: fmt.Errorf("test error"), Description: "test description"},
			phase:    druidv1alpha1.LastOperationTypeExecution,
			expected: "error",
		},
		{
			name:     "Should handle success when handler completes without error or requeue",
			result:   handler.Result{Description: "test description"},
			phase:    druidv1alpha1.LastOperationTypeAdmit,
			expected: "success",
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			task := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build()
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithStatusSubresource(task).Build()
			err := cl.Create(ctx, task)
			g.Expect(err).ToNot(HaveOccurred())

			reconciler := newTestReconciler(t, cl)
			result := reconciler.handleTaskResult(ctx, reconciler.logger, task, tc.result, tc.phase)

			switch tc.expected {
			case "requeue":
				g.Expect(result.NeedsRequeue()).To(BeTrue())
				if tc.result.Error != nil {
					g.Expect(result.HasErrors()).To(BeTrue())
					g.Expect(result.GetCombinedError().Error()).To(Equal(tc.result.Error.Error()))
				} else {
					g.Expect(result.GetResult().RequeueAfter).To(Equal(reconciler.config.RequeueInterval.Duration))
				}
			case "error":
				g.Expect(result.NeedsRequeue()).To(BeTrue())
				if tc.phase == druidv1alpha1.LastOperationTypeCleanup {
					g.Expect(result.HasErrors()).To(BeTrue())
				} else {
					g.Expect(result.HasErrors()).To(BeFalse())
				}
			case "success":
				if tc.phase == druidv1alpha1.LastOperationTypeExecution {
					g.Expect(result.NeedsRequeue()).To(BeTrue())
					g.Expect(result.HasErrors()).To(BeFalse())
				} else {
					g.Expect(result.NeedsRequeue()).To(BeFalse())
				}
			}
		})
	}
}

// TestHandleRequeue tests the handleRequeue method
func TestHandleRequeue(t *testing.T) {
	tests := []struct {
		name               string
		result             handler.Result
		expectedOperation  druidapicommon.LastOperation
		expectedMessage    string
		expectError        bool
		expectRequeueAfter bool
	}{
		{
			name:               "Should requeue for admit phase result with no error",
			result:             handler.Result{Requeue: true, Description: "test description"},
			expectedOperation:  druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeAdmit, State: druidv1alpha1.LastOperationStateInProgress, Description: "test description"},
			expectedMessage:    "Task admit in progress",
			expectError:        false,
			expectRequeueAfter: true,
		},
		{
			name:               "Should requeue with error for admit phase result with error",
			result:             handler.Result{Requeue: true, Error: fmt.Errorf("test error"), Description: "test description"},
			expectedOperation:  druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeAdmit, State: druidv1alpha1.LastOperationStateInProgress, Description: "test description"},
			expectedMessage:    "Task admit in progress",
			expectError:        true,
			expectRequeueAfter: false,
		},
		{
			name:               "Should requeue for execution phase result with no error",
			result:             handler.Result{Requeue: true, Description: "test description"},
			expectedOperation:  druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeExecution, State: druidv1alpha1.LastOperationStateInProgress, Description: "test description"},
			expectedMessage:    "Task running in progress",
			expectError:        false,
			expectRequeueAfter: true,
		},
		{
			name:               "Should requeue with error for execution phase result with error",
			result:             handler.Result{Requeue: true, Error: fmt.Errorf("test error"), Description: "test description"},
			expectedOperation:  druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeExecution, State: druidv1alpha1.LastOperationStateInProgress, Description: "test description"},
			expectedMessage:    "Task running in progress",
			expectError:        true,
			expectRequeueAfter: false,
		},
		{
			name:               "Should requeue for cleanup phase result with no error",
			result:             handler.Result{Requeue: true, Description: "cleanup description"},
			expectedOperation:  druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeCleanup, State: druidv1alpha1.LastOperationStateInProgress, Description: "cleanup description"},
			expectedMessage:    "Task cleanup in progress",
			expectError:        false,
			expectRequeueAfter: true,
		},
		{
			name:               "Should requeue with error for cleanup phase result with error",
			result:             handler.Result{Requeue: true, Error: fmt.Errorf("cleanup error"), Description: "cleanup description"},
			expectedOperation:  druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeCleanup, State: druidv1alpha1.LastOperationStateInProgress, Description: "cleanup description"},
			expectedMessage:    "Task cleanup in progress",
			expectError:        true,
			expectRequeueAfter: false,
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			task := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build()
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithStatusSubresource(task).Build()
			err := cl.Create(ctx, task)
			g.Expect(err).ToNot(HaveOccurred())

			reconciler := newTestReconciler(t, cl)
			taskObjKey := client.ObjectKeyFromObject(task)

			result := reconciler.handleRequeue(ctx, reconciler.logger, task, tc.result, tc.expectedOperation.Type)

			g.Expect(result.NeedsRequeue()).To(BeTrue())
			if tc.expectError {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError().Error()).To(Equal(tc.result.Error.Error()))
			} else if tc.expectRequeueAfter {
				g.Expect(result.GetResult().RequeueAfter).To(Equal(reconciler.config.RequeueInterval.Duration))
			}

			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(ctx, taskObjKey, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(updatedTask.Status.LastOperation).ToNot(BeNil())
			g.Expect(updatedTask.Status.LastOperation.Type).To(Equal(tc.expectedOperation.Type))
			g.Expect(updatedTask.Status.LastOperation.State).To(Equal(tc.expectedOperation.State))
			g.Expect(updatedTask.Status.LastOperation.Description).To(Equal(tc.expectedOperation.Description))

			if tc.result.Error != nil {
				g.Expect(updatedTask.Status.LastErrors).To(HaveLen(1))
			}
		})
	}
}

// TestHandleError tests the handleError method
func TestHandleError(t *testing.T) {
	tests := []struct {
		name              string
		result            handler.Result
		expectedOperation druidapicommon.LastOperation
		expectedTaskState *druidv1alpha1.TaskState
		expectedErrorMsg  string
	}{
		{
			name:              "Should mark task as rejected task when error occurs in admit phase",
			result:            handler.Result{Error: fmt.Errorf("admit error"), Description: "admit failed"},
			expectedOperation: druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeAdmit, State: druidv1alpha1.LastOperationStateFailed, Description: "admit failed"},
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateRejected),
			expectedErrorMsg:  "task rejected, handing to deletion flow",
		},
		{
			name:              "Should mark task as failed when error occurs in execution phase",
			result:            handler.Result{Error: fmt.Errorf("run error"), Description: "run failed"},
			expectedOperation: druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeExecution, State: druidv1alpha1.LastOperationStateFailed, Description: "run failed"},
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateFailed),
			expectedErrorMsg:  "task completed",
		},
		{
			name:              "Should return error when error occurs in cleanup phase",
			result:            handler.Result{Error: fmt.Errorf("cleanup error"), Description: "cleanup failed"},
			expectedOperation: druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeCleanup, State: druidv1alpha1.LastOperationStateFailed, Description: "cleanup failed"},
			expectedTaskState: nil,
			expectedErrorMsg:  "cleanup failed",
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			task := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build()
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithStatusSubresource(task).Build()
			err := cl.Create(ctx, task)
			g.Expect(err).ToNot(HaveOccurred())

			reconciler := newTestReconciler(t, cl)
			taskObjKey := client.ObjectKeyFromObject(task)

			result := reconciler.handleError(ctx, reconciler.logger, task, tc.result, tc.expectedOperation.Type)

			g.Expect(result.NeedsRequeue()).To(BeTrue())

			if tc.expectedOperation.Type == druidv1alpha1.LastOperationTypeCleanup {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError().Error()).To(Equal(tc.expectedErrorMsg))
			} else {
				g.Expect(result.HasErrors()).To(BeFalse())
				g.Expect(result.GetResult().RequeueAfter).To(BeNumerically(">", 0))
			}

			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(ctx, taskObjKey, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(updatedTask.Status.LastOperation).ToNot(BeNil())
			g.Expect(updatedTask.Status.LastOperation.Type).To(Equal(tc.expectedOperation.Type))
			g.Expect(updatedTask.Status.LastOperation.State).To(Equal(tc.expectedOperation.State))
			g.Expect(updatedTask.Status.LastOperation.Description).To(Equal(tc.expectedOperation.Description))

			if tc.expectedTaskState != nil {
				g.Expect(updatedTask.Status.State).ToNot(BeNil())
				g.Expect(*updatedTask.Status.State).To(Equal(*tc.expectedTaskState))
			}

			g.Expect(updatedTask.Status.LastErrors).To(HaveLen(1))
		})
	}
}

// TestHandleSuccess tests the handleSuccess method
func TestHandleSuccess(t *testing.T) {
	tests := []struct {
		name                string
		result              handler.Result
		expectedOperation   druidapicommon.LastOperation
		expectedTaskState   *druidv1alpha1.TaskState
		expectContinue      bool
		expectTaskCompleted bool
	}{
		{
			name:                "Should continue reconciliation when admit phase succeeds",
			result:              handler.Result{Description: "admit successful"},
			expectedOperation:   druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeAdmit, State: druidv1alpha1.LastOperationStateCompleted, Description: "admit successful"},
			expectedTaskState:   nil, // No state change for admit success
			expectContinue:      true,
			expectTaskCompleted: false,
		},
		{
			name:                "Should mark task as succeeded when execution phase completes",
			result:              handler.Result{Description: "run successful"},
			expectedOperation:   druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeExecution, State: druidv1alpha1.LastOperationStateCompleted, Description: "run successful"},
			expectedTaskState:   ptr.To(druidv1alpha1.TaskStateSucceeded),
			expectContinue:      false,
			expectTaskCompleted: true,
		},
		{
			name:                "Should continue reconciliation when cleanup phase succeeds",
			result:              handler.Result{Description: "cleanup successful"},
			expectedOperation:   druidapicommon.LastOperation{Type: druidv1alpha1.LastOperationTypeCleanup, State: druidv1alpha1.LastOperationStateInProgress, Description: "cleanup successful"},
			expectedTaskState:   nil,
			expectContinue:      true,
			expectTaskCompleted: false,
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			task := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build()
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithStatusSubresource(task).Build()
			err := cl.Create(ctx, task)
			g.Expect(err).ToNot(HaveOccurred())

			reconciler := newTestReconciler(t, cl)
			taskObjKey := client.ObjectKeyFromObject(task)

			result := reconciler.handleSuccess(ctx, reconciler.logger, task, tc.result, tc.expectedOperation.Type)

			if tc.expectContinue {
				g.Expect(result.NeedsRequeue()).To(BeFalse())
			} else if tc.expectTaskCompleted {
				g.Expect(result.NeedsRequeue()).To(BeTrue())
				if tc.expectedOperation.Type == druidv1alpha1.LastOperationTypeExecution {
					g.Expect(result.HasErrors()).To(BeFalse())
					g.Expect(result.GetResult().RequeueAfter).To(BeNumerically(">", 0))
				} else {
					g.Expect(result.HasErrors()).To(BeTrue())
					g.Expect(result.GetCombinedError().Error()).To(Equal("task completed"))
				}
			}

			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(ctx, taskObjKey, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(updatedTask.Status.LastOperation).ToNot(BeNil())
			g.Expect(updatedTask.Status.LastOperation.Type).To(Equal(tc.expectedOperation.Type))
			g.Expect(updatedTask.Status.LastOperation.State).To(Equal(tc.expectedOperation.State))
			g.Expect(updatedTask.Status.LastOperation.Description).To(Equal(tc.expectedOperation.Description))

			if tc.expectedTaskState != nil {
				g.Expect(updatedTask.Status.State).ToNot(BeNil())
				g.Expect(*updatedTask.Status.State).To(Equal(*tc.expectedTaskState))
			}
		})
	}
}
