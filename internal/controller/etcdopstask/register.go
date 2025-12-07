// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const controllerName = "etcdopstask-controller"

// RegisterWithManager sets up the controller on the given manager.
func (r *Reconciler) RegisterWithManager(mgr ctrl.Manager) error {
	return ctrl.
		NewControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: *r.config.ConcurrentSyncs,
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				10*time.Millisecond,
				r.config.RequeueInterval.Duration,
			),
		}).
		For(&druidv1alpha1.EtcdOpsTask{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
