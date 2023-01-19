package secretcontroller

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druidmapper "github.com/gardener/etcd-druid/pkg/mapper"
	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func (r *SecretReconciler) AddToManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: r.Config.Workers,
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
