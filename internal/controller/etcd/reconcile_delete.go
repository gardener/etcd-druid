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
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// triggerDeletionFlow is the entry point for the deletion flow triggered for an etcd component which has a DeletionTimeStamp set on it.
func (r *Reconciler) triggerDeletionFlow(ctx component.OperatorContext, logger logr.Logger, etcdObjectKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	deleteStepFns := []reconcileFn{
		r.recordDeletionStartOperation,
		r.deleteEtcdResources,
		r.verifyNoResourcesAwaitCleanUp,
		r.removeFinalizer,
	}
	for _, fn := range deleteStepFns {
		if stepResult := fn(ctx, etcdObjectKey); ctrlutils.ShortCircuitReconcileFlow(stepResult) {
			return r.recordIncompleteDeletionOperation(ctx, logger, etcdObjectKey, stepResult)
		}
	}
	return ctrlutils.DoNotRequeue()
}

func (r *Reconciler) deleteEtcdResources(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcdPartialObjMeta := ctrlutils.EmptyEtcdPartialObjectMetadata()
	if result := ctrlutils.GetLatestEtcdPartialObjectMeta(ctx, r.client, etcdObjKey, etcdPartialObjMeta); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	operators := r.operatorRegistry.AllOperators()
	deleteTasks := make([]utils.OperatorTask, 0, len(operators))
	for kind, operator := range operators {
		// TODO: once we move to go 1.22 (https://go.dev/blog/loopvar-preview)
		operator := operator
		deleteTasks = append(deleteTasks, utils.OperatorTask{
			Name: fmt.Sprintf("triggerDeletionFlow-%s-component", kind),
			Fn: func(ctx component.OperatorContext) error {
				return operator.TriggerDelete(ctx, etcdPartialObjMeta.ObjectMeta)
			},
		})
	}
	ctx.Logger.Info("triggering triggerDeletionFlow operators for all resources")
	if errs := utils.RunConcurrently(ctx, deleteTasks); len(errs) > 0 {
		return ctrlutils.ReconcileWithError(errs...)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) verifyNoResourcesAwaitCleanUp(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcdPartialObjMeta := ctrlutils.EmptyEtcdPartialObjectMetadata()
	if result := ctrlutils.GetLatestEtcdPartialObjectMeta(ctx, r.client, etcdObjKey, etcdPartialObjMeta); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	operators := r.operatorRegistry.AllOperators()
	resourceNamesAwaitingCleanup := make([]string, 0, len(operators))
	for _, operator := range operators {
		existingResourceNames, err := operator.GetExistingResourceNames(ctx, etcdPartialObjMeta.ObjectMeta)
		if err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		resourceNamesAwaitingCleanup = append(resourceNamesAwaitingCleanup, existingResourceNames...)
	}
	if len(resourceNamesAwaitingCleanup) > 0 {
		ctx.Logger.Info("Cleanup of all resources has not yet been completed", "resourceNamesAwaitingCleanup", resourceNamesAwaitingCleanup)
		return ctrlutils.ReconcileAfter(5*time.Second, "Cleanup of all resources has not yet been completed. Skipping removal of Finalizer")
	}
	ctx.Logger.Info("All resources have been cleaned up")
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) removeFinalizer(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcdPartialObjMeta := ctrlutils.EmptyEtcdPartialObjectMetadata()
	if result := ctrlutils.GetLatestEtcdPartialObjectMeta(ctx, r.client, etcdObjKey, etcdPartialObjMeta); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	//etcd := &druidv1alpha1.Etcd{}
	//if result := ctrlutils.GetLatestEtcd(ctx, r.client, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
	//	return result
	//}
	ctx.Logger.Info("Removing finalizer", "finalizerName", common.FinalizerName)
	if err := controllerutils.RemoveFinalizers(ctx, r.client, etcdPartialObjMeta, common.FinalizerName); client.IgnoreNotFound(err) != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordDeletionStartOperation(ctx component.OperatorContext, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcdObjMeta := ctrlutils.EmptyEtcdPartialObjectMetadata()
	if result := ctrlutils.GetLatestEtcdPartialObjectMeta(ctx, r.client, etcdObjKey, etcdObjMeta); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if err := r.lastOpErrRecorder.RecordStart(ctx, etcdObjKey, druidv1alpha1.LastOperationTypeDelete); err != nil {
		ctx.Logger.Error(err, "failed to record etcd deletion start operation")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordIncompleteDeletionOperation(ctx component.OperatorContext, logger logr.Logger, etcdObjKey client.ObjectKey, exitReconcileStepResult ctrlutils.ReconcileStepResult) ctrlutils.ReconcileStepResult {
	etcdObjMeta := ctrlutils.EmptyEtcdPartialObjectMetadata()
	if result := ctrlutils.GetLatestEtcdPartialObjectMeta(ctx, r.client, etcdObjKey, etcdObjMeta); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if err := r.lastOpErrRecorder.RecordErrors(ctx, etcdObjKey, druidv1alpha1.LastOperationTypeDelete, exitReconcileStepResult.GetDescription(), exitReconcileStepResult.GetErrors()...); err != nil {
		logger.Error(err, "failed to record last operation and last errors for etcd deletion")
		return ctrlutils.ReconcileWithError(err)
	}
	return exitReconcileStepResult
}
