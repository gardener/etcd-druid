package etcd

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func (r *Reconciler) triggerReconcileSpec() error {
	return nil
}

func (r *Reconciler) canReconcileSpec(etcd *druidv1alpha1.Etcd) bool {
	//Check if spec reconciliation has been suspended, if yes, then record the event and return false.
	if suspendReconcileAnnotKey := r.getSuspendEtcdSpecReconcileAnnotationKey(etcd); suspendReconcileAnnotKey != nil {
		r.recordEtcdSpecReconcileSuspension(etcd, *suspendReconcileAnnotKey)
		return false
	}
	// Check if
	return true
}

// getSuspendEtcdSpecReconcileAnnotationKey gets the annotation key set on an etcd resource signalling the intent
// to suspend spec reconciliation for this etcd resource. If no annotation is set then it will return nil.
func (r *Reconciler) getSuspendEtcdSpecReconcileAnnotationKey(etcd *druidv1alpha1.Etcd) *string {
	var annotationKey *string
	if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation) {
		annotationKey = pointer.String(druidv1alpha1.SuspendEtcdSpecReconcileAnnotation)
	} else if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.IgnoreReconciliationAnnotation) {
		annotationKey = pointer.String(druidv1alpha1.IgnoreReconciliationAnnotation)
	}
	return annotationKey
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

//func (r *Reconciler) reconcileSpec(ctx context.Context, etcd *druidv1alpha1.Etcd, logger logr.Logger) utils.ReconcileStepResult {
//	operatorCtx := resource.NewOperatorContext(ctx, r.client, logger, etcd.GetNamespaceName())
//	resourceOperators := r.getOrderedOperatorsForSync()
//	for _, operator := range resourceOperators {
//		if err := operator.Sync(operatorCtx, etcd); err != nil {
//			return utils.ReconcileWithError(err)
//		}
//	}
//	return utils.ContinueReconcile()
//}
//
//func (r *Reconciler) getOrderedOperatorsForSync() []resource.Operator {
//	return []resource.Operator{
//		r.operatorRegistry.MemberLeaseOperator(),
//		r.operatorRegistry.SnapshotLeaseOperator(),
//		r.operatorRegistry.ClientServiceOperator(),
//		r.operatorRegistry.PeerServiceOperator(),
//		r.operatorRegistry.ConfigMapOperator(),
//		r.operatorRegistry.PodDisruptionBudgetOperator(),
//		r.operatorRegistry.ServiceAccountOperator(),
//		r.operatorRegistry.RoleOperator(),
//		r.operatorRegistry.RoleBindingOperator(),
//	}
//}
