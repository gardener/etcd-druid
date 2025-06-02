// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler reconciles secrets referenced in Etcd objects.
type Reconciler struct {
	client.Client
	Config druidconfigv1alpha1.SecretControllerConfiguration
	logger logr.Logger
}

// NewReconciler creates a new reconciler for Secret.
func NewReconciler(mgr manager.Manager, config druidconfigv1alpha1.SecretControllerConfiguration) *Reconciler {
	return &Reconciler{
		Client: mgr.GetClient(),
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

	if needed, etcd := isFinalizerNeeded(secret.Name, etcdList); needed {
		if hasFinalizer(secret) {
			return ctrl.Result{}, nil
		}
		logger.Info("Adding finalizer for secret since it is referenced by etcd resource",
			"secretNamespace", secret.Namespace, "secretName", secret.Name, "etcdNamespace", etcd.Namespace, "etcdName", etcd.Name)
		return ctrl.Result{}, addFinalizer(ctx, logger, r.Client, secret)
	}
	return ctrl.Result{}, removeFinalizer(ctx, logger, r.Client, secret)
}

func isFinalizerNeeded(secretName string, etcdList *druidv1alpha1.EtcdList) (bool, *druidv1alpha1.Etcd) {
	for _, etcd := range etcdList.Items {
		if etcd.Spec.Etcd.ClientUrlTLS != nil &&
			(etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name == secretName ||
				etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name == secretName ||
				etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name == secretName) {
			return true, &etcd
		}

		if etcd.Spec.Etcd.PeerUrlTLS != nil &&
			(etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name == secretName ||
				etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name == secretName) { // Currently, no client certificate for peer url is used in ETCD cluster
			return true, &etcd
		}

		if etcd.Spec.Backup.Store != nil &&
			etcd.Spec.Backup.Store.SecretRef != nil &&
			etcd.Spec.Backup.Store.SecretRef.Name == secretName {
			return true, &etcd
		}
	}

	return false, nil
}

func hasFinalizer(secret *corev1.Secret) bool {
	return sets.NewString(secret.Finalizers...).Has(common.FinalizerName)
}

func addFinalizer(ctx context.Context, logger logr.Logger, k8sClient client.Client, secret *corev1.Secret) error {
	if finalizers := sets.NewString(secret.Finalizers...); finalizers.Has(common.FinalizerName) {
		return nil
	}
	logger.Info("Adding finalizer", "namespace", secret.Namespace, "name", secret.Name, "finalizerName", common.FinalizerName)
	return client.IgnoreNotFound(kubernetes.AddFinalizers(ctx, k8sClient, secret, common.FinalizerName))
}

func removeFinalizer(ctx context.Context, logger logr.Logger, k8sClient client.Client, secret *corev1.Secret) error {
	if finalizers := sets.NewString(secret.Finalizers...); !finalizers.Has(common.FinalizerName) {
		return nil
	}
	logger.Info("Removing finalizer", "namespace", secret.Namespace, "name", secret.Name, "finalizerName", common.FinalizerName)
	return client.IgnoreNotFound(kubernetes.RemoveFinalizers(ctx, k8sClient, secret, common.FinalizerName))
}
