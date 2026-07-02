// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const controllerName = "ondelete-controller"

// RegisterWithManager wires the OnDelete controller: it watches StatefulSets
// filtered by onDeleteStrategy() and updateRevisionChanged(), and their owned
// Pods so that pod-lifecycle transitions re-enqueue the parent StatefulSet.
func (r *Reconciler) RegisterWithManager(mgr ctrl.Manager) error {
	return ctrl.
		NewControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: *r.config.ConcurrentSyncs,
		}).
		For(&appsv1.StatefulSet{}, builder.WithPredicates(predicate.And(
			onDeleteStrategy(),
			updateRevisionChanged(),
		))).
		Owns(&corev1.Pod{}).
		Complete(r)
}

// onDeleteStrategy accepts events for StatefulSets whose current or previous
// updateStrategy.Type is OnDelete, so a transition into OnDelete is observed.
func onDeleteStrategy() predicate.Predicate {
	isOnDelete := func(obj client.Object) bool {
		sts, ok := obj.(*appsv1.StatefulSet)
		if !ok {
			return false
		}
		return sts.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType
	}
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isOnDelete(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isOnDelete(e.ObjectNew) || isOnDelete(e.ObjectOld)
		},
		DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
	}
}

// updateRevisionChanged accepts every Create (so restarts pick up mid-rollouts
// via artificial Create events on cache warmup) and Updates that change either
// updateRevision or the strategy type. Status-only churn is filtered out.
func updateRevisionChanged() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSts, ok := e.ObjectOld.(*appsv1.StatefulSet)
			if !ok {
				return false
			}
			newSts, ok := e.ObjectNew.(*appsv1.StatefulSet)
			if !ok {
				return false
			}
			if oldSts.Status.UpdateRevision != newSts.Status.UpdateRevision {
				return true
			}
			return oldSts.Spec.UpdateStrategy.Type != newSts.Spec.UpdateStrategy.Type
		},
		DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
	}
}
