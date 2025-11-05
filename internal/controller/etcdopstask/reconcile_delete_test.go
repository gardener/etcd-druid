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
		resultRequeue         bool
		expectedLastOperation *druidv1alpha1.LastOperation
		expectedLastErrors    *druidv1alpha1.LastError
		expectedPhase         *druidv1alpha1.OperationPhase
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
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:        druidv1alpha1.LastOperationTypeDelete,
				State:       druidv1alpha1.LastOperationStateProcessing,
				Description: "Cleanup skipped for rejected task",
			},
			expectedPhase: ptr.To(druidv1alpha1.OperationPhaseCleanup),
		},
		{
			name:           "result.requeue is false, no error, last operation is updated",
			task:           newTestTask(nil),
			cleanupFailed:  false,
			expectedResult: ctrlutils.ContinueReconcile(),
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeDelete,
				State: druidv1alpha1.LastOperationStateProcessing,
			},
			expectedPhase: ptr.To(druidv1alpha1.OperationPhaseCleanup),
		},
		{
			name:           "result.requeue is false, error, last operation and last Error is updated",
			task:           newTestTask(nil),
			expectedResult: ctrlutils.ReconcileWithError(druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error")),
			cleanupFailed:  true,
			resultRequeue:  false,
			expectedLastOperation: &druidv1alpha1.LastOperation{
				Type:  druidv1alpha1.LastOperationTypeDelete,
				State: druidv1alpha1.LastOperationStateError,
			},
			expectedLastErrors: &druidv1alpha1.LastError{
				Code:        druidv1alpha1.ErrorCode("TestError"),
				Description: "This is a test error",
			},
			expectedPhase: ptr.To(druidv1alpha1.OperationPhaseCleanup),
		},
		{
			name:           "result.requeue is true, no error, requeues task",
			task:           newTestTask(nil),
			cleanupFailed:  false,
			resultRequeue:  true,
			expectedResult: ctrlutils.ReconcileAfter(60*time.Second, "Task cleanup in progress"),
			expectedPhase:  ptr.To(druidv1alpha1.OperationPhaseCleanup),
		},
		{
			name:           "result.requeue is true, error, last Error is updated",
			task:           newTestTask(nil),
			expectedResult: ctrlutils.ReconcileWithError(druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error")),
			cleanupFailed:  true,
			resultRequeue:  true,
			expectedLastErrors: &druidv1alpha1.LastError{
				Code:        druidv1alpha1.ErrorCode("TestError"),
				Description: "This is a test error",
			},
			expectedPhase: ptr.To(druidv1alpha1.OperationPhaseCleanup),
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
			fakeHandler := utils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)
			if tc.cleanupFailed {
				fakeHandler.WithCleanup(handler.Result{
					Requeue: tc.resultRequeue,
					Error:   druiderr.WrapError(fmt.Errorf("test error"), "TestError", "TestOperation", "This is a test error"),
				})
			} else {
				fakeHandler.WithCleanup(handler.Result{
					Requeue: tc.resultRequeue,
				})
			}

			result := reconciler.cleanupTaskResources(context.TODO(), reconciler.logger, types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, fakeHandler)
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

			if tc.expectedPhase != nil {
				g.Expect(updatedTask.Status.Phase).ToNot(BeNil())
				g.Expect(*updatedTask.Status.Phase).To(Equal(*tc.expectedPhase))
			}
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
					Finalizers: []string{druidapicommon.EtcdOpsTaskFinalizerName, "other-finalizer"},
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
			fakeHandler := utils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)

			result := reconciler.removeTaskFinalizer(context.TODO(), reconciler.logger, types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, fakeHandler)
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
			fakeHandler := utils.NewFakeEtcdOpsTaskHandler("test-task", types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, reconciler.logger)
			fakeHandler.WithCleanup(handler.Result{Requeue: true})

			result := reconciler.removeTask(context.TODO(), reconciler.logger, types.NamespacedName{Name: "test-task", Namespace: "test-ns"}, fakeHandler)
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
