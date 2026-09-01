// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"context"
	"fmt"
	"testing"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/gomega"
)

// TestDeleteSurplusPVCs verifies that scale-in deletes the PVCs of removed pod
// ordinals (>= spec.replicas) and leaves the retained ones intact, while leaving
// all PVCs intact on a transition to zero replicas (hibernation). The function
// issues the deletes and returns nil (fire-and-forget); the caller (Sync) then
// shrinks the StatefulSet in the same pass, which reclaims the Terminating PVCs.
func TestDeleteSurplusPVCs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		specReplicas int32
		stsReplicas  int32
		// extraPVCs are ordinals whose PVCs exist beyond the StatefulSet's current
		// size, modelling leaked PVCs after Sync has already shrunk the STS or PVCs
		// retained across hibernation.
		extraPVCs []int
		// scaleInInProgress seeds ScaleOperationComplete=False/ScalingIn.
		scaleInInProgress bool
		// deleteIntercept, when non-nil, replaces the fake client's Delete verb so
		// individual cases can inject transport-level errors or NotFound responses.
		deleteIntercept func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error
		// wantErrCode, when set, asserts the function returns a DruidError with this
		// code instead of succeeding.
		wantErrCode druidapicommon.ErrorCode
		// wantDeleted / wantKept are pod ordinals whose PVCs must be gone / present.
		wantDeleted []int
		wantKept    []int
	}{
		{
			name:              "scale-in 5->3 deletes ordinals 3 and 4, keeps 0-2",
			specReplicas:      3,
			stsReplicas:       5,
			scaleInInProgress: true,
			wantDeleted:       []int{3, 4},
			wantKept:          []int{0, 1, 2},
		},
		{
			name:              "STS already shrunk to 3 with scale-in condition: delta is zero, no deletes",
			specReplicas:      3,
			stsReplicas:       3,
			extraPVCs:         []int{3, 4},
			scaleInInProgress: true,
			wantKept:          []int{0, 1, 2, 3, 4},
		},
		{
			name:         "no scale-in condition: keeps all PVCs",
			specReplicas: 5,
			stsReplicas:  5,
			wantKept:     []int{0, 1, 2, 3, 4},
		},
		{
			name:         "hibernation (spec 0): keeps all PVCs",
			specReplicas: 0,
			stsReplicas:  5,
			wantKept:     []int{0, 1, 2, 3, 4},
		},
		{
			name:         "wake-up from hibernation without scale-in condition: keeps PVCs",
			specReplicas: 3,
			stsReplicas:  0,
			extraPVCs:    []int{0, 1, 2, 3, 4},
			wantKept:     []int{0, 1, 2, 3, 4},
		},
		{
			name:              "Delete returns server error -> surfaces as ErrDeletePVC",
			specReplicas:      3,
			stsReplicas:       5,
			scaleInInProgress: true,
			deleteIntercept: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
					return fmt.Errorf("server error")
				}
				return nil
			},
			wantErrCode: ErrDeletePVC,
		},
		{
			name:              "surplus PVC absent (NotFound) -> treated as success",
			specReplicas:      3,
			stsReplicas:       5,
			scaleInInProgress: true,
			deleteIntercept: func(_ context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
					return apierrors.NewNotFound(corev1.Resource("persistentvolumeclaims"), obj.GetName())
				}
				return cl.Delete(context.Background(), obj, opts...)
			},
			wantKept: []int{0, 1, 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
				WithReplicas(tc.specReplicas).
				Build()
			etcd.UID = removalEtcdUID
			if tc.scaleInInProgress {
				etcd.Status.Conditions = []druidv1alpha1.Condition{{
					Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
					Status: druidv1alpha1.ConditionFalse,
					Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
				}}
			}

			sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, tc.stsReplicas)
			sts.Spec.Replicas = ptr.To(tc.stsReplicas)

			objs := []client.Object{sts}
			for i := int32(0); i < tc.stsReplicas; i++ {
				podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, int(i))
				objs = append(objs, testutils.CreatePVC(sts, podName, corev1.ClaimBound))
			}
			for _, ordinal := range tc.extraPVCs {
				podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, ordinal)
				objs = append(objs, testutils.CreatePVC(sts, podName, corev1.ClaimBound))
			}
			clientBuilder := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).WithObjects(objs...)
			if tc.deleteIntercept != nil {
				clientBuilder = clientBuilder.WithInterceptorFuncs(interceptor.Funcs{Delete: tc.deleteIntercept})
			}
			cl := clientBuilder.Build()

			r := _resource{client: cl}
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

			err := r.deleteSurplusPVCs(opCtx, etcd)
			if tc.wantErrCode != "" {
				g.Expect(err).To(HaveOccurred())
				derr := druiderr.AsDruidError(err)
				g.Expect(derr).NotTo(BeNil())
				g.Expect(derr.Code).To(Equal(tc.wantErrCode))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			vctName := etcd.Name
			stsName := druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta)
			pvcExists := func(ordinal int) bool {
				pvcName := fmt.Sprintf("%s-%s-%d", vctName, stsName, ordinal)
				getErr := cl.Get(context.Background(), types.NamespacedName{Namespace: removalNamespace, Name: pvcName}, &corev1.PersistentVolumeClaim{})
				return getErr == nil
			}
			for _, ordinal := range tc.wantDeleted {
				g.Expect(pvcExists(ordinal)).To(BeFalse(), "PVC for ordinal %d should be deleted", ordinal)
			}
			for _, ordinal := range tc.wantKept {
				g.Expect(pvcExists(ordinal)).To(BeTrue(), "PVC for ordinal %d should be retained", ordinal)
			}
		})
	}
}
