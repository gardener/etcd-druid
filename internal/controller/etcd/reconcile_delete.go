// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"fmt"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// triggerDeletionFlow is the entry point for the deletion flow triggered for an etcd component which has a DeletionTimeStamp set on it.
func (r *Reconciler) triggerDeletionFlow(ctx component.OperatorContext, logger logr.Logger, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	deleteStepFns := []reconcileFn{
		r.recordDeletionStartOperation,
		r.deleteEtcdResources,
		r.verifyNoResourcesAwaitCleanUp,
		r.removeFinalizer,
	}
	for _, fn := range deleteStepFns {
		if stepResult := fn(ctx, etcd); ctrlutils.ShortCircuitReconcileFlow(stepResult) {
			return r.recordIncompleteDeletionOperation(ctx, logger, etcd, stepResult)
		}
	}
	return ctrlutils.DoNotRequeue()
}

func (r *Reconciler) deleteEtcdResources(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	operators := r.operatorRegistry.AllOperators()
	deleteTasks := make([]utils.OperatorTask, 0, len(operators))
	for kind, operator := range operators {
		deleteTasks = append(deleteTasks, utils.OperatorTask{
			Name: fmt.Sprintf("triggerDeletionFlow-%s-component", kind),
			Fn: func(ctx component.OperatorContext) error {
				return operator.TriggerDelete(ctx, etcd.ObjectMeta)
			},
		})
	}
	ctx.Logger.Info("triggering triggerDeletionFlow operators for all druid-managed resources")
	if errs := utils.RunConcurrently(ctx, deleteTasks); len(errs) > 0 {
		return ctrlutils.ReconcileWithError(errs...)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) verifyNoResourcesAwaitCleanUp(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	operators := r.operatorRegistry.AllOperators()
	resourceNamesAwaitingCleanup := make([]string, 0, len(operators))
	for _, operator := range operators {
		existingResourceNames, err := operator.GetExistingResourceNames(ctx, etcd.ObjectMeta)
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

func (r *Reconciler) removeFinalizer(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	ctx.Logger.Info("Removing finalizer", "finalizerName", druidapicommon.EtcdFinalizerName)
	if err := kubernetes.RemoveFinalizers(ctx, r.client, etcd, druidapicommon.EtcdFinalizerName); client.IgnoreNotFound(err) != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordDeletionStartOperation(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	if err := r.lastOpErrRecorder.RecordStart(ctx, etcd, druidv1alpha1.LastOperationTypeDelete); err != nil {
		ctx.Logger.Error(err, "failed to record etcd deletion start operation")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordIncompleteDeletionOperation(ctx component.OperatorContext, logger logr.Logger, etcd *druidv1alpha1.Etcd, exitReconcileStepResult ctrlutils.ReconcileStepResult) ctrlutils.ReconcileStepResult {
	if err := r.lastOpErrRecorder.RecordErrors(ctx, etcd, druidv1alpha1.LastOperationTypeDelete, exitReconcileStepResult); err != nil {
		logger.Error(err, "failed to record last operation and last errors for etcd deletion")
		return ctrlutils.ReconcileWithError(err)
	}
	return exitReconcileStepResult
}
