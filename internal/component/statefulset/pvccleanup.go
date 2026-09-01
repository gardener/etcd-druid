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
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrDeletePVC is returned when deleting a surplus PersistentVolumeClaim fails.
const ErrDeletePVC druidapicommon.ErrorCode = "ERR_DELETE_PVC"

// deleteSurplusPVCs deletes PVCs for ordinals in [spec.replicas, sts.replicas)
// during a scale-in and returns nil so the caller (Sync) proceeds to shrink the
// StatefulSet in the same pass. Returns nil when there is nothing to delete.
//
// The delete is fire-and-forget: a surplus PVC still mounted by its pod is held
// in Terminating by the pvc-protection finalizer, and is reclaimed once the
// StatefulSet shrink (createOrPatch, called right after this in Sync) deletes the
// pod. Requeuing here to wait for termination would deadlock, because the pod is
// only removed by that shrink. See docs/proposals/08-scale-in.md.
//
// A transition to zero replicas is hibernation, not a scale-in; PVCs are left
// intact in that case.
func (r _resource) deleteSurplusPVCs(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	if etcd.Spec.Replicas <= 0 || !druidv1alpha1.IsScaleInInProgress(etcd) {
		return nil
	}
	delta, err := kubernetes.ComputeScaleInReplicaDelta(ctx, r.client, etcd)
	if err != nil {
		return druiderr.WrapError(err, ErrDeletePVC, component.OperationSync,
			fmt.Sprintf("failed to get StatefulSet while deleting surplus PVCs for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	if delta == 0 {
		return nil
	}
	var errs error
	for _, e := range utils.RunConcurrently(ctx, r.surplusPVCDeleteTasks(etcd, delta)) {
		errs = multierror.Append(errs, e)
	}
	return errs
}

// surplusPVCDeleteTasks returns one delete task per surplus ordinal in
// [spec.replicas, spec.replicas+delta).
func (r _resource) surplusPVCDeleteTasks(etcd *druidv1alpha1.Etcd, delta int32) []utils.OperatorTask {
	vctName := ptr.Deref(etcd.Spec.VolumeClaimTemplate, etcd.Name)
	tasks := make([]utils.OperatorTask, 0, delta)
	for ordinal := etcd.Spec.Replicas; ordinal < etcd.Spec.Replicas+delta; ordinal++ {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, int(ordinal))
		objKey := client.ObjectKey{
			Name:      fmt.Sprintf("%s-%s", vctName, podName),
			Namespace: etcd.Namespace,
		}
		tasks = append(tasks, utils.OperatorTask{
			Name: "DeletePVC-" + objKey.Name,
			Fn: func(ctx component.OperatorContext) error {
				return r.doDeleteSurplusPVC(ctx, etcd, objKey)
			},
		})
	}
	return tasks
}

// doDeleteSurplusPVC deletes a single surplus PVC. A NotFound response is
// treated as success because the PVC may have been removed out-of-band.
func (r _resource) doDeleteSurplusPVC(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, objKey client.ObjectKey) error {
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = objKey.Name
	pvc.Namespace = objKey.Namespace
	if err := r.client.Delete(ctx, pvc); client.IgnoreNotFound(err) != nil {
		return druiderr.WrapError(err, ErrDeletePVC, component.OperationSync,
			fmt.Sprintf("failed to delete surplus PVC %s for etcd: %v", objKey.Name, client.ObjectKeyFromObject(etcd)))
	}
	ctx.Logger.Info("deleted surplus PVC", "pvc", objKey.Name)
	return nil
}
