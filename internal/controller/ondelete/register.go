// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const controllerName = "ondelete-controller"

// RegisterWithManager registers the OnDelete controller with the given controller manager.
func (r *Reconciler) RegisterWithManager(mgr ctrl.Manager) error {
	return ctrl.
		NewControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: *r.config.ConcurrentSyncs,
		}).
		For(&appsv1.StatefulSet{}, builder.WithPredicates(onDeleteRolloutTrigger())).
		Owns(&corev1.Pod{}).
		Complete(r)
}

// onDeleteRolloutTrigger is the predicate for the OnDelete controller.
func onDeleteRolloutTrigger() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			sts, ok := e.Object.(*appsv1.StatefulSet)
			return ok && sts.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType
		},
		DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSts, ok := e.ObjectOld.(*appsv1.StatefulSet)
			if !ok {
				return false
			}
			newSts, ok := e.ObjectNew.(*appsv1.StatefulSet)
			if !ok {
				return false
			}
			if newSts.Spec.UpdateStrategy.Type != appsv1.OnDeleteStatefulSetStrategyType {
				return false
			}
			return oldSts.Status.UpdateRevision != newSts.Status.UpdateRevision ||
				oldSts.Spec.UpdateStrategy.Type != newSts.Spec.UpdateStrategy.Type
		},
	}
}
