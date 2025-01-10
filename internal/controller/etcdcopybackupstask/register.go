// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdcopybackupstask

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const controllerName = "etcdcopybackupstask-controller"

// RegisterWithManager registers the EtcdCopyBackupsTask Controller with the given controller manager.
func (r *Reconciler) RegisterWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Config.Workers,
		}).
		For(
			&druidv1alpha1.EtcdCopyBackupsTask{},
			ctrlbuilder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Owns(&batchv1.Job{}).
		Complete(r)
}
