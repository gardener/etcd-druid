// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	testutils "github.com/gardener/etcd-druid/test/utils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/gomega"
)

// TestEnsureFinalizer tests the ensureTaskFinalizer step function
func TestEnsureFinalizer(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name              string
		task              *druidv1alpha1.EtcdOpsTask
		ContainsFinalizer bool
		patchFails        bool
		expectError       bool
		errSubstring      string
		expectedResult    ctrlutils.ReconcileStepResult
	}{
		{
			name:              "Finalizer already exists",
			task:              newTestTask(nil),
			ContainsFinalizer: true,
			expectedResult:    ctrlutils.ContinueReconcile(),
		},
		{
			name:              "Finalizer does not exist, add finalizer",
			task:              newTestTask(nil),
			ContainsFinalizer: false,
			expectedResult:    ctrlutils.ContinueReconcile(),
		},
		{
			name:         "Task not found",
			task:         nil,
			expectError:  true,
			errSubstring: "not found",
		},
		{
			name:         "Patch fails",
			task:         newTestTask(nil),
			patchFails:   true,
			errSubstring: "patch failed",
			expectError:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				cl  client.Client
				err error
			)
			if tc.task != nil {
				if tc.patchFails {
					patchErr := apierrors.NewInternalError(fmt.Errorf("patch failed"))
					cl = testutils.NewTestClientBuilder().
						WithScheme(kubernetes.Scheme).
						WithObjects(tc.task).
						RecordErrorForObjects(testutils.ClientMethodPatch, patchErr, client.ObjectKeyFromObject(tc.task)).
						Build()
				} else {
					cl = setupFakeClient(tc.task, false)
					err = cl.Create(context.TODO(), tc.task)
					g.Expect(err).ToNot(HaveOccurred(), "Failed to create task")
					if tc.ContainsFinalizer {
						controllerutil.AddFinalizer(tc.task, FinalizerName)
						err = cl.Update(context.TODO(), tc.task)
						g.Expect(err).ToNot(HaveOccurred(), "Failed to update task with finalizer")
					}
				}
			} else {
				cl = setupFakeClient(nil, false)
			}

			r := newTestReconciler(t, cl)

			taskKey := client.ObjectKey{Name: "test-task", Namespace: "test-ns"}
			if tc.task != nil {
				taskKey = client.ObjectKeyFromObject(tc.task)
			}
			result := r.ensureTaskFinalizer(context.TODO(), r.logger, taskKey, nil)

			if tc.expectError {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError()).To(HaveOccurred())
				g.Expect(result.GetCombinedError().Error()).To(ContainSubstring(tc.errSubstring))
				return
			}

			g.Expect(result).To(Equal(tc.expectedResult))
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(context.TODO(), taskKey, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(controllerutil.ContainsFinalizer(updatedTask, FinalizerName)).To(BeTrue())
		})
	}
}

// TestTransitionToPendingState tests the transitionToPendingState step function.
func TestTransitionToPendingState(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		task           *druidv1alpha1.EtcdOpsTask
		expectedResult ctrlutils.ReconcileStepResult
	}{
		{
			name:           "Task state is not nil, i.e either Pending or InProgress",
			task:           newTestTask(ptr.To(druidv1alpha1.TaskStatePending)),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:           "Task state is nil, should transition to Pending",
			task:           newTestTask(nil),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cl := setupFakeClient(tc.task, true)
			r := newTestReconciler(t, cl)
			err := cl.Create(context.TODO(), tc.task)
			g.Expect(err).To(BeNil())

			result := r.transitionToPendingState(context.TODO(), r.logger, client.ObjectKeyFromObject(tc.task), nil)
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(context.TODO(), client.ObjectKeyFromObject(tc.task), updatedTask)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(result).To(Equal(tc.expectedResult))

			if tc.task.Status.State == nil {
				g.Expect(updatedTask.Status.State).ToNot(BeNil())
				g.Expect(*updatedTask.Status.State).To(Equal(druidv1alpha1.TaskStatePending))
			} else {
				g.Expect(updatedTask.Status.State).ToNot(BeNil())
				g.Expect(*updatedTask.Status.State).To(Equal(*tc.task.Status.State))
			}
		})
	}
}

// TestAdmitTask tests the admitTask step function.
func TestAdmitTask(t *testing.T) {
	g := NewGomegaWithT(t)

	tests := []struct {
		name                  string
		task                  *druidv1alpha1.EtcdOpsTask
		additionalTaskPresent bool
		admitFailed           bool
		resultCompleted       bool
		expectedResult        ctrlutils.ReconcileStepResult
		expectedLastErrors    *druidv1alpha1.LastError
		expectedLastOperation *druidv1alpha1.LastOperation
	}{
		{
			name:        "Task does not exist",
			task:        nil,
			admitFailed: false,
		},
		{
			name:                  "Duplicate task present",
			additionalTaskPresent: true,
			task:                  newTestTask(ptr.To(druidv1alpha1.TaskStatePending)),
			expectedLastErrors: &druidv1alpha1.LastError{
				Code:        ErrDuplicateTask,
				Description: "Operation: AdmitOperation, Code: ErrDuplicateTask message: duplicate EtcdOpsTask for the same etcd is already in progress, cause: duplicate EtcdOpsTask for etcd  is already in progress (task: test-task-1)",
			},
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.OperationTypeAdmit,
				State: druidv1alpha1.OperationStateFailed,
			},
			expectedResult: ctrlutils.ReconcileAfter(1*time.Second, "Duplicate task found, handing to deletion flow"),
		},
		{
			name:           "Task state is not Pending",
			task:           newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress)),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:        "result.error is not nil, result.Completed is false",
			task:        newTestTask(ptr.To(druidv1alpha1.TaskStatePending)),
			admitFailed: true,
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.OperationTypeAdmit,
				State: druidv1alpha1.OperationStateInProgress,
			},
			expectedLastErrors: &druidv1alpha1.LastError{
				Code:        druidv1alpha1.ErrorCode("TestError"),
				Description: "This is a Test Error",
			},
			expectedResult: ctrlutils.ReconcileWithError(fmt.Errorf("admit returned error: This is a Test Error")),
		},
		{
			name:            "result.Completed is true, result.Error is nil",
			task:            newTestTask(ptr.To(druidv1alpha1.TaskStatePending)),
			resultCompleted: true,
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.OperationTypeAdmit,
				State: druidv1alpha1.OperationStateInProgress,
			},
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:            "result.completed is true, result.Error is not nil",
			task:            newTestTask(ptr.To(druidv1alpha1.TaskStatePending)),
			admitFailed:     true,
			resultCompleted: true,
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.OperationTypeAdmit,
				State: druidv1alpha1.OperationStateFailed,
			},
			expectedLastErrors: &druidv1alpha1.LastError{
				Code:        druidv1alpha1.ErrorCode("TestError"),
				Description: "This is a Test Error",
			},
			expectedResult: ctrlutils.ReconcileAfter(1*time.Second, "Task rejected, handing to deletion flow"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cl client.Client
			var objects []client.Object
			if tc.additionalTaskPresent {
				existingTask := newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress))
				existingTask.Name = "test-task-1"
				objects = append(objects, existingTask)
			}

			if tc.task != nil {
				objects = append(objects, tc.task)
			}
			if len(objects) > 0 {
				cl = testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(objects...).WithStatusSubresource(tc.task).Build()
			} else {
				cl = setupFakeClient(nil, false)
			}

			reconciler := newTestReconciler(t, cl)
			fakeHandler := testutils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)

			if tc.admitFailed {
				fakeHandler.WithAdmit(handler.Result{
					Completed: tc.resultCompleted,
					Error:     druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error"),
				})
			} else {
				fakeHandler.WithAdmit(handler.Result{
					Completed: tc.resultCompleted,
				})
			}

			result := reconciler.admitTask(context.TODO(), reconciler.logger, client.ObjectKey{Name: "test-task", Namespace: "test-ns"}, fakeHandler)

			if tc.task == nil {
				checksForNilTask(g, result)
				return
			}

			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err := cl.Get(context.TODO(), client.ObjectKey{Name: "test-task", Namespace: "test-ns"}, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())
			expectDruidErrors(g, result.GetErrors(), tc.expectedResult.GetErrors())
			checkLastOperation(g, updatedTask, tc.expectedLastOperation)
			checkLastErrors(g, updatedTask, tc.expectedLastErrors)
		})
	}
}

// TestTransitionToInProgressState tests the transitionToInProgressState step function.
func TestTransitionToInProgressState(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		task           *druidv1alpha1.EtcdOpsTask
		expectedResult ctrlutils.ReconcileStepResult
	}{
		{
			name:           "Task state is not 'Pending', skipping state updation",
			task:           newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress)),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:           "Task state is nil, skipping state updation",
			task:           newTestTask(nil),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:           "Task state is 'Pending'. Update to 'InProgress'",
			task:           newTestTask(ptr.To(druidv1alpha1.TaskStatePending)),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cl := setupFakeClient(tc.task, true)
			r := newTestReconciler(t, cl)
			err := cl.Create(context.TODO(), tc.task)
			g.Expect(err).To(BeNil())

			handler := testutils.NewFakeEtcdOpsTaskHandler("test-handler", client.ObjectKeyFromObject(tc.task), r.logger)
			result := r.transitionToInProgressState(context.TODO(), r.logger, client.ObjectKeyFromObject(tc.task), handler)
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(context.TODO(), client.ObjectKeyFromObject(tc.task), updatedTask)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(result).To(Equal(tc.expectedResult))

			if tc.task.Status.State == nil {
				g.Expect(updatedTask.Status.State).To(BeNil())
			} else if *tc.task.Status.State != druidv1alpha1.TaskStatePending {
				g.Expect(*updatedTask.Status.State).To(Equal(*tc.task.Status.State))
			} else {
				g.Expect(updatedTask.Status.State).ToNot(BeNil())
				g.Expect(*updatedTask.Status.State).To(Equal(druidv1alpha1.TaskStateInProgress))
			}
		})
	}
}

// TestRunTask tests the runTask step function.
func TestRunTask(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name                  string
		task                  *druidv1alpha1.EtcdOpsTask
		resultCompleted       bool
		runFailed             bool
		isTTLExpired          bool
		isResultToBeRequeued  bool
		expectedResult        ctrlutils.ReconcileStepResult
		expectedLastErrors    *druidv1alpha1.LastError
		expectedLastOperation *druidv1alpha1.LastOperation
		expectedTaskState     *druidv1alpha1.TaskState
	}{
		{
			name:      "Task does not exist",
			task:      nil,
			runFailed: false,
		},
		{
			name:            "Result completed is true, result error is not nil",
			task:            newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress)),
			resultCompleted: true,
			runFailed:       true,
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.OperationTypeRunning,
				State: druidv1alpha1.OperationStateFailed,
			},
			expectedLastErrors: &druidv1alpha1.LastError{
				Code:        druidv1alpha1.ErrorCode("TestError"),
				Description: "This is a Test Error",
			},
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateFailed),
			expectedResult:    ctrlutils.ReconcileAfter(1*time.Second, "Task completed"),
		},
		{
			name:            "Result completed is true, result error is nil",
			task:            newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress)),
			resultCompleted: true,
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.OperationTypeRunning,
				State: druidv1alpha1.OperationStateCompleted,
			},
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateSucceeded),
			expectedResult:    ctrlutils.ReconcileAfter(1*time.Second, "Task completed"),
		},
		{
			name:                 "Result completed is false, result error is not nil",
			task:                 newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress)),
			resultCompleted:      false,
			runFailed:            true,
			isResultToBeRequeued: true,
			expectedLastErrors: &druidv1alpha1.LastError{
				Code:        druidv1alpha1.ErrorCode("TestError"),
				Description: "This is a Test Error",
			},
			expectedResult: ctrlutils.ReconcileAfter(60*time.Second, "Task in progress"),
		},
		{
			name:                 "Result completed is false, result error is nil",
			task:                 newTestTask(ptr.To(druidv1alpha1.TaskStateInProgress)),
			resultCompleted:      false,
			isResultToBeRequeued: true,
			expectedTaskState:    ptr.To(druidv1alpha1.TaskStateInProgress),
			expectedResult:       ctrlutils.ReconcileAfter(60*time.Second, "Task in progress"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cl client.Client

			if tc.task != nil && tc.isTTLExpired {
				tc.task.Spec.TTLSecondsAfterFinished = ptr.To(int32(0))
			}

			if tc.task != nil {
				cl = setupFakeClient(tc.task, true)
				err := cl.Create(context.TODO(), tc.task)
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				cl = setupFakeClient(nil, false)
			}

			reconciler := newTestReconciler(t, cl)
			fakeHandler := testutils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)

			if tc.isResultToBeRequeued {
				if tc.runFailed {
					fakeHandler.WithRun(handler.Result{
						Completed: tc.resultCompleted,
						Error:     druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error"),
					})
				} else {
					fakeHandler.WithRun(handler.Result{
						Completed: tc.resultCompleted,
					})
				}
			} else {
				if tc.runFailed {
					fakeHandler.WithRun(handler.Result{
						Completed: tc.resultCompleted,
						Error:     druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error"),
					})
				} else {
					fakeHandler.WithRun(handler.Result{
						Completed: tc.resultCompleted,
					})
				}
			}

			result := reconciler.runTask(context.TODO(), reconciler.logger, client.ObjectKey{Name: "test-task", Namespace: "test-ns"}, fakeHandler)
			if tc.task == nil {
				checksForNilTask(g, result)
				return
			}
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err := cl.Get(context.TODO(), client.ObjectKey{Name: "test-task", Namespace: "test-ns"}, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())
			expectDruidErrors(g, result.GetErrors(), tc.expectedResult.GetErrors())
			g.Expect(result.GetDescription()).To(Equal(tc.expectedResult.GetDescription()))
			g.Expect(result.NeedsRequeue()).To(Equal(tc.expectedResult.NeedsRequeue()))

			if tc.expectedTaskState != nil {
				g.Expect(updatedTask.Status.State).ToNot(BeNil())
				g.Expect(*updatedTask.Status.State).To(Equal(*tc.expectedTaskState))
			}
			checkLastOperation(g, updatedTask, tc.expectedLastOperation)
			checkLastErrors(g, updatedTask, tc.expectedLastErrors)
		})
	}
}
