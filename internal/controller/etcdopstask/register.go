// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// TODO: Redefine the below after discussion.
package etcdopstask

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdopstasks,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdopstasks/status,verbs=get;create;update;patch

// RegisterWithManager sets up the controller on the given manager.
func (r *Reconciler) RegisterWithManager(mgr ctrl.Manager) error {
	return ctrl.
		NewControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				10*time.Millisecond,
				10*time.Second,
			),
		}).
		For(&druidv1alpha1.EtcdOpsTask{}).
		WithEventFilter(r.buildPredicate()).
		Complete(r)
}

// buildPredicate returns a predicate that filters events that are relevant for the Etcd controller.
// NOTE:
// For all conditions the following is applicable:
// 1. create and delete events are always reconciled irrespective of whether reconcile annotation is present or auto-reconcile has been enabled.
// 2. generic events are never reconciled. If there is a need in future to react to generic events then this should be changed.
// Conditions for reconciliation:
// Scenario 1: {Auto-Reconcile: false, Reconcile-Annotation-Present: false, Spec-Updated: true/false, Status-Updated: true/false, update-event-reconciled: false}
// Scenario 2: {Auto-Reconcile: false, Reconcile-Annotation-Present: true, Spec-Updated: true, Last-Reconcile-Succeeded: true/false, Status-Updated: true/false, update-event-reconciled: true}
// Scenario 3: {Auto-Reconcile: false, Reconcile-Annotation-Present: true, Spec-Updated: false, Last-Reconcile-Succeeded: true, Status-Updated: true/false, update-event-reconciled: true}
// Scenario 4: {Auto-Reconcile: false, Reconcile-Annotation-Present: true, Spec-Updated: false, Last-Reconcile-Succeeded: false, Status-Updated: true/false, update-event-reconciled: false}
// Scenario 5: {Auto-Reconcile: true, Reconcile-Annotation-Present: false, Spec-Updated: false, Status-Updated: false, update-event-reconciled: NA}, This condition cannot happen. In case of a controller restart there will only be a CreateEvent.
// Scenario 6: {Auto-Reconcile: true, Reconcile-Annotation-Present: false, Spec-Updated: true, Status-Updated: true/false, update-event-reconciled: true}
// Scenario 7: {Auto-Reconcile: true, Reconcile-Annotation-Present: false, Spec-Updated: false, Status-Updated: true/false, update-event-reconciled: false}
// Scenario 8: {Auto-Reconcile: true, Reconcile-Annotation-Present: true, Spec-Updated: true, Status-Updated: true/false, update-event-reconciled: true}
// Scenario 9: {Auto-Reconcile: true, Reconcile-Annotation-Present: true, Spec-Updated: false, Last-Reconcile-Succeeded: true, Status-Updated: true/false, update-event-reconciled: true}
// Scenario 9: {Auto-Reconcile: true, Reconcile-Annotation-Present: true, Spec-Updated: false, Last-Reconcile-Succeeded: false, Status-Updated: true/false, update-event-reconciled: false}
func (r *Reconciler) buildPredicate() predicate.Predicate {
	// If the reconcile annotation is set then only allow reconciliation if one of the conditions is true:
	// 1. There has been a spec update.
	// 2. The last reconcile operation has finished.
	// It is possible that during the previous reconcile one of the steps errored out. This gets captured in etcd.Status.LastOperation.
	// Update of status will generate an event. This event should not trigger a reconcile especially when the reconcile annotation has still
	// not been removed (since the last reconcile is not yet successfully completed).
	onReconcileAnnotationSetPredicate := predicate.And(
		r.hasReconcileAnnotation(),
	)

	return predicate.Or(
		onReconcileAnnotationSetPredicate,
	)
}

// hasReconcileAnnotation returns a predicate that filters events based on the presence of the reconcile annotation.
// Annotation `gardener.cloud/operation: reconcile` is used to force a reconcile for an etcd resource. Irrespective of
// enablement of `auto-reconcile` this annotation will trigger a reconcile. At the end of a successful reconcile spec flow
// it should be ensured that this annotation is removed successfully.
func (r *Reconciler) hasReconcileAnnotation() predicate.Predicate {
	// If the reconcile annotation is set then only allow reconciliation if one of the conditions is true:
	// 1. There has been a spec update.
	// 2. The last reconcile operation has finished.
	// It is possible that during the previous reconcile one of the steps errored out. This gets captured in etcd.Status.LastOperation.
	// Update of status will generate an event. This event should not trigger a reconcile especially when the reconcile annotation has still
	// not been removed (since the last reconcile is not yet successfully completed).
	return predicate.Funcs{
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			newEtcdOperatorTask, ok := updateEvent.ObjectNew.(*druidv1alpha1.EtcdOpsTask)
			if !ok {
				return false
			}
			return druidv1alpha1.HasReconcileOperationAnnotation(newEtcdOperatorTask.ObjectMeta)
		},
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		DeleteFunc:  func(_ event.DeleteEvent) bool { return true },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
	}
}
