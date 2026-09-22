// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"
	"strconv"
	"strings"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrDeletePVC indicates an error deleting a surplus PersistentVolumeClaim during scale-in.
const ErrDeletePVC druidapicommon.ErrorCode = "ERR_DELETE_PVC"

// deleteSurplusPVCs deletes the PVCs of pod ordinals at or above the desired
// replica count during a scale-in. A StatefulSet does not reclaim its per-pod
// PVCs when scaled down, so without this the storage of removed members leaks.
// It returns allDeleted == true once every surplus PVC is gone or terminating;
// Sync uses this to hold the replica shrink until deletion has been initiated.
func (r _resource) deleteSurplusPVCs(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (bool, error) {
	if !IsPVCCleanupNeeded(etcd) {
		return true, nil
	}

	pvcs, err := r.listMemberPVCs(ctx, etcd)
	if err != nil {
		return false, err
	}

	allDeleted := true
	prefix := pvcNamePrefix(etcd)
	for i := range pvcs {
		pvc := &pvcs[i]
		if !isSurplusPVC(pvc, prefix, etcd.Spec.Replicas) {
			continue
		}
		if pvc.DeletionTimestamp != nil {
			continue
		}
		allDeleted = false
		ctx.Logger.Info("deleting surplus PVC for scale-in", "pvc", pvc.Name)
		if err := client.IgnoreNotFound(r.client.Delete(ctx, pvc)); err != nil {
			return false, druiderr.WrapError(err, ErrDeletePVC, component.OperationSync,
				fmt.Sprintf("failed to delete surplus PVC %s for etcd: %v", pvc.Name, client.ObjectKeyFromObject(etcd)))
		}
	}
	return allDeleted, nil
}

// IsPVCCleanupNeeded reports whether surplus PVC deletion should run. Deleting
// ordinal PVCs is only safe during an explicitly recorded scale-in: a transition
// to zero replicas leaves the data intact, and while scaling back up the
// StatefulSet can be at 0 while higher-ordinal PVCs from the previous cluster
// still exist and must be kept.
func IsPVCCleanupNeeded(etcd *druidv1alpha1.Etcd) bool {
	return !druidv1alpha1.HasZeroReplicas(etcd) && druidv1alpha1.IsScaleInInProgress(etcd)
}

// listMemberPVCs lists the PVCs owned by this etcd via its default labels.
func (r _resource) listMemberPVCs(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]corev1.PersistentVolumeClaim, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.client.List(ctx, pvcList,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)),
	); err != nil {
		return nil, druiderr.WrapError(err, ErrDeletePVC, component.OperationSync,
			fmt.Sprintf("failed to list PVCs while deleting surplus PVCs for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	return pvcList.Items, nil
}

// pvcNamePrefix is the "<volumeClaimTemplate>-<statefulSet>-" prefix shared by
// every member PVC; the pod ordinal is the suffix.
func pvcNamePrefix(etcd *druidv1alpha1.Etcd) string {
	vctName := ptr.Deref(etcd.Spec.VolumeClaimTemplate, etcd.Name)
	return fmt.Sprintf("%s-%s-", vctName, druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta))
}

// isSurplusPVC reports whether pvc backs a pod ordinal at or above replicas.
// Non-member PVCs and names without an ordinal suffix are never surplus.
func isSurplusPVC(pvc *corev1.PersistentVolumeClaim, prefix string, replicas int32) bool {
	ordinalStr, ok := strings.CutPrefix(pvc.Name, prefix)
	if !ok {
		return false
	}
	ordinal, err := strconv.Atoi(ordinalStr)
	if err != nil {
		return false
	}
	return int32(ordinal) >= replicas // #nosec G115 G109 -- pod ordinal is a small non-negative index; the conversion cannot overflow.
}
