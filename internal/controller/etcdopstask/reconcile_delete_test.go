// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// TestCleanupTaskResources tests the cleanupTaskResources step function
func TestCleanupTaskResources(t *testing.T) {
	g := NewGomegaWithT(t)
	testErr := fmt.Errorf("test error")
	tests := []struct {
		name                  string
		task                  *druidv1alpha1.EtcdOpsTask
		expectedResult        ctrlutils.ReconcileStepResult
		cleanupFailed         bool
		resultRequeue         bool
		expectedLastOperation *druidapicommon.LastOperation
		expectedLastErrors    *druidapicommon.LastError
	}{
		{
			name:           "Should skip cleanup phase for rejected task",
			task:           utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").WithState(druidv1alpha1.TaskStateRejected).Build(),
			cleanupFailed:  false,
			expectedResult: ctrlutils.ContinueReconcile(),
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeCleanup,
				State:       druidv1alpha1.LastOperationStateCompleted,
				Description: "Cleanup skipped for rejected task",
			},
		},
		{
			name:           "Should continue reconcile when cleanup phase completes successfully",
			task:           utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			cleanupFailed:  false,
			expectedResult: ctrlutils.ContinueReconcile(),
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeCleanup,
				State: druidv1alpha1.LastOperationStateInProgress,
			},
		},
		{
			name:           "Should update task status when cleanup fails with non-temporary error",
			task:           utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			expectedResult: ctrlutils.ReconcileWithError(druiderr.WrapError(testErr, "TestError", "TestOperation", "This is a test error")),
			cleanupFailed:  true,
			resultRequeue:  false,
			expectedLastOperation: &druidapicommon.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeCleanup,
				State: druidv1alpha1.LastOperationStateFailed,
			},
			expectedLastErrors: &druidapicommon.LastError{
				Code:        druidapicommon.ErrorCode("TestError"),
				Description: "This is a test error",
			},
		},
		{
			name:           "Should requeue cleanup when cleanup is in progress",
			task:           utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			cleanupFailed:  false,
			resultRequeue:  true,
			expectedResult: ctrlutils.ReconcileAfter(60*time.Second, "Task cleanup in progress"),
		},
		{
			name:           "Should requeue with error when cleanup fails with temporary error",
			task:           utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-ns").Build(),
			expectedResult: ctrlutils.ReconcileWithError(druiderr.WrapError(testErr, "TestError", "TestOperation", "This is a test error")),
			cleanupFailed:  true,
			resultRequeue:  true,
			expectedLastErrors: &druidapicommon.LastError{
				Code:        druidapicommon.ErrorCode("TestError"),
				Description: "This is a test error",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			cl := utils.NewTestClientBuilder().
				WithScheme(kubernetes.Scheme).
				WithStatusSubresource(tc.task).
				Build()
			err := cl.Create(ctx, tc.task)
			g.Expect(err).ToNot(HaveOccurred())
			reconciler := newTestReconciler(t, cl)
			fakeHandler := utils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)
			if tc.cleanupFailed {
				fakeHandler.WithCleanup(handler.Result{
					Requeue: tc.resultRequeue,
					Error:   druiderr.WrapError(testErr, "TestError", "TestOperation", "This is a test error"),
				})
			} else {
				fakeHandler.WithCleanup(handler.Result{
					Requeue: tc.resultRequeue,
				})
			}

			result := reconciler.cleanupTaskResources(ctx, reconciler.logger, tc.task, fakeHandler)
			utils.CheckDruidErrorList(g, result.GetErrors(), tc.expectedResult.GetErrors())
			updatedTask := &druidv1alpha1.EtcdOpsTask{}
			err = cl.Get(ctx, types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, updatedTask)
			g.Expect(err).ToNot(HaveOccurred())
			utils.CheckLastOperation(g, updatedTask.Status.LastOperation, tc.expectedLastOperation)
			utils.CheckLastErrors(g, updatedTask.Status.LastErrors, tc.expectedLastErrors)
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
			name: "Should successfully remove finalizer when task has finalizer",
			task: &druidv1alpha1.EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-task",
					Namespace:  "test-ns",
					Finalizers: []string{druidapicommon.EtcdOpsTaskFinalizerName, "other-finalizer"},
				},
			},
			expectedResult: ctrlutils.ContinueReconcile(),
		},
		{
			name: "Should continue when task has no finalizer to remove",
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
			ctx := context.Background()
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
			err := cl.Create(ctx, tc.task)
			g.Expect(err).ToNot(HaveOccurred())

			reconciler := newTestReconciler(t, cl)
			fakeHandler := utils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)

			result := reconciler.removeTaskFinalizer(ctx, reconciler.logger, tc.task, fakeHandler)
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
		deleteError    bool
		expectedResult ctrlutils.ReconcileStepResult
	}{
		{
			name: "Should successfully delete task",
			task: &druidv1alpha1.EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "test-ns",
				},
			},
			deleteError:    false,
			expectedResult: ctrlutils.DoNotRequeue(),
		},
		{
			name: "Should return error when task deletion fails",
			task: &druidv1alpha1.EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "test-ns",
				},
			},
			deleteError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			var cl client.Client
			if tc.deleteError {
				statusErr := &apierrors.StatusError{ErrStatus: metav1.Status{Message: "delete failed"}}
				cl = utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(tc.task).RecordErrorForObjects(utils.ClientMethodDelete, statusErr, types.NamespacedName{Name: "test-task", Namespace: "test-ns"}).Build()
			} else {
				cl = utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
				if tc.task != nil {
					err := cl.Create(ctx, tc.task)
					g.Expect(err).ToNot(HaveOccurred(), "Failed to create test task")
				}
			}

			reconciler := newTestReconciler(t, cl)
			fakeHandler := utils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)
			fakeHandler.WithCleanup(handler.Result{Requeue: true})

			result := reconciler.removeTask(ctx, reconciler.logger, tc.task, fakeHandler)
			if tc.deleteError {
				g.Expect(result.HasErrors()).To(BeTrue())
				g.Expect(result.GetCombinedError()).To(HaveOccurred())
				g.Expect(result.GetCombinedError().Error()).To(ContainSubstring("delete failed"))
				return
			}
			g.Expect(result).To(Equal(tc.expectedResult))
		})
	}
}
