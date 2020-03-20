// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package healthz

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	// ControllerName is the name of the controller
	ControllerName = "health_controller"
)

// SetupWithManager sets up manager with a new controller and r as the reconcile.Reconciler
func SetupWithManager(mgr ctrl.Manager, workers int) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: workers,
	})
	restCfg := mgr.GetConfig()
	cli, err := kubernetes.NewForConfig(restCfg, client.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		return err
	}
	reconciler := NewHealthReconciler(cli)
	// Owns(&appsv1.StatefulSet{}). doesn't add ownerReference to resource.
	// But since etcd-druid itself doesn't add ownerReference to statefulset,
	// because of missing CRD support in VPA, currently this line isn't much effective.
	return builder.For(&druidv1alpha1.Etcd{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&v1.Service{}).
		Complete(reconciler)
}
