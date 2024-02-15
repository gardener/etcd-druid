package etcd

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const controllerName = "etcd-controller"

// RegisterWithManager registers the Etcd Controller with the given controller manager.
func (r *Reconciler) RegisterWithManager(mgr ctrl.Manager) error {
	builder := ctrl.
		NewControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.config.Workers,
			RateLimiter:             workqueue.NewItemExponentialFailureRateLimiter(10*time.Millisecond, r.config.EtcdStatusSyncPeriod),
		}).
		For(&druidv1alpha1.Etcd{}).
		WithEventFilter(r.buildPredicate())

	return builder.Complete(r)
}

func (r *Reconciler) buildPredicate() predicate.Predicate {
	// Since we do etcd status updates in the same reconciliation flow, this will also generate an event which should be ignored. Any changes in the predicates should ensure that this is not violated.
	return predicate.Or(
		// If reconcile operation annotation is present but there is no change to spec or status then we should allow reconciliation. Consider a case where the spec reconciliation has failed
		predicate.And(r.forcedReconcile(), noSpecAndStatusUpdated()),
		// If the reconciliation is allowed and the spec has changed, we should reconcile.
		predicate.And(r.reconcilePermitted(), onlySpecUpdated()),
	)
}

func onlySpecUpdated() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return hasSpecChanged(updateEvent) && !hasStatusChanged(updateEvent)
		},
	}
}

func noSpecAndStatusUpdated() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return !hasSpecChanged(updateEvent) && !hasStatusChanged(updateEvent)
		},
	}
}

func hasSpecChanged(updateEvent event.UpdateEvent) bool {
	return updateEvent.ObjectNew.GetGeneration() != updateEvent.ObjectOld.GetGeneration()
}

func hasStatusChanged(updateEvent event.UpdateEvent) bool {
	oldEtcd, ok := updateEvent.ObjectOld.(*druidv1alpha1.Etcd)
	if !ok {
		return false
	}
	newEtcd, ok := updateEvent.ObjectNew.(*druidv1alpha1.Etcd)
	if !ok {
		return false
	}
	return !apiequality.Semantic.DeepEqual(oldEtcd.Status, newEtcd.Status)
}

// reconcilePermitted
func (r *Reconciler) reconcilePermitted() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			if r.config.EnableEtcdSpecAutoReconcile || r.config.IgnoreOperationAnnotation {
				return true
			}
			return !hasReconcileAnnotation(updateEvent.ObjectOld) && hasReconcileAnnotation(updateEvent.ObjectNew)
		},
	}
}

func (r *Reconciler) forcedReconcile() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			if r.config.EnableEtcdSpecAutoReconcile || r.config.IgnoreOperationAnnotation {
				return false
			}
			return !hasReconcileAnnotation(updateEvent.ObjectOld) && hasReconcileAnnotation(updateEvent.ObjectNew)
		},
	}
}

func hasReconcileAnnotation(object client.Object) bool {
	return object.GetAnnotations()[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile
}
