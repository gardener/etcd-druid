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
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler reconciles status of Etcd object
type Reconciler struct {
	client.Client
	scheme *runtime.Scheme
	config *Config
	logger logr.Logger
}

// NewReconciler creates a new reconciler for Custodian.
func NewReconciler(mgr manager.Manager, config *Config) *Reconciler {
	return &Reconciler{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: config,
		logger: log.Log.WithName(controllerName),
	}
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts;services;configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch

// Reconcile reconciles the etcd.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger.Info("Custodian controller reconciliation started")
	etcd := &druidv1alpha1.Etcd{}
	if err := r.Get(ctx, req.NamespacedName, etcd); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String())

	if !metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation) &&
		etcd.Status.LastOperation != nil && etcd.Status.LastOperation.State != druidv1alpha1.LastOperationStateProcessing {
		if err := r.triggerEtcdReconcile(ctx, logger, etcd); err != nil {
			return ctrl.Result{Requeue: true}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) triggerEtcdReconcile(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	logger.Info("Adding operation annotation", "annotation", v1beta1constants.GardenerOperation)
	withOpAnnotation := etcd.DeepCopy()
	withOpAnnotation.Annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
	return r.Patch(ctx, etcd, client.MergeFrom(withOpAnnotation))
}
