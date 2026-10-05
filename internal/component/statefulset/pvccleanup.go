// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrDeletePVC indicates an error deleting a surplus PersistentVolumeClaim during scale-in.
const ErrDeletePVC druidapicommon.ErrorCode = "ERR_DELETE_PVC"

// deleteSurplusPVCs deletes the PVCs of pod ordinals in [etcd.Spec.Replicas,
// existingSTS.Spec.Replicas) during a scale-in. A StatefulSet does not reclaim
// its per-pod PVCs when scaled down, so without this the storage of removed
// members leaks. The pvc-protection finalizer keeps a PVC until its pod is
// gone, so deletion can be issued before the shrink.
func (r _resource) deleteSurplusPVCs(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, existingSTS *appsv1.StatefulSet) error {
	if !isPVCCleanupNeeded(etcd) || existingSTS == nil {
		return nil
	}

	prefix := pvcNamePrefix(etcd)
	stsReplicas := ptr.Deref(existingSTS.Spec.Replicas, 0)
	for ordinal := etcd.Spec.Replicas; ordinal < stsReplicas; ordinal++ {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s%d", prefix, ordinal),
				Namespace: etcd.Namespace,
			},
		}
		ctx.Logger.Info("deleting surplus PVC for scale-in", "pvc", pvc.Name)
		if err := client.IgnoreNotFound(r.client.Delete(ctx, pvc)); err != nil {
			return druiderr.WrapError(err, ErrDeletePVC, component.OperationSync,
				fmt.Sprintf("failed to delete surplus PVC %s for etcd: %v", pvc.Name, client.ObjectKeyFromObject(etcd)))
		}
	}
	return nil
}

// isPVCCleanupNeeded reports whether surplus PVC deletion should run. Deleting
// ordinal PVCs is only safe during an explicitly recorded scale-in: a transition
// to zero replicas leaves the data intact, and while scaling back out the
// StatefulSet can be at 0 while higher-ordinal PVCs from the previous cluster
// still exist and must be kept.
func isPVCCleanupNeeded(etcd *druidv1alpha1.Etcd) bool {
	return etcd.Spec.Replicas != 0 &&
		druidv1alpha1.IsScaleOperationInProgressWithReason(etcd, druidv1alpha1.ScaleOperationReasonScalingIn)
}

// pvcNamePrefix is the "<volumeClaimTemplate>-<statefulSet>-" prefix shared by
// every member PVC; the pod ordinal is the suffix.
func pvcNamePrefix(etcd *druidv1alpha1.Etcd) string {
	vctName := ptr.Deref(etcd.Spec.VolumeClaimTemplate, etcd.Name)
	return fmt.Sprintf("%s-%s-", vctName, druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta))
}
