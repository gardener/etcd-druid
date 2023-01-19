package secretcontroller

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers"
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

// SecretReconciler reconciles secrets referenced in Etcd objects
type SecretReconciler struct {
	client.Client
	Config *Config
	logger logr.Logger
}

// NewSecretReconciler creates a new reconciler.
func NewSecretReconciler(mgr manager.Manager) *SecretReconciler {
	return &SecretReconciler{
		Client: mgr.GetClient(),
		logger: log.Log.WithName("secret-controller"),
	}
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update;patch

// Reconcile reconciles the secret.
func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
		return ctrl.Result{}, r.addFinalizer(ctx, logger, secret)
	}
	return ctrl.Result{}, r.removeFinalizer(ctx, logger, secret)
}

func (r *SecretReconciler) addFinalizer(ctx context.Context, logger logr.Logger, secret *corev1.Secret) error {
	if finalizers := sets.NewString(secret.Finalizers...); finalizers.Has(controllers.FinalizerName) {
		return nil
	}

	logger.Info("Adding finalizer")
	return client.IgnoreNotFound(controllerutils.AddFinalizers(ctx, r.Client, secret, controllers.FinalizerName))
}

func (r *SecretReconciler) removeFinalizer(ctx context.Context, logger logr.Logger, secret *corev1.Secret) error {
	if finalizers := sets.NewString(secret.Finalizers...); !finalizers.Has(controllers.FinalizerName) {
		return nil
	}

	logger.Info("Removing finalizer")
	return client.IgnoreNotFound(controllerutils.RemoveFinalizers(ctx, r.Client, secret, controllers.FinalizerName))
}

// SetupWithManager sets up manager with a new controller and s as the reconcile.Reconciler.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager, workers int) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: workers,
	})

	c, err := builder.
		For(&corev1.Secret{}).
		Build(r)
	if err != nil {
		return err
	}

	return c.Watch(
		&source.Kind{Type: &druidv1alpha1.Etcd{}},
		mapper.EnqueueRequestsFrom(druidmapper.EtcdToSecret(), mapper.UpdateWithOldAndNew, c.GetLogger()),
	)
}
