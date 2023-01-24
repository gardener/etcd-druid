package compactionlease

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druidpredicates "github.com/gardener/etcd-druid/pkg/predicate"
	coordinationv1 "k8s.io/api/coordination/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const compactionLeaseControllerName = "compaction-lease-controller"

func (r *Reconciler) AddToManager(mgr ctrl.Manager) error {
	c, err := controller.New(compactionLeaseControllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: r.config.Workers,
	})
	if err != nil {
		return err
	}

	return c.Watch(
		&source.Kind{Type: &coordinationv1.Lease{}},
		&handler.EnqueueRequestForOwner{OwnerType: &druidv1alpha1.Etcd{}, IsController: true},
		druidpredicates.LeaseHolderIdentityChange(),
		druidpredicates.IsSnapshotLease(),
	)
}
