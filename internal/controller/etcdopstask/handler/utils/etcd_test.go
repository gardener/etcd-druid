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

// TestGetEtcd tests the GetEtcd function.
func TestGetEtcd(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		etcdObject     *druidv1alpha1.Etcd
		etcdReference  types.NamespacedName
		phase          druidapicommon.LastOperationType
		expectedResult *taskhandler.Result
		expectErr      bool
	}{
		{
			name:       "Should return error without requeue when Etcd object is not found",
			etcdObject: nil,
			etcdReference: types.NamespacedName{
				Name:      "test-etcd",
				Namespace: "test-namespace",
			},
			phase: druidv1alpha1.LastOperationTypeAdmit,
			expectedResult: &taskhandler.Result{
				Description: "Etcd object not found",
				Error: &druiderr.DruidError{
					Code:      taskhandler.ErrGetEtcd,
					Operation: string(druidv1alpha1.LastOperationTypeAdmit),
					Message:   "etcd object not found",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Should successfully fetch Etcd object when it exists",
			etcdObject: testutils.EtcdBuilderWithDefaults("test-etcd", "test-namespace").Build(),
			etcdReference: types.NamespacedName{
				Name:      "test-etcd",
				Namespace: "test-namespace",
			},
			phase:          druidv1alpha1.LastOperationTypeAdmit,
			expectedResult: nil,
			expectErr:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var objs []client.Object
			if tc.etcdObject != nil {
				objs = append(objs, tc.etcdObject)
			}
			cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(objs...).Build()

			etcd, errResult := GetEtcd(context.Background(), cl, tc.etcdReference, tc.phase)

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
				g.Expect(etcd).ToNot(BeNil())
				g.Expect(etcd.Name).To(Equal(tc.etcdReference.Name))
				g.Expect(etcd.Namespace).To(Equal(tc.etcdReference.Namespace))
			}
		})
	}
}
