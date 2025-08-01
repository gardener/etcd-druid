package etcdopstask

import (
	"context"
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/task"
	"github.com/gardener/etcd-druid/test/utils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// TestCleanupTaskResources tests the function testCleanupTaskResources
func TestCleanupTaskResources(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name                  string
		task                  *druidv1alpha1.EtcdOpsTask
		expectedResult        ctrlutils.ReconcileStepResult
		cleanupFailed         bool
		resultCompleted       bool
		nilResult             bool
		expectedLastOperation *druidv1alpha1.EtcdOpsLastOperation
		expectedLastErrors    *druidv1alpha1.EtcdOpsTaskLastError
	}{
		{
			name:          "Task not found",
			task:          nil,
			cleanupFailed: false,
		},
		{
			name:           "Task is in rejected state, last operation is updated",
			task:           newTestTask(ptr.To(druidv1alpha1.TaskStateRejected)),
			cleanupFailed:  false,
			expectedResult: ctrlutils.ContinueReconcile(),
			expectedLastOperation: &druidv1alpha1.EtcdOpsLastOperation{
				Phase: druidv1alpha1.OperationPhaseCleanup,
				State: druidv1alpha1.OperationStateCompleted,
			},
		},
		{
			name:           "result.Completed is true, no error, last operation is updated",
			task:           newTestTask(nil),
			cleanupFailed:  false,
			expectedResult: ctrlutils.ContinueReconcile(),
			expectedLastOperation: &druidv1alpha1.EtcdOpsLastOperation{
				Phase: druidv1alpha1.OperationPhaseCleanup,
				State: druidv1alpha1.OperationStateCompleted,
			},
		},
		{
			name:            "result.Completed is true, error, last operation and last Error is updated",
			task:            newTestTask(nil),
			expectedResult:  ctrlutils.ReconcileWithError(druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error")),
			cleanupFailed:   true,
			resultCompleted: true,
			expectedLastOperation: &druidv1alpha1.EtcdOpsLastOperation{
				Phase: druidv1alpha1.OperationPhaseCleanup,
				State: druidv1alpha1.OperationStateFailed,
			},
			expectedLastErrors: &druidv1alpha1.EtcdOpsTaskLastError{
				Code:        druidv1alpha1.ErrorCode("TestError"),
				Description: "This is a test error",
			},
		},
		{
			name:           "result.Completed is false, no error, last operation is not updated",
			task:           newTestTask(nil),
			cleanupFailed:  false,
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name:           "result.Completed is false, error, last Error is updated",
			task:           newTestTask(nil),
			expectedResult: ctrlutils.ContinueReconcile(),
			cleanupFailed:  true,
			expectedLastErrors: &druidv1alpha1.EtcdOpsTaskLastError{
				Code:        druidv1alpha1.ErrorCode("TestError"),
				Description: "This is a test error",
			},
		},
		{
			name:      "Cleanup returns nil result",
			task:      newTestTask(nil),
			nilResult: true,
			expectedResult: ctrlutils.ReconcileWithError(
				druiderr.WrapError(
					fmt.Errorf("cleanup returned nil TaskResult; this is a bug in the TaskHandler implementation"),
					ErrNilResult,
					string(druidv1alpha1.OperationPhaseCleanup),
					"cleanup returned nil TaskResult; this is a bug in the TaskHandler implementation",
				),
			),
			expectedLastOperation: &druidv1alpha1.EtcdOpsLastOperation{
				Phase: druidv1alpha1.OperationPhaseCleanup,
				State: druidv1alpha1.OperationStateInProgress,
			},
			expectedLastErrors: &druidv1alpha1.EtcdOpsTaskLastError{
				Code:        ErrNilResult,
				Description: "Operation: Cleanup, Code: ErrNilResult message: cleanup returned nil TaskResult; this is a bug in the TaskHandler implementation, cause: cleanup returned nil TaskResult; this is a bug in the TaskHandler implementation",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cl client.Client
			if tc.task != nil {
				cl = setupFakeClient(tc.task, true)
				err := cl.Create(context.TODO(), tc.task)
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				cl = setupFakeClient(nil, false)
			}
			reconciler := newTestReconciler(t, cl)
			fakeHandler := utils.NewFakeHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)
			if tc.nilResult {
				fakeHandler.CleanupResult = nil
			} else if tc.cleanupFailed {
				fakeHandler.WithCleanup(&task.Result{
					Completed: tc.resultCompleted,
					Error:     druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error"),
				})
			} else {
				fakeHandler.WithCleanup(&task.Result{
					Completed: true,
				})
			}

			result := reconciler.cleanupTaskResources(context.TODO(), types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, fakeHandler)
			if tc.task == nil {
				checksForNilTask(g, result)
				return
			}
			expectDruidErrors(g, result.GetErrors(), tc.expectedResult.GetErrors())
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err := cl.Get(context.TODO(), types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())
			checkLastOperation(g, updatedTask, tc.expectedLastOperation)
			checkLastErrors(g, updatedTask, tc.expectedLastErrors)
		})

	}
}

// TestRemoveTaskFinalizer tests the function removeTaskFinalizer
func TestRemoveTaskFinalizer(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		task           *druidv1alpha1.EtcdOpsTask
		expectedResult ctrlutils.ReconcileStepResult
	}{
		{
			name: "Task not found",
			task: nil,
		},
		{
			name: "Task found with finalizer - successfully removed",
			task: &druidv1alpha1.EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-task",
					Namespace:  "test-ns",
					Finalizers: []string{FinalizerName, "other-finalizer"},
				},
			},
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name: "Task found without finalizer",
			task: &druidv1alpha1.EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "test-ns",
				},
			},
			expectedResult: ctrlutils.ContinueReconcile(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cl := setupFakeClient(nil, false)
			if tc.task != nil {
				err := cl.Create(context.TODO(), tc.task)
				g.Expect(err).ToNot(HaveOccurred())
			}

			reconciler := newTestReconciler(t, cl)
			fakeHandler := utils.NewFakeHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)

			result := reconciler.removeTaskFinalizer(context.TODO(), types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, fakeHandler)
			if tc.task == nil {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError()).To(HaveOccurred())
				g.Expect(result.GetCombinedError().Error()).To(ContainSubstring("not found"))
				return
			}
			g.Expect(result).To(Equal(tc.expectedResult))
		})
	}
}

// TestRemoveTask tests the function removeTask
func TestRemoveTask(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		task           *druidv1alpha1.EtcdOpsTask
		expectedResult ctrlutils.ReconcileStepResult
	}{
		{
			name: "Error getting the task",
			task: nil,
		},
		{
			name: "Successful deletion",
			task: &druidv1alpha1.EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "test-ns",
				},
			},
			expectedResult: ctrlutils.DoNotRequeue(),
		},
		{
			name: "Deletion fails",
			task: &druidv1alpha1.EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "test-ns",
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cl client.Client
			if tc.name == "Deletion fails" {
				statusErr := &apierrors.StatusError{ErrStatus: metav1.Status{Message: "delete failed"}}
				cl = utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(tc.task).RecordErrorForObjects(utils.ClientMethodDelete, statusErr, types.NamespacedName{Name: "test-task", Namespace: "test-ns"}).Build()
			} else {
				cl = setupFakeClient(nil, false)
				if tc.task != nil {
					err := cl.Create(context.TODO(), tc.task)
					g.Expect(err).ToNot(HaveOccurred(), "Failed to create test task")
				}
			}

			reconciler := newTestReconciler(t, cl)
			fakeHandler := utils.NewFakeHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)
			fakeHandler.WithCleanup(&task.Result{Completed: true})

			result := reconciler.removeTask(context.TODO(), types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, fakeHandler)
			if tc.task == nil {
				checksForNilTask(g, result)
				return
			}
			if tc.name == "Deletion fails" {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError()).To(HaveOccurred())
				g.Expect(result.GetCombinedError().Error()).To(ContainSubstring("delete failed"))
				return
			}
			g.Expect(result).To(Equal(tc.expectedResult))
		})
	}
}
