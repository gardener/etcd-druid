// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) completeReconcile(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	rLog := r.logger.WithValues("etcd", etcd.Name, "operation", "completeReconcile").WithValues("runID", ctx.RunID)
	ctx.SetLogger(rLog)

	reconcileCompletionStepFns := []reconcileFn{
		r.updateObservedGeneration,
		r.removeOperationAnnotation,
	}

	for _, fn := range reconcileCompletionStepFns {
		if stepResult := fn(ctx, etcd); ctrlutils.ShortCircuitReconcileFlow(stepResult) {
			return r.recordIncompleteReconcileOperation(ctx, etcd, stepResult)
		}
	}
	ctx.Logger.Info("Finished reconciliation completion flow")
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) updateObservedGeneration(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	originalEtcd := etcd.DeepCopy()
	etcd.Status.ObservedGeneration = &etcd.Generation
	if err := r.client.Status().Patch(ctx, etcd, client.MergeFrom(originalEtcd)); err != nil {
		ctx.Logger.Error(err, "failed to patch status.ObservedGeneration")
		return ctrlutils.ReconcileWithError(err)
	}
	ctx.Logger.Info("patched status.ObservedGeneration", "ObservedGeneration", etcd.Generation)
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) removeOperationAnnotation(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	if druidv1alpha1.HasReconcileOperationAnnotation(etcd.ObjectMeta) {
		ctx.Logger.Info("Removing operation annotation")
		withOpAnnotation := etcd.DeepCopy()
		druidv1alpha1.RemoveOperationAnnotation(etcd.ObjectMeta)
		if err := r.client.Patch(ctx, etcd, client.MergeFrom(withOpAnnotation)); err != nil {
			ctx.Logger.Error(err, "failed to remove operation annotation")
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}
