// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"testing"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// TestGetEtcdOpsTask tests the GetEtcdOpsTask function.
func TestGetEtcdOpsTask(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		taskObject     *druidv1alpha1.EtcdOpsTask
		taskReference  types.NamespacedName
		phase          druidapicommon.LastOperationType
		expectedResult *taskhandler.Result
		expectErr      bool
	}{
		{
			name:       "Should return error without requeue when EtcdOpsTask object is not found",
			taskObject: nil,
			taskReference: types.NamespacedName{
				Name:      "test-task",
				Namespace: "test-namespace",
			},
			phase: druidv1alpha1.LastOperationTypeExecution,
			expectedResult: &taskhandler.Result{
				Description: "EtcdOpsTask object not found",
				Error: &druiderr.DruidError{
					Code:      taskhandler.ErrGetEtcdOpsTask,
					Operation: string(druidv1alpha1.LastOperationTypeExecution),
					Message:   "etcdopstask object not found",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Should successfully fetch EtcdOpsTask object when it exists",
			taskObject: testutils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-namespace").Build(),
			taskReference: types.NamespacedName{
				Name:      "test-task",
				Namespace: "test-namespace",
			},
			phase:          druidv1alpha1.LastOperationTypeExecution,
			expectedResult: nil,
			expectErr:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var objs []client.Object
			if tc.taskObject != nil {
				objs = append(objs, tc.taskObject)
			}
			cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(objs...).Build()

			task, errResult := GetEtcdOpsTask(context.Background(), cl, tc.taskReference, tc.phase)

			if tc.expectErr {
				g.Expect(errResult).ToNot(BeNil())
				g.Expect(errResult.Description).To(Equal(tc.expectedResult.Description))
				g.Expect(errResult.Requeue).To(Equal(tc.expectedResult.Requeue))
				g.Expect(errResult.Error).ToNot(BeNil())

				if expectedDruidErr, ok := tc.expectedResult.Error.(*druiderr.DruidError); ok {
					druidErr := errResult.Error.(*druiderr.DruidError)
					g.Expect(druidErr.Code).To(Equal(expectedDruidErr.Code))
					g.Expect(druidErr.Operation).To(Equal(expectedDruidErr.Operation))
				}
			} else {
				g.Expect(errResult).To(BeNil())
				g.Expect(task).ToNot(BeNil())
				g.Expect(task.Name).To(Equal(tc.taskReference.Name))
				g.Expect(task.Namespace).To(Equal(tc.taskReference.Namespace))
			}
		})
	}
}
