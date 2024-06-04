// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
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

// buildPredicate returns a predicate that filters events that are relevant for the Etcd controller.
// NOTE:
// For all conditions the following is applicable:
// 1. create and delete events are always reconciled irrespective of whether reconcile annotation is present or auto-reconcile has been enabled.
// 2. generic events are never reconciled. If there is a need in future to react to generic events then this should be changed.
// Conditions for reconciliation:
// Scenario 1: {Auto-Reconcile: false, Reconcile-Annotation-Present: false, Spec-Updated: true/false, Status-Updated: true/false, update-event-reconciled: false}
// Scenario 2: {Auto-Reconcile: false, Reconcile-Annotation-Present: true, Spec-Updated: true/false, Status-Updated: true/false, update-event-reconciled: true}
// Scenario 3: {Auto-Reconcile: true, Reconcile-Annotation-Present: false, Spec-Updated: false, Status-Updated: false, update-event-reconciled: NA}, This condition cannot happen. In case of a controller restart there will only be a CreateEvent.
// Scenario 4: {Auto-Reconcile: true, Reconcile-Annotation-Present: false, Spec-Updated: true, Status-Updated: false, update-event-reconciled: true}
// Scenario 5: {Auto-Reconcile: true, Reconcile-Annotation-Present: false, Spec-Updated: false, Status-Updated: true, update-event-reconciled: false}
// Scenario 6: {Auto-Reconcile: true, Reconcile-Annotation-Present: false, Spec-Updated: true, Status-Updated: true, update-event-reconciled: true}
// Scenario 7: {Auto-Reconcile: true, Reconcile-Annotation-Present: false, Spec-Updated: false, Status-Updated: false, update-event-reconciled: false}
// Scenario 8: {Auto-Reconcile: true, Reconcile-Annotation-Present: true, Spec-Updated: true/false, Status-Updated: true/false, update-event-reconciled: true}
func (r *Reconciler) buildPredicate() predicate.Predicate {
	// If there is a spec change (irrespective of status change) and if there is an update event then it will trigger a reconcile only when either
	// auto-reconcile has been enabled or an operator has added the reconcile annotation to the etcd resource.
	onSpecChangePredicate := predicate.And(
		predicate.Or(
			r.hasReconcileAnnotation(),
			r.autoReconcileEnabled(),
		),
		specUpdated(),
	)

	return predicate.Or(
		r.hasReconcileAnnotation(),
		onSpecChangePredicate,
	)
}

// hasReconcileAnnotation returns a predicate that filters events based on the presence of the reconcile annotation.
// Annotation `gardener.cloud/operation: reconcile` is used to force a reconcile for an etcd resource. Irrespective of
// enablement of `auto-reconcile` this annotation will trigger a reconcile. At the end of a successful reconcile spec flow
// it should be ensured that this annotation is removed successfully.
func (r *Reconciler) hasReconcileAnnotation() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return updateEvent.ObjectNew.GetAnnotations()[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile
		},
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return true
		},
		DeleteFunc:  func(deleteEvent event.DeleteEvent) bool { return true },
		GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
	}
}

func (r *Reconciler) autoReconcileEnabled() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return r.config.EnableEtcdSpecAutoReconcile || r.config.IgnoreOperationAnnotation
		},
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return true
		},
		DeleteFunc:  func(deleteEvent event.DeleteEvent) bool { return true },
		GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
	}
}

func specUpdated() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return hasSpecChanged(updateEvent)
		},
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return true
		},
		DeleteFunc:  func(deleteEvent event.DeleteEvent) bool { return true },
		GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
	}
}

func hasSpecChanged(updateEvent event.UpdateEvent) bool {
	return updateEvent.ObjectNew.GetGeneration() != updateEvent.ObjectOld.GetGeneration()
}
