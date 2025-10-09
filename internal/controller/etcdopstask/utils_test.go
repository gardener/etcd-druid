// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
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

// TestRecordLastOperation tests the recordLastOperation function.
func TestRecordLastOperation(t *testing.T) {
	tests := []struct {
		name          string
		task          *druidv1alpha1.EtcdOpsTask
		initialLastOp *druidv1alpha1.LastOperation
		opType        druidv1alpha1.LastOperationType
		state         druidv1alpha1.LastOperationState
	}{
		{
			name:          "Previous LastOperation is nil",
			task:          newTestTask(nil),
			initialLastOp: nil,
			opType:        druidv1alpha1.OperationTypeAdmit,
			state:         druidv1alpha1.OperationStateInProgress,
		},
		{
			name: "Phase changed, state unchanged",
			task: newTestTask(nil),
			initialLastOp: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.OperationTypeAdmit,
				State: druidv1alpha1.OperationStateInProgress,
			},
			opType: druidv1alpha1.OperationTypeRunning,
			state:  druidv1alpha1.OperationStateInProgress,
		},
		{
			name: "State changed, phase unchanged",
			task: newTestTask(nil),
			initialLastOp: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.OperationTypeRunning,
				State: druidv1alpha1.OperationStateInProgress,
			},
			opType: druidv1alpha1.OperationTypeRunning,
			state:  druidv1alpha1.OperationStateCompleted,
		},
		{
			name: "Both phase and state changed",
			task: newTestTask(nil),
			initialLastOp: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.OperationTypeRunning,
				State: druidv1alpha1.OperationStateCompleted,
			},
			opType: druidv1alpha1.OperationTypeCleanup,
			state:  druidv1alpha1.OperationStateFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			cl := setupFakeClient(tt.task, true)
			err := cl.Create(context.TODO(), tt.task)
			g.Expect(err).NotTo(HaveOccurred())
			r := newTestReconciler(t, cl)
			taskKey := client.ObjectKey{
				Name:      tt.task.Name,
				Namespace: tt.task.Namespace,
			}
			if tt.initialLastOp != nil {
				tt.task.Status.LastOperation = tt.initialLastOp
				err = cl.Status().Update(context.TODO(), tt.task)
				g.Expect(err).NotTo(HaveOccurred())
			}
			err = r.recordLastOperation(context.TODO(), taskKey, tt.opType, tt.state, "")
			g.Expect(err).NotTo(HaveOccurred())
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(context.TODO(), taskKey, updatedTask)
			g.Expect(err).NotTo(HaveOccurred())

			expectedLastOperation := &druidv1alpha1.LastOperation{
				Type:  tt.opType,
				State: tt.state,
			}
			checkLastOperation(g, updatedTask, expectedLastOperation)

		})
	}
}

// TestRecordTaskState tests the recordTaskState function.
func TestRecordTaskState(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name         string
		task         *druidv1alpha1.EtcdOpsTask
		initialState *druidv1alpha1.TaskState
		state        druidv1alpha1.TaskState
	}{
		{
			name:         "Initial state is nil",
			task:         newTestTask(nil),
			initialState: nil,
			state:        druidv1alpha1.TaskStatePending,
		},
		{
			name:         "No overall state change",
			task:         newTestTask(nil),
			initialState: ptr.To(druidv1alpha1.TaskStateInProgress),
			state:        druidv1alpha1.TaskStateInProgress,
		},
		{
			name:         "State changed from InProgress to Completed",
			task:         newTestTask(nil),
			initialState: ptr.To(druidv1alpha1.TaskStateInProgress),
			state:        druidv1alpha1.TaskStateSucceeded,
		},
		{
			name:         "InitiatedAt is set when transitioning to InProgress",
			task:         newTestTask(nil),
			initialState: ptr.To(druidv1alpha1.TaskStatePending),
			state:        druidv1alpha1.TaskStateInProgress,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl := setupFakeClient(tc.task, true)
			err := cl.Create(context.TODO(), tc.task)
			g.Expect(err).NotTo(HaveOccurred())
			r := newTestReconciler(t, cl)

			taskKey := client.ObjectKey{
				Name:      tc.task.Name,
				Namespace: tc.task.Namespace,
			}
			if tc.initialState != nil {
				tc.task.Status.State = tc.initialState
				err = cl.Status().Update(context.TODO(), tc.task)
				g.Expect(err).NotTo(HaveOccurred())
			}
			err = r.recordTaskState(context.TODO(), taskKey, tc.state)
			g.Expect(err).NotTo(HaveOccurred())
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(context.TODO(), taskKey, updatedTask)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updatedTask.Status.State).NotTo(BeNil())
			g.Expect(*updatedTask.Status.State).To(Equal(tc.state))

			// when transitioning to InProgress, check if InitiatedAt is set
			if tc.state == druidv1alpha1.TaskStateInProgress && *tc.initialState == druidv1alpha1.TaskStatePending {
				g.Expect(updatedTask.Status.StartedAt).To(Not(BeNil()))
			}
		})
	}
}

// TestRecordLastError tests the recordLastError function.
func TestRecordLastError(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name             string
		task             *druidv1alpha1.EtcdOpsTask
		initialErrorSize int
		error            error
	}{
		{
			name:             "No initial last error",
			task:             newTestTask(nil),
			initialErrorSize: 0,
			error:            druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error"),
		},
		{
			name:             "Initial error exists, but total size is less than 3",
			task:             newTestTask(nil),
			initialErrorSize: 1,
			error:            druiderr.WrapError(fmt.Errorf("another test error"), "AnotherTestError", "TestOperation", "This is another test error"),
		},
		{
			name:             "Initial error exists, total size equals max limit (3)",
			task:             newTestTask(nil),
			initialErrorSize: 2,
			error:            druiderr.WrapError(fmt.Errorf("yet another test error"), "YetAnotherTestError", "TestOperation", "This is yet another test error"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl := setupFakeClient(tc.task, true)
			err := cl.Create(context.TODO(), tc.task)
			g.Expect(err).NotTo(HaveOccurred(), "Failed to create test task")
			r := newTestReconciler(t, cl)

			taskKey := client.ObjectKey{
				Name:      tc.task.Name,
				Namespace: tc.task.Namespace,
			}
			if tc.initialErrorSize > 0 {
				for i := range tc.initialErrorSize {
					tc.task.Status.LastErrors = append(tc.task.Status.LastErrors, druidv1alpha1.LastError{
						Code:        "InitialError",
						Description: fmt.Sprintf("initial error %d", i),
					})
				}
				err = cl.Status().Update(context.TODO(), tc.task)
				g.Expect(err).NotTo(HaveOccurred())
			}
			err = r.recordLastError(context.TODO(), taskKey, tc.error)
			g.Expect(err).NotTo(HaveOccurred())
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(context.TODO(), taskKey, updatedTask)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updatedTask.Status.LastErrors).NotTo(BeEmpty())
			g.Expect(len(updatedTask.Status.LastErrors)).To(BeNumerically("<=", 3))
			expectedLastErrors := &druidv1alpha1.LastError{
				Code: tc.error.(*druiderr.DruidError).Code,
			}
			checkLastErrors(g, updatedTask, expectedLastErrors)
			if tc.initialErrorSize >= 2 {
				size := len(updatedTask.Status.LastErrors)
				// size should be 3 (max limit)
				g.Expect(size).To(Equal(3))
			}
		})
	}
}
