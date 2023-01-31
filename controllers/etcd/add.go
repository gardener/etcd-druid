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

package etcd

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druidpredicates "github.com/gardener/etcd-druid/pkg/predicate"
	predicateutils "github.com/gardener/gardener/pkg/controllerutils/predicate"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const controllerName = "etcd-controller"

func (r *Reconciler) AddToManager(mgr ctrl.Manager, ignoreOperationAnnotation bool) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: r.Config.Workers,
	})
	builder = builder.
		Named(controllerName).
		WithEventFilter(BuildPredicate(ignoreOperationAnnotation)).
		For(&druidv1alpha1.Etcd{})
	if ignoreOperationAnnotation {
		builder = builder.Owns(&corev1.Service{}).
			Owns(&corev1.ConfigMap{}).
			Owns(&appsv1.StatefulSet{})
	}
	return builder.Complete(r)
}

func BuildPredicate(ignoreOperationAnnotation bool) predicate.Predicate {
	if ignoreOperationAnnotation {
		return predicate.GenerationChangedPredicate{}
	}

	return predicate.Or(
		druidpredicates.HasOperationAnnotation(),
		druidpredicates.LastOperationNotSuccessful(),
		predicateutils.IsDeleting(),
	)
}
