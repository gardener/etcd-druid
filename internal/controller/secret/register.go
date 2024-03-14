// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druidmapper "github.com/gardener/etcd-druid/pkg/mapper"

	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "secret-controller"

// RegisterWithManager registers the Secret Controller with the given controller manager.
func (r *Reconciler) RegisterWithManager(ctx context.Context, mgr ctrl.Manager) error {
	c, err := ctrl.
		NewControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Config.Workers,
		}).
		For(&corev1.Secret{}).
		Build(r)
	if err != nil {
		return err
	}

	return c.Watch(
		&source.Kind{Type: &druidv1alpha1.Etcd{}},
		mapper.EnqueueRequestsFrom(ctx, mgr.GetCache(), druidmapper.EtcdToSecret(), mapper.UpdateWithOldAndNew, c.GetLogger()),
	)
}
