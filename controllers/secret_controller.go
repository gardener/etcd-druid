// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package controllers

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druidmapper "github.com/gardener/etcd-druid/pkg/mapper"

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Secret reconciles secrets referenced in Etcd objects
type Secret struct {
	client.Client
	logger logr.Logger
}

// NewSecret creates a new reconciler.
func NewSecret(mgr manager.Manager) *Secret {
	return &Secret{
		Client: mgr.GetClient(),
		logger: log.Log.WithName("secret-controller"),
	}
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=secrets,verbs=get;list;watch;update;patch

// Reconcile reconciles the secret.
func (s *Secret) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	secret := &corev1.Secret{}
	if err := s.Get(ctx, req.NamespacedName, secret); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger := s.logger.WithValues("secret", client.ObjectKeyFromObject(secret))

	etcdList := &druidv1alpha1.EtcdList{}
	if err := s.Client.List(ctx, etcdList, client.InNamespace(secret.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	needsFinalizer := false

	for _, etcd := range etcdList.Items {
		if etcd.Spec.Etcd.ClientUrlTLS != nil &&
			(etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name == secret.Name ||
				etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name == secret.Name ||
				etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name == secret.Name) {

			needsFinalizer = true
			break
		}

		if etcd.Spec.Etcd.PeerUrlTLS != nil &&
			(etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name == secret.Name ||
				etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name == secret.Name) { // Currently, no client certificate for peer url is used in ETCD cluster

			needsFinalizer = true
			break
		}

		if etcd.Spec.Backup.Store != nil &&
			etcd.Spec.Backup.Store.SecretRef != nil &&
			etcd.Spec.Backup.Store.SecretRef.Name == secret.Name {

			needsFinalizer = true
			break
		}
	}

	if needsFinalizer {
		return ctrl.Result{}, s.addFinalizer(ctx, logger, secret)
	}
	return ctrl.Result{}, s.removeFinalizer(ctx, logger, secret)
}

func (s *Secret) addFinalizer(ctx context.Context, logger logr.Logger, secret *corev1.Secret) error {
	if finalizers := sets.NewString(secret.Finalizers...); finalizers.Has(FinalizerName) {
		return nil
	}

	logger.Info("Adding finalizer")
	return client.IgnoreNotFound(controllerutils.StrategicMergePatchAddFinalizers(ctx, s.Client, secret, FinalizerName))
}

func (s *Secret) removeFinalizer(ctx context.Context, logger logr.Logger, secret *corev1.Secret) error {
	if finalizers := sets.NewString(secret.Finalizers...); !finalizers.Has(FinalizerName) {
		return nil
	}

	logger.Info("Removing finalizer")
	return client.IgnoreNotFound(controllerutils.PatchRemoveFinalizers(ctx, s.Client, secret, FinalizerName))
}

// SetupWithManager sets up manager with a new controller and s as the reconcile.Reconciler.
func (s *Secret) SetupWithManager(mgr ctrl.Manager, workers int) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: workers,
	})

	return builder.
		For(&corev1.Secret{}).
		Watches(
			&source.Kind{Type: &druidv1alpha1.Etcd{}},
			mapper.EnqueueRequestsFrom(druidmapper.EtcdToSecret(), mapper.UpdateWithOldAndNew),
		).
		Complete(s)
}
