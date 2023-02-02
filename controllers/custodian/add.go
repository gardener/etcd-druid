// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package custodian

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druidmapper "github.com/gardener/etcd-druid/pkg/mapper"
	druidpredicates "github.com/gardener/etcd-druid/pkg/predicate"

	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "custodian-controller"

func (r *Reconciler) AddToManager(ctx context.Context, mgr ctrl.Manager, ignoreOperationAnnotation bool) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: r.config.Workers,
	})

	c, err := builder.
		Named(controllerName).
		For(
			&druidv1alpha1.Etcd{},
			ctrlbuilder.WithPredicates(druidpredicates.EtcdReconciliationFinished(ignoreOperationAnnotation))).
		Owns(&coordinationv1.Lease{}).
		Build(r)
	if err != nil {
		return err
	}

	return c.Watch(
		&source.Kind{Type: &appsv1.StatefulSet{}},
		mapper.EnqueueRequestsFrom(druidmapper.StatefulSetToEtcd(ctx, mgr.GetClient()), mapper.UpdateWithNew, c.GetLogger()),
		druidpredicates.StatefulSetStatusChange(),
	)
}
