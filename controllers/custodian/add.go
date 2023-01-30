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

const custodianControllerName = "custodian-controller"

func (r *Reconciler) AddToManager(ctx context.Context, mgr ctrl.Manager, ignoreOperationAnnotation bool) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: r.Config.Workers,
	})

	c, err := builder.
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
