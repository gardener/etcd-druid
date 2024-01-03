package etcd

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
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
		WithEventFilter(
			predicate.Or(r.etcdReconcilePredicate()),
		)

	return builder.Complete(r)
}

func (r *Reconciler) etcdReconcilePredicate() predicate.Predicate {
	// shouldReconcileBasedOnReconcileAnnotation checks if the object should be reconciled based on its annotations.
	shouldReconcileBasedOnReconcileAnnotation := func(obj client.Object) bool {
		if r.config.EnableEtcdSpecAutoReconcile || r.config.IgnoreOperationAnnotation {
			return true // Auto reconcile is enabled or operation annotations are ignored.
		}
		// Reconcile only if the GardenerOperation annotation is set to 'Reconcile'.
		return obj.GetAnnotations()[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile
	}

	// updatePredicate defines the logic to decide if an update event should trigger a reconcile.
	updatePredicate := func(event event.UpdateEvent) bool {
		if !shouldReconcileBasedOnReconcileAnnotation(event.ObjectNew) {
			return false // Skip if the new object does not have the required annotation.
		}

		// Reconcile if the old object did not have the reconcile annotation.
		if !shouldReconcileBasedOnReconcileAnnotation(event.ObjectOld) {
			return true // New reconcile annotation added to new object, hence reconcile required.
		}

		// Detect if there's a change in the spec.
		specChanged := event.ObjectNew.GetGeneration() != event.ObjectOld.GetGeneration()
		if specChanged {
			return true // Reconcile due to spec change.
		}

		etcd, ok := event.ObjectNew.(*druidv1alpha1.Etcd)
		if !ok {
			return false
		}

		if etcd.Status.LastOperation.Type == druidv1alpha1.LastOperationTypeReconcile && etcd.Status.LastOperation.State == druidv1alpha1.LastOperationStateSucceeded {
			return false // Skip reconcile if the last reconcile operation was successful.
		}

		// Calculate the time elapsed since the last update.
		elapsedDuration := time.Since(etcd.Status.LastOperation.LastUpdateTime.UTC())
		return elapsedDuration > r.config.EtcdStatusSyncPeriod // Reconcile if the time elapsed is more than sync period.
	}

	// Constructing the predicate.Funcs with specific functions for each event type.
	return predicate.Funcs{
		UpdateFunc:  updatePredicate,
		CreateFunc:  func(event event.CreateEvent) bool { return true },
		GenericFunc: func(event event.GenericEvent) bool { return shouldReconcileBasedOnReconcileAnnotation(event.Object) },
		DeleteFunc:  func(event event.DeleteEvent) bool { return true },
	}
}
