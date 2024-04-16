// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package custodian

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druidpredicates "github.com/gardener/etcd-druid/controllers/predicate"
	druidmapper "github.com/gardener/etcd-druid/pkg/mapper"

	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "custodian-controller"

// RegisterWithManager registers the Custodian Controller with the given controller manager.
func (r *Reconciler) RegisterWithManager(ctx context.Context, mgr ctrl.Manager, ignoreOperationAnnotation bool) error {
	c, err := ctrl.
		NewControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.config.Workers,
		}).
		For(
			&druidv1alpha1.Etcd{},
			ctrlbuilder.WithPredicates(druidpredicates.EtcdReconciliationFinished(ignoreOperationAnnotation)),
		).
		Build(r)
	if err != nil {
		return err
	}

	return c.Watch(
		source.Kind(mgr.GetCache(), &appsv1.StatefulSet{}),
		mapper.EnqueueRequestsFrom(ctx, mgr.GetCache(), druidmapper.StatefulSetToEtcd(ctx, mgr.GetClient()), mapper.UpdateWithNew, c.GetLogger()),
		druidpredicates.StatefulSetStatusChange(),
	)
}
