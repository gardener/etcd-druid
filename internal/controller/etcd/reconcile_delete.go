package etcd

import (
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// deleteStepFn is a step in the deletion flow. Every deletion step must have this signature.
type deleteStepFn func(ctx resource.OperatorContext, logger logr.Logger, etcdObjectKey client.ObjectKey) ctrlutils.ReconcileStepResult

// triggerDeletionFlow is the entry point for the deletion flow triggered for an etcd resource which has a DeletionTimeStamp set on it.
func (r *Reconciler) triggerDeletionFlow(ctx resource.OperatorContext, logger logr.Logger, etcdObjectKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	deleteStepFns := []deleteStepFn{
		r.recordDeletionStartOperation,
		r.deleteEtcdResources,
		r.verifyNoResourcesAwaitCleanUp,
		r.removeFinalizer,
	}
	for _, fn := range deleteStepFns {
		if stepResult := fn(ctx, logger, etcdObjectKey); ctrlutils.ShortCircuitReconcileFlow(stepResult) {
			return r.recordIncompleteDeletionOperation(ctx, logger, etcdObjectKey, stepResult)
		}
	}
	return ctrlutils.DoNotRequeue()
}

func (r *Reconciler) deleteEtcdResources(ctx resource.OperatorContext, logger logr.Logger, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	operators := r.operatorRegistry.AllOperators()
	deleteTasks := make([]utils.OperatorTask, len(operators))
	for kind, operator := range operators {
		operator := operator
		deleteTasks = append(deleteTasks, utils.OperatorTask{
			Name: fmt.Sprintf("triggerDeletionFlow-%s-operator", kind),
			Fn: func(ctx resource.OperatorContext) error {
				return operator.TriggerDelete(ctx, etcd)
			},
		})
	}
	logger.Info("triggering triggerDeletionFlow operators for all resources")
	if errs := utils.RunConcurrently(ctx, deleteTasks); len(errs) > 0 {
		return ctrlutils.ReconcileWithError(errs...)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) verifyNoResourcesAwaitCleanUp(ctx resource.OperatorContext, logger logr.Logger, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	operators := r.operatorRegistry.AllOperators()
	resourceNamesAwaitingCleanup := make([]string, 0, len(operators))
	for _, operator := range operators {
		existingResourceNames, err := operator.GetExistingResourceNames(ctx, etcd)
		if err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
		resourceNamesAwaitingCleanup = append(resourceNamesAwaitingCleanup, existingResourceNames...)
	}
	if len(resourceNamesAwaitingCleanup) > 0 {
		logger.Info("Cleanup of all resources has not yet been completed", "resourceNamesAwaitingCleanup", resourceNamesAwaitingCleanup)
		return ctrlutils.ReconcileAfter(5*time.Second, "Cleanup of all resources has not yet been completed. Skipping removal of Finalizer")
	}
	logger.Info("All resources have been cleaned up")
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) removeFinalizer(ctx resource.OperatorContext, logger logr.Logger, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	logger.Info("Removing finalizer", "finalizerName", common.FinalizerName)
	if err := controllerutils.RemoveFinalizers(ctx, r.client, etcd, common.FinalizerName); client.IgnoreNotFound(err) != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordDeletionStartOperation(ctx resource.OperatorContext, logger logr.Logger, etcdObjKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if err := r.lastOpErrRecorder.RecordStart(ctx, etcd, druidv1alpha1.LastOperationTypeDelete); err != nil {
		logger.Error(err, "failed to record etcd deletion start operation")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordIncompleteDeletionOperation(ctx resource.OperatorContext, logger logr.Logger, etcdObjKey client.ObjectKey, exitReconcileStepResult ctrlutils.ReconcileStepResult) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if err := r.lastOpErrRecorder.RecordError(ctx, etcd, druidv1alpha1.LastOperationTypeDelete, exitReconcileStepResult.GetDescription(), exitReconcileStepResult.GetErrors()...); err != nil {
		logger.Error(err, "failed to record last operation and last errors for etcd deletion")
		return ctrlutils.ReconcileWithError(err)
	}
	return exitReconcileStepResult
}
