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

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr/testr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// newTestReconciler creates a new Reconciler instance for testing.
func newTestReconciler(t *testing.T, cl client.Client) *Reconciler {
	return &Reconciler{
		logger: testr.New(t),
		client: cl,
		config: &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
			RequeueInterval: &metav1.Duration{Duration: 60 * time.Second},
		},
	}
}

// newTestTask creates a new EtcdOpsTask for tests.
func newTestTask(state *druidv1alpha1.TaskState) *druidv1alpha1.EtcdOpsTask {
	now := metav1.Now()
	ts := &druidv1alpha1.EtcdOpsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
			Namespace: "test-ns",
		},
		Spec: druidv1alpha1.EtcdOpsTaskSpec{
			Config:                  druidv1alpha1.EtcdOpsTaskConfig{},
			TTLSecondsAfterFinished: ptr.To(int32(60)),
			EtcdName:                ptr.To("test-etcd"),
		},
		Status: druidv1alpha1.EtcdOpsTaskStatus{
			LastTransitionTime: &now,
		},
	}
	if state != nil {
		ts.Status.State = state
	}
	return ts
}

// setupFakeClient creates a fake client with the given task and status.
func setupFakeClient(task *druidv1alpha1.EtcdOpsTask, status bool) client.Client {
	if status {
		return utils.NewTestClientBuilder().
			WithScheme(kubernetes.Scheme).
			WithStatusSubresource(task).
			Build()
	}
	return utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
}

// checksForNilTask checks the expected scenarios for cases where the task isn't present.
func checksForNilTask(g *WithT, result ctrlutils.ReconcileStepResult) {
	g.Expect(result.HasErrors()).To(BeTrue())
	g.Expect(result.GetCombinedError()).To(HaveOccurred())
	g.Expect(result.GetCombinedError().Error()).To(ContainSubstring("not found"))
}

// expectDruidErrors checks if the actual errors match the expected Druid errors.
func expectDruidErrors(g Gomega, actual, expected []error) {
	g.Expect(actual).To(HaveLen(len(expected)))
	for i, err := range actual {
		expectedErr := expected[i]
		var druidErr *druiderr.DruidError
		var expectedDruidErr *druiderr.DruidError
		if errors.As(err, &druidErr) && errors.As(expectedErr, &expectedDruidErr) {
			if expectedDruidErr, ok := expectedErr.(*druiderr.DruidError); ok {
				g.Expect(druidErr.Code).To(Equal(expectedDruidErr.Code))
				g.Expect(druidErr.Operation).To(Equal(expectedDruidErr.Operation))
				g.Expect(druidErr.Message).To(Equal(expectedDruidErr.Message))
			}
		}
	}
}

// checkLastErrors checks the LastErrors field in the task status with the expected values.
func checkLastErrors(g *WithT, updatedTask *druidv1alpha1.EtcdOpsTask, expectedLastErrors *druidv1alpha1.LastError) {
	if expectedLastErrors != nil {
		index := len(updatedTask.Status.LastErrors) - 1
		g.Expect(updatedTask.Status.LastErrors).ToNot(BeNil())
		g.Expect(updatedTask.Status.LastErrors[index].Code).To(Equal(expectedLastErrors.Code))
	} else {
		g.Expect(updatedTask.Status.LastErrors).To(BeNil())
	}
}

// checkLastOperation checks the LastOperation field in the task status with the expected values.
func checkLastOperation(g *WithT, updatedTask *druidv1alpha1.EtcdOpsTask, expectedLastOperation *druidv1alpha1.LastOperation) {
	if expectedLastOperation != nil {
		g.Expect(updatedTask.Status.LastOperation).ToNot(BeNil())
		g.Expect(updatedTask.Status.LastOperation.Type).To(Equal(expectedLastOperation.Type))
		g.Expect(updatedTask.Status.LastOperation.State).To(Equal(expectedLastOperation.State))
		if expectedLastOperation.Description != "" {
			g.Expect(updatedTask.Status.LastOperation.Description).To(Equal(expectedLastOperation.Description))
		}
	}
}

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
			name:        "Valid task name and namespace",
			taskName:    "test-task",
			taskNS:      "test-ns",
			expectError: false,
		},
		{
			name:        "Invalid task name",
			taskName:    "",
			taskNS:      "test-ns",
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl := setupFakeClient(nil, false)
			task := newTestTask(nil)
			err := cl.Create(context.TODO(), task)
			g.Expect(err).NotTo(HaveOccurred())
			r := newTestReconciler(t, cl)

			taskKey := client.ObjectKey{
				Name:      tc.taskName,
				Namespace: tc.taskNS,
			}
			taskObj, err := r.getTask(context.TODO(), taskKey)
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

// TestSetLastOperation tests the setLastOperation helper function
func TestSetLastOperation(t *testing.T) {
	tests := []struct {
		name                  string
		existingLastOperation bool
		lastOperation         *druidv1alpha1.LastOperation
		expectedLastOperation *druidv1alpha1.LastOperation
	}{
		{
			name:                  "new LastOperation when none exists",
			existingLastOperation: false,
			lastOperation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateProcessing,
				Description: "Starting reconciliation",
				RunID:       "test-run-123",
			},
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateProcessing,
				Description: "Starting reconciliation",
				RunID:       "test-run-123",
			},
		},
		{
			name:                  "sets even if already present",
			existingLastOperation: true,
			lastOperation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeDelete,
				State:       druidv1alpha1.LastOperationStateSucceeded,
				Description: "Deletion completed",
				RunID:       "new-run-456",
			},
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeDelete,
				State:       druidv1alpha1.LastOperationStateSucceeded,
				Description: "Deletion completed",
				RunID:       "new-run-456",
			},
		},
		{
			name:                  "if description isn't set",
			existingLastOperation: false,
			lastOperation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateError,
				Description: "",
				RunID:       "test-run-789",
			},
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeReconcile,
				State:       druidv1alpha1.LastOperationStateError,
				Description: "Reconcile is in state Error for task test-task",
				RunID:       "test-run-789",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t) // Create test task using newTestTask(nil)
			task := newTestTask(nil)

			if tc.existingLastOperation {
				task.Status.LastOperation = &druidv1alpha1.LastOperation{
					Type:           druidv1alpha1.LastOperationTypeReconcile,
					State:          druidv1alpha1.LastOperationStateProcessing,
					Description:    "Old description",
					RunID:          "old-run-id",
					LastUpdateTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				}
			}
			timeBefore := time.Now().UTC()

			setLastOperation(task, tc.lastOperation.Type, tc.lastOperation.State, tc.lastOperation.Description, tc.lastOperation.RunID)

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
	tests := []struct {
		name           string
		task           *druidv1alpha1.EtcdOpsTask
		newState       druidv1alpha1.TaskState
		expectedChange bool
		expectStarted  bool
	}{
		{
			name:           "initial state from nil",
			task:           newTestTask(nil),
			newState:       druidv1alpha1.TaskStatePending,
			expectedChange: true,
			expectStarted:  false,
		},
		{
			name:           "state change from Pending to InProgress",
			task:           newTestTask(ptr.To(druidv1alpha1.TaskStatePending)),
			newState:       druidv1alpha1.TaskStateInProgress,
			expectedChange: true,
			expectStarted:  true,
		},
		{
			name: "state change from InProgress to Succeeded",
			task: func() *druidv1alpha1.EtcdOpsTask {
				task := newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress))
				task.Status.StartedAt = &metav1.Time{Time: time.Now().Add(-5 * time.Minute)}
				return task
			}(),
			newState:       druidv1alpha1.TaskStateSucceeded,
			expectedChange: true,
			expectStarted:  false,
		},
		{
			name:           "no change when state is same",
			task:           newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress)),
			newState:       druidv1alpha1.TaskStateInProgress,
			expectedChange: false,
			expectStarted:  false,
		},
		{
			name: "transition to InProgress when StartedAt already exists",
			task: func() *druidv1alpha1.EtcdOpsTask {
				task := newTestTask(ptr.To(druidv1alpha1.TaskStatePending))
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
			g := NewGomegaWithT(t)

			originalStartedAt := tc.task.Status.StartedAt

			changed := setTaskState(tc.task, tc.newState)

			g.Expect(changed).To(Equal(tc.expectedChange))
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
	tests := []struct {
		name                string
		task                *druidv1alpha1.EtcdOpsTask
		initialErrorSize    int
		error               error
		expectedErrorsCount int
	}{
		{
			name:                "No initial last error",
			task:                newTestTask(nil),
			initialErrorSize:    0,
			error:               druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error"),
			expectedErrorsCount: 1,
		},
		{
			name:                "Initial error exists, but total size is less than 3",
			task:                newTestTask(nil),
			initialErrorSize:    1,
			error:               druiderr.WrapError(fmt.Errorf("another test error"), "AnotherTestError", "TestOperation", "This is another test error"),
			expectedErrorsCount: 2,
		},
		{
			name:                "Initial error exists, total size equals max limit (3)",
			task:                newTestTask(nil),
			initialErrorSize:    2,
			error:               druiderr.WrapError(fmt.Errorf("yet another test error"), "YetAnotherTestError", "TestOperation", "This is yet another test error"),
			expectedErrorsCount: 3,
		},
		{
			name:                "Exceeds max limit, should remove oldest error",
			task:                newTestTask(nil),
			initialErrorSize:    3,
			error:               druiderr.WrapError(fmt.Errorf("fourth test error"), "FourthTestError", "TestOperation", "This is the fourth test error"),
			expectedErrorsCount: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			if tc.initialErrorSize > 0 {
				for i := 0; i < tc.initialErrorSize; i++ {
					tc.task.Status.LastErrors = append(tc.task.Status.LastErrors, druidv1alpha1.LastError{
						Code:        druidv1alpha1.ErrorCode(fmt.Sprintf("InitialError%d", i)),
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
				expectedLastErrors := &druidv1alpha1.LastError{
					Code: druidv1alpha1.ErrorCode(druidErr.Code),
				}
				checkLastErrors(g, tc.task, expectedLastErrors)
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
	tests := []struct {
		name                string
		initialTask         *druidv1alpha1.EtcdOpsTask
		update              taskStatusUpdate
		expectError         bool
		expectedOpType      *druidv1alpha1.LastOperationType
		expectedOpState     *druidv1alpha1.LastOperationState
		expectedTaskState   *druidv1alpha1.TaskState
		expectedErrorsCount int
	}{
		{
			name:        "update operation only",
			initialTask: newTestTask(nil),
			update: taskStatusUpdate{
				Operation: &druidv1alpha1.LastOperation{
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
			name:        "update state only",
			initialTask: newTestTask(nil),
			update: taskStatusUpdate{
				State: ptr.To(druidv1alpha1.TaskStateInProgress),
			},
			expectError:       false,
			expectedOpType:    nil,
			expectedOpState:   nil,
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateInProgress),
		},
		{
			name:        "update error only",
			initialTask: newTestTask(nil),
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
			name:        "update all fields",
			initialTask: newTestTask(nil),
			update: taskStatusUpdate{
				Operation: &druidv1alpha1.LastOperation{
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
			g := NewGomegaWithT(t)

			cl := setupFakeClient(tc.initialTask, true)
			err := cl.Create(context.TODO(), tc.initialTask)
			g.Expect(err).NotTo(HaveOccurred())

			r := newTestReconciler(t, cl)
			taskKey := client.ObjectKey{
				Name:      tc.initialTask.Name,
				Namespace: tc.initialTask.Namespace,
			}

			err = r.updateTaskStatus(context.TODO(), tc.initialTask, tc.update)

			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(context.TODO(), taskKey, updatedTask)
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
		phase    druidv1alpha1.OperationPhase
		expected string
	}{
		{
			name:     "requeue with error",
			result:   handler.Result{Requeue: true, Error: fmt.Errorf("test error"), Description: "test description"},
			phase:    druidv1alpha1.OperationPhaseRunning,
			expected: "requeue",
		},
		{
			name:     "requeue without error",
			result:   handler.Result{Requeue: true, Description: "test description"},
			phase:    druidv1alpha1.OperationPhaseAdmit,
			expected: "requeue",
		},
		{
			name:     "error without requeue",
			result:   handler.Result{Error: fmt.Errorf("test error"), Description: "test description"},
			phase:    druidv1alpha1.OperationPhaseRunning,
			expected: "error",
		},
		{
			name:     "success",
			result:   handler.Result{Description: "test description"},
			phase:    druidv1alpha1.OperationPhaseAdmit,
			expected: "success",
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			task := newTestTask(nil)
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
				if tc.phase == druidv1alpha1.OperationPhaseCleanup {
					g.Expect(result.HasErrors()).To(BeTrue())
				} else {
					g.Expect(result.HasErrors()).To(BeFalse())
				}
			case "success":
				if tc.phase == druidv1alpha1.OperationPhaseRunning {
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
		phase              druidv1alpha1.OperationPhase
		expectedOpType     druidv1alpha1.LastOperationType
		expectedMessage    string
		expectError        bool
		expectRequeueAfter bool
	}{
		{
			name:               "requeue with error - reconcile phase",
			result:             handler.Result{Requeue: true, Error: fmt.Errorf("test error"), Description: "test description"},
			phase:              druidv1alpha1.OperationPhaseRunning,
			expectedOpType:     druidv1alpha1.LastOperationTypeReconcile,
			expectedMessage:    "Task running in progress",
			expectError:        true,
			expectRequeueAfter: false,
		},
		{
			name:               "requeue without error - reconcile phase",
			result:             handler.Result{Requeue: true, Description: "test description"},
			phase:              druidv1alpha1.OperationPhaseAdmit,
			expectedOpType:     druidv1alpha1.LastOperationTypeReconcile,
			expectedMessage:    "Task admit in progress",
			expectError:        false,
			expectRequeueAfter: true,
		},
		{
			name:               "requeue with error - cleanup phase",
			result:             handler.Result{Requeue: true, Error: fmt.Errorf("cleanup error"), Description: "cleanup description"},
			phase:              druidv1alpha1.OperationPhaseCleanup,
			expectedOpType:     druidv1alpha1.LastOperationTypeDelete,
			expectedMessage:    "Task cleanup in progress",
			expectError:        true,
			expectRequeueAfter: false,
		},
		{
			name:               "requeue without error - cleanup phase",
			result:             handler.Result{Requeue: true, Description: "cleanup description"},
			phase:              druidv1alpha1.OperationPhaseCleanup,
			expectedOpType:     druidv1alpha1.LastOperationTypeDelete,
			expectedMessage:    "Task cleanup in progress",
			expectError:        false,
			expectRequeueAfter: true,
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			task := newTestTask(nil)
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithStatusSubresource(task).Build()
			err := cl.Create(ctx, task)
			g.Expect(err).ToNot(HaveOccurred())

			reconciler := newTestReconciler(t, cl)
			taskObjKey := client.ObjectKeyFromObject(task)

			result := reconciler.handleRequeue(ctx, reconciler.logger, task, tc.result, tc.phase)

			if tc.expectError {
				g.Expect(result.NeedsRequeue()).To(BeTrue())
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError().Error()).To(Equal(tc.result.Error.Error()))
			} else if tc.expectRequeueAfter {
				g.Expect(result.NeedsRequeue()).To(BeTrue())
				g.Expect(result.GetResult().RequeueAfter).To(Equal(reconciler.config.RequeueInterval.Duration))
			}

			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(ctx, taskObjKey, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(updatedTask.Status.LastOperation).ToNot(BeNil())
			g.Expect(updatedTask.Status.LastOperation.Type).To(Equal(tc.expectedOpType))
			g.Expect(updatedTask.Status.LastOperation.State).To(Equal(druidv1alpha1.LastOperationStateRequeue))
			g.Expect(updatedTask.Status.LastOperation.Description).To(Equal(tc.result.Description))

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
		phase             druidv1alpha1.OperationPhase
		expectedOpType    druidv1alpha1.LastOperationType
		expectedTaskState *druidv1alpha1.TaskState
		expectedErrorMsg  string
	}{
		{
			name:              "error in admit phase",
			result:            handler.Result{Error: fmt.Errorf("admit error"), Description: "admit failed"},
			phase:             druidv1alpha1.OperationPhaseAdmit,
			expectedOpType:    druidv1alpha1.LastOperationTypeReconcile,
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateRejected),
			expectedErrorMsg:  "task rejected, handing to deletion flow",
		},
		{
			name:              "error in running phase",
			result:            handler.Result{Error: fmt.Errorf("run error"), Description: "run failed"},
			phase:             druidv1alpha1.OperationPhaseRunning,
			expectedOpType:    druidv1alpha1.LastOperationTypeReconcile,
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateFailed),
			expectedErrorMsg:  "task completed",
		},
		{
			name:              "error in cleanup phase",
			result:            handler.Result{Error: fmt.Errorf("cleanup error"), Description: "cleanup failed"},
			phase:             druidv1alpha1.OperationPhaseCleanup,
			expectedOpType:    druidv1alpha1.LastOperationTypeDelete,
			expectedTaskState: nil,
			expectedErrorMsg:  "cleanup failed",
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			task := newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress))
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithStatusSubresource(task).Build()
			err := cl.Create(ctx, task)
			g.Expect(err).ToNot(HaveOccurred())

			reconciler := newTestReconciler(t, cl)
			taskObjKey := client.ObjectKeyFromObject(task)

			result := reconciler.handleError(ctx, reconciler.logger, task, tc.result, tc.phase)

			g.Expect(result.NeedsRequeue()).To(BeTrue())

			if tc.phase == druidv1alpha1.OperationPhaseCleanup {
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
			g.Expect(updatedTask.Status.LastOperation.Type).To(Equal(tc.expectedOpType))
			g.Expect(updatedTask.Status.LastOperation.State).To(Equal(druidv1alpha1.LastOperationStateError))
			g.Expect(updatedTask.Status.LastOperation.Description).To(Equal(tc.result.Description))

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
		phase               druidv1alpha1.OperationPhase
		expectedOpType      druidv1alpha1.LastOperationType
		expectedOpState     druidv1alpha1.LastOperationState
		expectedTaskState   *druidv1alpha1.TaskState
		expectContinue      bool
		expectTaskCompleted bool
	}{
		{
			name:                "success in admit phase",
			result:              handler.Result{Description: "admit successful"},
			phase:               druidv1alpha1.OperationPhaseAdmit,
			expectedOpType:      druidv1alpha1.LastOperationTypeReconcile,
			expectedOpState:     druidv1alpha1.LastOperationStateProcessing,
			expectedTaskState:   nil, // No state change for admit success
			expectContinue:      true,
			expectTaskCompleted: false,
		},
		{
			name:                "success in running phase",
			result:              handler.Result{Description: "run successful"},
			phase:               druidv1alpha1.OperationPhaseRunning,
			expectedOpType:      druidv1alpha1.LastOperationTypeReconcile,
			expectedOpState:     druidv1alpha1.LastOperationStateSucceeded,
			expectedTaskState:   ptr.To(druidv1alpha1.TaskStateSucceeded),
			expectContinue:      false,
			expectTaskCompleted: true,
		},
		{
			name:                "success in cleanup phase",
			result:              handler.Result{Description: "cleanup successful"},
			phase:               druidv1alpha1.OperationPhaseCleanup,
			expectedOpType:      druidv1alpha1.LastOperationTypeDelete,
			expectedOpState:     druidv1alpha1.LastOperationStateProcessing,
			expectedTaskState:   nil,
			expectContinue:      true,
			expectTaskCompleted: false,
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			task := newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress))
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithStatusSubresource(task).Build()
			err := cl.Create(ctx, task)
			g.Expect(err).ToNot(HaveOccurred())

			reconciler := newTestReconciler(t, cl)
			taskObjKey := client.ObjectKeyFromObject(task)

			result := reconciler.handleSuccess(ctx, reconciler.logger, task, tc.result, tc.phase)

			if tc.expectContinue {
				g.Expect(result.NeedsRequeue()).To(BeFalse())
			} else if tc.expectTaskCompleted {
				g.Expect(result.NeedsRequeue()).To(BeTrue())
				if tc.phase == druidv1alpha1.OperationPhaseRunning {
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
			g.Expect(updatedTask.Status.LastOperation.Type).To(Equal(tc.expectedOpType))
			g.Expect(updatedTask.Status.LastOperation.State).To(Equal(tc.expectedOpState))
			g.Expect(updatedTask.Status.LastOperation.Description).To(Equal(tc.result.Description))

			if tc.expectedTaskState != nil {
				g.Expect(updatedTask.Status.State).ToNot(BeNil())
				g.Expect(*updatedTask.Status.State).To(Equal(*tc.expectedTaskState))
			}
		})
	}
}
