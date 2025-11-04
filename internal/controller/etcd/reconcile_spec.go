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
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// syncRetryInterval will be used by both sync and preSync stages for a component and should be used when there is a need to requeue for retrying after a specific interval.
const syncRetryInterval = 10 * time.Second

func (r *Reconciler) reconcileSpec(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	rLog := r.logger.WithValues("etcd", client.ObjectKeyFromObject(etcd), "operation", "reconcileSpec").WithValues("runID", ctx.RunID)
	ctx.SetLogger(rLog)

	reconcileStepFns := []reconcileFn{
		r.recordReconcileStartOperation,
		r.ensureFinalizer,
		r.preSyncEtcdResources,
		r.syncEtcdResources,
		r.cleanupEtcdResources,
		r.recordReconcileSuccessOperation,
	}

	for _, fn := range reconcileStepFns {
		if stepResult := fn(ctx, etcd); ctrlutils.ShortCircuitReconcileFlow(stepResult) {
			return r.recordIncompleteReconcileOperation(ctx, etcd, stepResult)
		}
	}
	ctx.Logger.Info("Finished spec reconciliation flow")
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) ensureFinalizer(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	if !controllerutil.ContainsFinalizer(etcd, common.FinalizerName) {
		ctx.Logger.Info("Adding finalizer", "finalizerName", common.FinalizerName)
		if err := kubernetes.AddFinalizers(ctx, r.client, etcd, common.FinalizerName); err != nil {
			ctx.Logger.Error(err, "failed to add finalizer")
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) preSyncEtcdResources(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
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

func (r *Reconciler) syncEtcdResources(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	resourceOperators := r.getOrderedOperatorsForSync(etcd.ObjectMeta)
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

// cleanupEtcdResources cleans up the resources that are no longer required for the Etcd cluster.
// This is required when runtime components are disabled for an Etcd cluster after it was created with runtime components enabled,
// so druid needs to ensure that the previously created runtime components are now cleaned up to avoid leaked resources in the cluster.
func (r *Reconciler) cleanupEtcdResources(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	resourceOperators := r.getOperatorsForCleanup(etcd.ObjectMeta)
	deleteTasks := make([]utils.OperatorTask, 0, len(resourceOperators))
	for _, kind := range resourceOperators {
		operator := r.operatorRegistry.GetOperator(kind)
		deleteTasks = append(deleteTasks, utils.OperatorTask{
			Name: fmt.Sprintf("cleanup-%s-component", kind),
			Fn: func(ctx component.OperatorContext) error {
				return operator.TriggerDelete(ctx, etcd.ObjectMeta)
			},
		})
	}

	ctx.Logger.Info("triggering cleanup for druid-managed resources that are no longer required")
	if errs := utils.RunConcurrently(ctx, deleteTasks); len(errs) > 0 {
		return ctrlutils.ReconcileWithError(errs...)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordReconcileStartOperation(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	if err := r.lastOpErrRecorder.RecordStart(ctx, etcd, druidv1alpha1.LastOperationTypeReconcile); err != nil {
		ctx.Logger.Error(err, "failed to record etcd reconcile start operation")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordReconcileSuccessOperation(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	if err := r.lastOpErrRecorder.RecordSuccess(ctx, etcd, druidv1alpha1.LastOperationTypeReconcile); err != nil {
		ctx.Logger.Error(err, "failed to record etcd reconcile success operation")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) recordIncompleteReconcileOperation(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, exitReconcileStepResult ctrlutils.ReconcileStepResult) ctrlutils.ReconcileStepResult {
	if err := r.lastOpErrRecorder.RecordErrors(ctx, etcd, druidv1alpha1.LastOperationTypeReconcile, exitReconcileStepResult); err != nil {
		ctx.Logger.Error(err, "failed to record last operation and last errors for etcd reconciliation")
		return ctrlutils.ReconcileWithError(err)
	}
	return exitReconcileStepResult
}

// shouldReconcileSpec assesses whether the Etcd spec should undergo reconciliation.
//
// Reconciliation decision follows these rules:
// - Skipped if 'druid.gardener.cloud/suspend-etcd-spec-reconcile' annotation is present, signaling a pause in reconciliation.
// - Automatic reconciliation occurs if EnableEtcdSpecAutoReconcile is true.
// - If 'gardener.cloud/operation: reconcile' annotation exists and 'druid.gardener.cloud/suspend-etcd-spec-reconcile' annotation is not set, reconciliation proceeds upon Etcd spec changes.
// - Reconciliation is not initiated if EnableEtcdSpecAutoReconcile is false and none of the relevant annotations are present.
func (r *Reconciler) shouldReconcileSpec(etcd *druidv1alpha1.Etcd) bool {
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
	if druidv1alpha1.HasReconcileOperationAnnotation(etcd.ObjectMeta) {
		return true
	}

	// If the observed generation is nil then it indicates that the Etcd is new and therefore spec reconciliation should be allowed.
	if etcd.Status.ObservedGeneration == nil {
		return true
	}

	// enabledByDefault case: Do not reconcile.
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

func (r *Reconciler) getOrderedOperatorsForSync(etcdObjMeta metav1.ObjectMeta) []component.Kind {
	var operators []component.Kind

	if druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(etcdObjMeta) {
		operators = []component.Kind{
			component.ServiceAccountKind,
			component.RoleKind,
			component.RoleBindingKind,
			component.MemberLeaseKind,
			component.SnapshotLeaseKind,
			component.PodDisruptionBudgetKind,
			component.ClientServiceKind,
			component.PeerServiceKind,
		}
	}

	// add the rest of the operators that are always needed for the etcd cluster
	operators = append(operators,
		component.ConfigMapKind,
		component.StatefulSetKind,
	)

	return operators
}

func (r *Reconciler) getOperatorsForCleanup(etcdObjMeta metav1.ObjectMeta) []component.Kind {
	if druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(etcdObjMeta) {
		return nil
	}
	return []component.Kind{
		component.ServiceAccountKind,
		component.RoleKind,
		component.RoleBindingKind,
		component.MemberLeaseKind,
		component.SnapshotLeaseKind,
		component.PodDisruptionBudgetKind,
		component.ClientServiceKind,
		component.PeerServiceKind,
	}
}
