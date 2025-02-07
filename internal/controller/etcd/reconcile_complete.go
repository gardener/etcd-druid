package etcd

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) completeReconcile(ctx component.OperatorContext, etcdObjectKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	rLog := r.logger.WithValues("etcd", etcdObjectKey, "operation", "completeReconcile").WithValues("runID", ctx.RunID)
	ctx.SetLogger(rLog)

	reconcileCompletionStepFns := []reconcileFn{
		r.updateObservedGeneration,
		r.removeOperationAnnotation,
	}

	for _, fn := range reconcileCompletionStepFns {
		if stepResult := fn(ctx, etcdObjectKey); ctrlutils.ShortCircuitReconcileFlow(stepResult) {
			return r.recordIncompleteReconcileOperation(ctx, etcdObjectKey, stepResult)
		}
	}
	ctx.Logger.Info("Finished reconciliation completion flow")
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) updateObservedGeneration(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := ctrlutils.GetLatestEtcd(ctx, r.client, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	originalEtcd := etcd.DeepCopy()
	etcd.Status.ObservedGeneration = &etcd.Generation
	if err := r.client.Status().Patch(ctx, etcd, client.MergeFrom(originalEtcd)); err != nil {
		ctx.Logger.Error(err, "failed to patch status.ObservedGeneration")
		return ctrlutils.ReconcileWithError(err)
	}
	ctx.Logger.Info("patched status.ObservedGeneration", "ObservedGeneration", etcd.Generation)
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) removeOperationAnnotation(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcdPartialObjMeta := ctrlutils.EmptyEtcdPartialObjectMetadata()
	if result := ctrlutils.GetLatestEtcdPartialObjectMeta(ctx, r.client, etcdObjKey, etcdPartialObjMeta); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}

	if metav1.HasAnnotation(etcdPartialObjMeta.ObjectMeta, v1beta1constants.GardenerOperation) {
		ctx.Logger.Info("Removing operation annotation")
		withOpAnnotation := etcdPartialObjMeta.DeepCopy()
		delete(etcdPartialObjMeta.Annotations, v1beta1constants.GardenerOperation)
		if err := r.client.Patch(ctx, etcdPartialObjMeta, client.MergeFrom(withOpAnnotation)); err != nil {
			ctx.Logger.Error(err, "failed to remove operation annotation")
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}
