// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/operator"
	"github.com/gardener/etcd-druid/internal/operator/component"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) triggerReconcileSpecFlow(ctx component.OperatorContext, etcdObjectKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	reconcileStepFns := []reconcileFn{
		r.recordReconcileStartOperation,
		r.syncEtcdResources,
		r.updateObservedGeneration,
		r.recordReconcileSuccessOperation,
		r.removeOperationAnnotation,
	}

	for _, fn := range reconcileStepFns {
		if stepResult := fn(ctx, etcdObjectKey); ctrlutils.ShortCircuitReconcileFlow(stepResult) {
			return r.recordIncompleteReconcileOperation(ctx, etcdObjectKey, stepResult)
		}
	}
	ctx.Logger.Info("Finished spec reconciliation flow")
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) removeOperationAnnotation(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if _, ok := etcd.Annotations[v1beta1constants.GardenerOperation]; ok {
		ctx.Logger.Info("Removing operation annotation")
		withOpAnnotation := etcd.DeepCopy()
		delete(etcd.Annotations, v1beta1constants.GardenerOperation)
		if err := r.client.Patch(ctx, etcd, client.MergeFrom(withOpAnnotation)); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}
func (r *Reconciler) syncEtcdResources(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	resourceOperators := r.getOrderedOperatorsForSync()
	for _, kind := range resourceOperators {
		op := r.operatorRegistry.GetOperator(kind)
		if err := op.Sync(ctx, etcd); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) updateObservedGeneration(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	originalEtcd := etcd.DeepCopy()
	etcd.Status.ObservedGeneration = &etcd.Generation
	if err := r.client.Status().Patch(ctx, etcd, client.MergeFrom(originalEtcd)); err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	ctx.Logger.Info("patched status.ObservedGeneration", "ObservedGeneration", etcd.Generation)
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
	if err := r.lastOpErrRecorder.RecordError(ctx, etcdObjKey, druidv1alpha1.LastOperationTypeReconcile, exitReconcileStepResult.GetDescription(), exitReconcileStepResult.GetErrors()...); err != nil {
		ctx.Logger.Error(err, "failed to record last operation and last errors for etcd reconciliation")
		return ctrlutils.ReconcileWithError(err)
	}
	return exitReconcileStepResult
}

// canReconcileSpec assesses whether the Etcd spec should undergo reconciliation.
//
// Reconciliation decision follows these rules:
// - Skipped if 'druid.gardener.cloud/suspend-etcd-spec-reconcile' annotation is present, signaling a pause in reconciliation.
// - Also skipped if the deprecated 'druid.gardener.cloud/ignore-reconciliation' annotation is set.
// - Automatic reconciliation occurs if EnableEtcdSpecAutoReconcile is true.
// - If 'gardener.cloud/operation: reconcile' annotation exists and neither 'druid.gardener.cloud/suspend-etcd-spec-reconcile' nor the deprecated 'druid.gardener.cloud/ignore-reconciliation' is set to true, reconciliation proceeds upon Etcd spec changes.
// - Reconciliation is not initiated if EnableEtcdSpecAutoReconcile is false and none of the relevant annotations are present.
func (r *Reconciler) canReconcileSpec(etcd *druidv1alpha1.Etcd) bool {
	// Check if spec reconciliation has been suspended, if yes, then record the event and return false.
	if suspendReconcileAnnotKey := etcd.GetSuspendEtcdSpecReconcileAnnotationKey(); suspendReconcileAnnotKey != nil {
		r.recordEtcdSpecReconcileSuspension(etcd, *suspendReconcileAnnotKey)
		return false
	}

	// Prefer using EnableEtcdSpecAutoReconcile for automatic reconciliation.
	if r.config.EnableEtcdSpecAutoReconcile {
		return true
	}

	// Fallback to deprecated IgnoreOperationAnnotation if EnableEtcdSpecAutoReconcile is false.
	if r.config.IgnoreOperationAnnotation {
		return true
	}

	// Reconcile if the 'reconcile-op' annotation is present.
	if hasOperationAnnotationToReconcile(etcd) {
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

func (r *Reconciler) getOrderedOperatorsForSync() []operator.Kind {
	return []operator.Kind{
		operator.MemberLeaseKind,
		operator.SnapshotLeaseKind,
		operator.ClientServiceKind,
		operator.PeerServiceKind,
		operator.ConfigMapKind,
		operator.PodDisruptionBudgetKind,
		operator.ServiceAccountKind,
		operator.RoleKind,
		operator.RoleBindingKind,
		operator.StatefulSetKind,
	}
}

func hasOperationAnnotationToReconcile(etcd *druidv1alpha1.Etcd) bool {
	return etcd.GetAnnotations()[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile
}
