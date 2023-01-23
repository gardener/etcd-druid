// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package secret

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconciler reconciles secrets referenced in Etcd objects
type Reconciler struct {
	client.Client
	Config *Config
	logger logr.Logger
}

// NewReconciler creates a new reconciler.
func NewReconciler(client client.Client, config *Config) *Reconciler {
	return &Reconciler{
		Client: client,
		Config: config,
		logger: log.Log.WithName("secret-controller"),
	}
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update;patch

// Reconcile reconciles the secret.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, req.NamespacedName, secret); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger := r.logger.WithValues("secret", client.ObjectKeyFromObject(secret))

	etcdList := &druidv1alpha1.EtcdList{}
	if err := r.Client.List(ctx, etcdList, client.InNamespace(secret.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	if r.isFinalizerNeeded(secret.Name, etcdList) {
		return ctrl.Result{}, r.addFinalizer(ctx, logger, secret)
	}
	return ctrl.Result{}, r.removeFinalizer(ctx, logger, secret)
}

func (r *Reconciler) isFinalizerNeeded(secretName string, etcdList *druidv1alpha1.EtcdList) bool {
	for _, etcd := range etcdList.Items {
		if etcd.Spec.Etcd.ClientUrlTLS != nil &&
			(etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name == secretName ||
				etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name == secretName ||
				etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name == secretName) {
			return true
		}

		if etcd.Spec.Etcd.PeerUrlTLS != nil &&
			(etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name == secretName ||
				etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name == secretName) { // Currently, no client certificate for peer url is used in ETCD cluster
			return true
		}

		if etcd.Spec.Backup.Store != nil &&
			etcd.Spec.Backup.Store.SecretRef != nil &&
			etcd.Spec.Backup.Store.SecretRef.Name == secretName {
			return true
		}
	}

	return false
}

func (r *Reconciler) addFinalizer(ctx context.Context, logger logr.Logger, secret *corev1.Secret) error {
	if finalizers := sets.NewString(secret.Finalizers...); finalizers.Has(common.FinalizerName) {
		return nil
	}
	logger.Info("Adding finalizer")
	return client.IgnoreNotFound(controllerutils.AddFinalizers(ctx, r.Client, secret, common.FinalizerName))
}

func (r *Reconciler) removeFinalizer(ctx context.Context, logger logr.Logger, secret *corev1.Secret) error {
	if finalizers := sets.NewString(secret.Finalizers...); !finalizers.Has(common.FinalizerName) {
		return nil
	}
	logger.Info("Removing finalizer")
	return client.IgnoreNotFound(controllerutils.RemoveFinalizers(ctx, r.Client, secret, common.FinalizerName))
}
