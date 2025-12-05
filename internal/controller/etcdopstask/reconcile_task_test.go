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

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
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
		patchErr          error
		expectedResult    ctrlutils.ReconcileStepResult
	}{
		{
			name:              "Should skip adding finalizer when it already exists",
			task:              testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			ContainsFinalizer: true,
			expectedResult:    ctrlutils.ContinueReconcile(),
		},
		{
			name:              "Should add finalizer when it does not exist",
			task:              testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			ContainsFinalizer: false,
			expectedResult:    ctrlutils.ContinueReconcile(),
		},
		{
			name: "Should return error when task does not exist",
			task: nil,
		},
		{
			name:     "Should return error when patch fails",
			task:     testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			patchErr: apierrors.NewInternalError(fmt.Errorf("patch failed")),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				cl  client.Client
				err error
			)
			ctx := context.Background()
			if tc.task != nil {
				if tc.patchErr != nil {
					cl = testutils.NewTestClientBuilder().
						WithScheme(kubernetes.Scheme).
						WithObjects(tc.task).
						RecordErrorForObjects(testutils.ClientMethodPatch, tc.patchErr.(*apierrors.StatusError), client.ObjectKeyFromObject(tc.task)).
						Build()
				} else {
					cl = testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
					err = cl.Create(ctx, tc.task)
					g.Expect(err).ToNot(HaveOccurred(), "Failed to create task")
					if tc.ContainsFinalizer {
						controllerutil.AddFinalizer(tc.task, druidapicommon.EtcdOpsTaskFinalizerName)
						err = cl.Update(ctx, tc.task)
						g.Expect(err).ToNot(HaveOccurred(), "Failed to update task with finalizer")
					}
				}
			} else {
				cl = testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
			}

			r := newTestReconciler(t, cl)

			taskKey := client.ObjectKey{Name: "test-task", Namespace: "test-ns"}
			if tc.task != nil {
				taskKey = client.ObjectKeyFromObject(tc.task)
			}
			result := r.ensureTaskFinalizer(ctx, r.logger, taskKey, nil)

			if tc.patchErr != nil {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(errors.Is(result.GetCombinedError(), tc.patchErr)).To(BeTrue())
				return
			}

			if tc.task == nil {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError()).To(HaveOccurred())
				return
			}

			g.Expect(result).To(Equal(tc.expectedResult))
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(ctx, taskKey, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(controllerutil.ContainsFinalizer(updatedTask, druidapicommon.EtcdOpsTaskFinalizerName)).To(BeTrue())
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
			name:           "Should skip task state update when current task state is not nil",
			task:           testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStatePending).Build(),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:           "Should transition task state to InProgress when current task state is Pending",
			task:           testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			cl := testutils.NewTestClientBuilder().
				WithScheme(kubernetes.Scheme).
				WithStatusSubresource(tc.task).
				Build()
			r := newTestReconciler(t, cl)
			err := cl.Create(ctx, tc.task)
			g.Expect(err).To(BeNil())

			result := r.transitionToPendingState(ctx, r.logger, client.ObjectKeyFromObject(tc.task), nil)
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(ctx, client.ObjectKeyFromObject(tc.task), updatedTask)
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
	testErr := fmt.Errorf("test error")

	tests := []struct {
		name                  string
		task                  *druidv1alpha1.EtcdOpsTask
		additionalTaskPresent bool
		admitFailed           bool
		resultRequeue         bool
		expectedResult        ctrlutils.ReconcileStepResult
		expectedLastErrors    *druidapicommon.LastError
		expectedLastOperation *druidapicommon.LastOperation
	}{
		{
			name:        "Should return error when task does not exist",
			task:        nil,
			admitFailed: false,
		},
		{
			name:                  "Should reject task when another task is already in progress",
			additionalTaskPresent: true,
			task:                  testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithEtcdName("test-etcd").WithState(druidv1alpha1.TaskStatePending).Build(),
			expectedLastErrors: &druidapicommon.LastError{
				Code: taskhandler.ErrDuplicateTask,
			},
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeAdmit,
				State: druidv1alpha1.LastOperationStateFailed,
			},
			expectedResult: ctrlutils.ReconcileAfter(3600*time.Second, "Task rejected, waiting for TTL to expire before deletion"),
		},
		{
			name:           "Should skip task admit when current task state is InProgress",
			task:           testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build(),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:          "Should requeue admit when admit fails with temporary error",
			task:          testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStatePending).Build(),
			admitFailed:   true,
			resultRequeue: true,
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeAdmit,
				State: druidv1alpha1.LastOperationStateInProgress,
			},
			expectedLastErrors: &druidapicommon.LastError{
				Code: druidapicommon.ErrorCode("TestError"),
			},
			expectedResult: ctrlutils.ReconcileWithError(druiderr.WrapError(testErr, "TestError", "TestOperation", "This is a test error")),
		},
		{
			name:          "Should mark LastOperation state as completed when admit succeeds",
			task:          testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStatePending).Build(),
			resultRequeue: false,
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeAdmit,
				State: druidv1alpha1.LastOperationStateCompleted,
			},
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:          "Should mark task as rejected when admit fails with non-temporary error",
			task:          testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStatePending).Build(),
			admitFailed:   true,
			resultRequeue: false,
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeAdmit,
				State: druidv1alpha1.LastOperationStateFailed,
			},
			expectedLastErrors: &druidapicommon.LastError{
				Code: druidapicommon.ErrorCode("TestError"),
			},
			expectedResult: ctrlutils.ReconcileAfter(3600*time.Second, "Task rejected, waiting for TTL to expire"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			var cl client.Client
			var existingTasks []client.Object
			if tc.additionalTaskPresent {
				existingTask := testutils.EtcdOpsTaskBuilderWithDefaults("test-task-1", "test-ns").WithEtcdName("test-etcd").WithState(druidv1alpha1.TaskStateInProgress).Build()
				existingTasks = append(existingTasks, existingTask)
			}

			if tc.task != nil {
				existingTasks = append(existingTasks, tc.task)
			}
			if len(existingTasks) > 0 {
				cl = testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(existingTasks...).WithStatusSubresource(tc.task).Build()
			} else {
				cl = testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
			}

			reconciler := newTestReconciler(t, cl)
			fakeHandler := testutils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)

			if tc.admitFailed {
				fakeHandler.WithAdmit(taskhandler.Result{
					Requeue: tc.resultRequeue,
					Error:   druiderr.WrapError(testErr, "TestError", "TestOperation", "This is a test error"),
				})
			} else {
				fakeHandler.WithAdmit(taskhandler.Result{
					Requeue: tc.resultRequeue,
				})
			}

			result := reconciler.admitTask(ctx, reconciler.logger, client.ObjectKey{Name: "test-task", Namespace: "test-ns"}, fakeHandler)

			if tc.task == nil {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError()).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(result.GetCombinedError())).To(BeTrue())
				return
			}

			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err := cl.Get(ctx, client.ObjectKey{Name: "test-task", Namespace: "test-ns"}, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())
			if tc.expectedResult.HasErrors() {
				testutils.CheckDruidErrorList(g, result.GetErrors(), tc.expectedResult.GetErrors())
			} else {
				g.Expect(result.GetErrors()).To(HaveLen(0))
			}
			testutils.CheckLastOperation(g, updatedTask.Status.LastOperation, tc.expectedLastOperation)
			testutils.CheckLastErrors(g, updatedTask.Status.LastErrors, tc.expectedLastErrors)
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
			name:           "Should skip task state update when current task state is InProgress",
			task:           testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build(),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:           "Should skip task state update when current task state is not Pending",
			task:           testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:           "Should transition task state to InProgress when current task state is Pending",
			task:           testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStatePending).Build(),
			expectedResult: ctrlutils.ContinueReconcile(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			cl := testutils.NewTestClientBuilder().
				WithScheme(kubernetes.Scheme).
				WithStatusSubresource(tc.task).
				Build()
			r := newTestReconciler(t, cl)
			err := cl.Create(ctx, tc.task)
			g.Expect(err).To(BeNil())

			handler := testutils.NewFakeEtcdOpsTaskHandler("test-handler", client.ObjectKeyFromObject(tc.task), r.logger)
			result := r.transitionToInProgressState(ctx, r.logger, client.ObjectKeyFromObject(tc.task), handler)
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(ctx, client.ObjectKeyFromObject(tc.task), updatedTask)
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

// TestExecuteTask tests the runTask step function.
func TestExecuteTask(t *testing.T) {
	g := NewGomegaWithT(t)
	testErr := fmt.Errorf("test error")
	tests := []struct {
		name                  string
		task                  *druidv1alpha1.EtcdOpsTask
		resultRequeued        bool
		runFailed             bool
		isTTLExpired          bool
		expectedResult        ctrlutils.ReconcileStepResult
		expectedLastErrors    *druidapicommon.LastError
		expectedLastOperation *druidapicommon.LastOperation
		expectedTaskState     *druidv1alpha1.TaskState
	}{
		{
			name:      "Should return error when task does not exist",
			task:      nil,
			runFailed: false,
		},
		{
			name:           "Should update task state as Failed when run fails with non-temporary error",
			task:           testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build(),
			resultRequeued: false,
			runFailed:      true,
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeExecution,
				State: druidv1alpha1.LastOperationStateFailed,
			},
			expectedLastErrors: &druidapicommon.LastError{
				Code: druidapicommon.ErrorCode("TestError"),
			},
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateFailed),
			expectedResult:    ctrlutils.ReconcileAfter(3600*time.Second, "Task failed, waiting for TTL to expire"),
		},
		{
			name:           "Should mark task state as Succeeded when run completes successfully",
			task:           testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build(),
			resultRequeued: false,
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeExecution,
				State: druidv1alpha1.LastOperationStateCompleted,
			},
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateSucceeded),
			expectedResult:    ctrlutils.ReconcileAfter(3600*time.Second, "Task succeeded, waiting for TTL to expire"),
		},
		{
			name:           "Should requeue with error when run fails with temporary error",
			task:           testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build(),
			resultRequeued: true,
			runFailed:      true,
			expectedLastErrors: &druidapicommon.LastError{
				Code: druidapicommon.ErrorCode("TestError"),
			},
			expectedResult: ctrlutils.ReconcileWithError(druiderr.WrapError(testErr, "TestError", "TestOperation", "This is a test error")),
		},
		{
			name:              "Should continue requeueing when task is still in progress",
			task:              testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateInProgress).Build(),
			resultRequeued:    true,
			expectedTaskState: ptr.To(druidv1alpha1.TaskStateInProgress),
			expectedResult:    ctrlutils.ReconcileAfter(60*time.Second, "Task execution in progress"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			var cl client.Client

			if tc.task != nil && tc.isTTLExpired {
				tc.task.Spec.TTLSecondsAfterFinished = ptr.To(int32(0))
			}

			if tc.task != nil {
				cl = testutils.NewTestClientBuilder().
					WithScheme(kubernetes.Scheme).
					WithStatusSubresource(tc.task).
					Build()
				err := cl.Create(ctx, tc.task)
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				cl = testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
			}

			reconciler := newTestReconciler(t, cl)
			fakeHandler := testutils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)

			if tc.runFailed {
				fakeHandler.WithExecute(taskhandler.Result{
					Requeue: tc.resultRequeued,
					Error:   druiderr.WrapError(testErr, "TestError", "TestOperation", "This is a test error"),
				})
			} else {
				fakeHandler.WithExecute(taskhandler.Result{
					Requeue: tc.resultRequeued,
				})
			}

			result := reconciler.executeTask(ctx, reconciler.logger, client.ObjectKey{Name: "test-task", Namespace: "test-ns"}, fakeHandler)
			if tc.task == nil {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError()).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(result.GetCombinedError())).To(BeTrue())
				return
			}
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err := cl.Get(ctx, client.ObjectKey{Name: "test-task", Namespace: "test-ns"}, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())
			if tc.expectedResult.HasErrors() {
				testutils.CheckDruidErrorList(g, result.GetErrors(), tc.expectedResult.GetErrors())
			} else {
				g.Expect(result.GetErrors()).To(HaveLen(0))
			}
			g.Expect(result.GetDescription()).To(Equal(tc.expectedResult.GetDescription()))
			g.Expect(result.NeedsRequeue()).To(Equal(tc.expectedResult.NeedsRequeue()))

			if tc.expectedTaskState != nil {
				g.Expect(updatedTask.Status.State).ToNot(BeNil())
				g.Expect(*updatedTask.Status.State).To(Equal(*tc.expectedTaskState))
			}
			testutils.CheckLastOperation(g, updatedTask.Status.LastOperation, tc.expectedLastOperation)
			testutils.CheckLastErrors(g, updatedTask.Status.LastErrors, tc.expectedLastErrors)
		})
	}
}
