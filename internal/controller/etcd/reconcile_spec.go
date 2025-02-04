// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// syncRetryInterval will be used by both sync and preSync stages for a component and should be used when there is a need to requeue for retrying after a specific interval.
const syncRetryInterval = 10 * time.Second

func (r *Reconciler) triggerReconcileSpecFlow(ctx component.OperatorContext, etcdObjectKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	reconcileStepFns := []reconcileFn{
		r.recordReconcileStartOperation,
		r.ensureFinalizer,
		r.preSyncEtcdResources,
		r.syncEtcdResources,
		r.recordReconcileSuccessOperation,
	}

	for _, fn := range reconcileStepFns {
		if stepResult := fn(ctx, etcdObjectKey); ctrlutils.ShortCircuitReconcileFlow(stepResult) {
			return r.recordIncompleteReconcileOperation(ctx, etcdObjectKey, stepResult)
		}
	}
	ctx.Logger.Info("Finished spec reconciliation flow")
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) ensureFinalizer(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcdPartialObjMeta := ctrlutils.EmptyEtcdPartialObjectMetadata()
	if result := ctrlutils.GetLatestEtcdPartialObjectMeta(ctx, r.client, etcdObjKey, etcdPartialObjMeta); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if !controllerutil.ContainsFinalizer(etcdPartialObjMeta, common.FinalizerName) {
		ctx.Logger.Info("Adding finalizer", "finalizerName", common.FinalizerName)
		if err := controllerutils.AddFinalizers(ctx, r.client, etcdPartialObjMeta, common.FinalizerName); err != nil {
			ctx.Logger.Error(err, "failed to add finalizer")
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) preSyncEtcdResources(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := ctrlutils.GetLatestEtcd(ctx, r.client, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	resourceOperators := r.getOrderedOperatorsForPreSync()
	for _, kind := range resourceOperators {
		op := r.operatorRegistry.GetOperator(kind)
		if err := op.PreSync(ctx, etcd); err != nil {
			if derr := druiderr.AsDruidError(err); derr != nil && derr.Code == druiderr.ErrRequeueAfter {
				ctx.Logger.Info("retrying pre-sync of component", "kind", kind, "syncRetryInterval", syncRetryInterval.String(), "reason", derr.Message)
				return ctrlutils.ReconcileAfter(syncRetryInterval, fmt.Sprintf("requeueing pre-sync of component %s to be retried after %s", kind, syncRetryInterval.String()))
			}
			ctx.Logger.Error(err, "failed to sync etcd resource", "kind", kind)
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) syncEtcdResources(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := ctrlutils.GetLatestEtcd(ctx, r.client, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	resourceOperators := r.getOrderedOperatorsForSync()
	for _, kind := range resourceOperators {
		op := r.operatorRegistry.GetOperator(kind)
		if err := op.Sync(ctx, etcd); err != nil {
			if derr := druiderr.AsDruidError(err); derr != nil && derr.Code == druiderr.ErrRequeueAfter {
				ctx.Logger.Info("retrying sync of component", "kind", kind, "syncRetryInterval", syncRetryInterval.String(), "reason", derr.Message)
				return ctrlutils.ReconcileAfter(syncRetryInterval, fmt.Sprintf("retrying sync of component %s after %s", kind, syncRetryInterval.String()))
			}
			ctx.Logger.Error(err, "failed to sync etcd resource", "kind", kind)
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordReconcileStartOperation(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	if err := r.lastOpErrRecorder.RecordStart(ctx, etcdObjKey, druidv1alpha1.LastOperationTypeReconcile); err != nil {
		ctx.Logger.Error(err, "failed to record etcd reconcile start operation")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordReconcileSuccessOperation(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	if err := r.lastOpErrRecorder.RecordSuccess(ctx, etcdObjKey, druidv1alpha1.LastOperationTypeReconcile); err != nil {
		ctx.Logger.Error(err, "failed to record etcd reconcile success operation")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordIncompleteReconcileOperation(ctx component.OperatorContext, etcdObjKey client.ObjectKey, exitReconcileStepResult ctrlutils.ReconcileStepResult) ctrlutils.ReconcileStepResult {
	if err := r.lastOpErrRecorder.RecordErrors(ctx, etcdObjKey, druidv1alpha1.LastOperationTypeReconcile, exitReconcileStepResult); err != nil {
		ctx.Logger.Error(err, "failed to record last operation and last errors for etcd reconciliation")
		return ctrlutils.ReconcileWithError(err)
	}
	return exitReconcileStepResult
}

// canReconcileSpec assesses whether the Etcd spec should undergo reconciliation.
//
// Reconciliation decision follows these rules:
// - Skipped if 'druid.gardener.cloud/suspend-etcd-spec-reconcile' annotation is present, signaling a pause in reconciliation.
// - Automatic reconciliation occurs if EnableEtcdSpecAutoReconcile is true.
// - If 'gardener.cloud/operation: reconcile' annotation exists and 'druid.gardener.cloud/suspend-etcd-spec-reconcile' annotation is not set, reconciliation proceeds upon Etcd spec changes.
// - Reconciliation is not initiated if EnableEtcdSpecAutoReconcile is false and none of the relevant annotations are present.
func (r *Reconciler) canReconcileSpec(etcd *druidv1alpha1.Etcd) bool {
	// Check if spec reconciliation has been suspended, if yes, then record the event and return false.
	if suspendReconcileAnnotKey := druidv1alpha1.GetSuspendEtcdSpecReconcileAnnotationKey(etcd.ObjectMeta); suspendReconcileAnnotKey != nil {
		r.recordEtcdSpecReconcileSuspension(etcd, *suspendReconcileAnnotKey)
		return false
	}

	// Prefer using EnableEtcdSpecAutoReconcile for automatic reconciliation.
	if r.config.EnableEtcdSpecAutoReconcile {
		return true
	}

	// Reconcile if the 'reconcile-op' annotation is present.
	if hasOperationAnnotationToReconcile(etcd) {
		return true
	}

	// If the observed generation is nil then it indicates that the Etcd is new and therefore spec reconciliation should be allowed.
	if etcd.Status.ObservedGeneration == nil {
		return true
	}

	// Default case: Do not reconcile.
	return false
}

func (r *Reconciler) recordEtcdSpecReconcileSuspension(etcd *druidv1alpha1.Etcd, annotationKey string) {
	r.recorder.Eventf(
		etcd,
		corev1.EventTypeWarning,
		"SpecReconciliationSkipped",
		"spec reconciliation of %s/%s is skipped by etcd-druid due to the presence of annotation %s on the etcd resource",
		etcd.Namespace,
		etcd.Name,
		annotationKey,
	)
}

func (r *Reconciler) getOrderedOperatorsForPreSync() []component.Kind {
	return []component.Kind{}
}

func (r *Reconciler) getOrderedOperatorsForSync() []component.Kind {
	return []component.Kind{
		component.MemberLeaseKind,
		component.SnapshotLeaseKind,
		component.ClientServiceKind,
		component.PeerServiceKind,
		component.ConfigMapKind,
		component.PodDisruptionBudgetKind,
		component.ServiceAccountKind,
		component.RoleKind,
		component.RoleBindingKind,
		component.StatefulSetKind,
	}
}

func hasOperationAnnotationToReconcile(etcd *druidv1alpha1.Etcd) bool {
	return etcd.GetAnnotations()[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile
}
