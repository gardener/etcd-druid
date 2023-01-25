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

func (r *Reconciler) AddToManager(mgr ctrl.Manager, ignoreOperationAnnotation bool) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: r.Config.Workers,
	})
	builder = builder.
		WithEventFilter(buildPredicate(ignoreOperationAnnotation)).
		For(&druidv1alpha1.Etcd{})
	if ignoreOperationAnnotation {
		builder = builder.Owns(&corev1.Service{}).
			Owns(&corev1.ConfigMap{}).
			Owns(&appsv1.StatefulSet{})
	}
	return builder.Complete(r)
}

func buildPredicate(ignoreOperationAnnotation bool) predicate.Predicate {
	if ignoreOperationAnnotation {
		return predicate.GenerationChangedPredicate{}
	}

	return predicate.Or(
		druidpredicates.HasOperationAnnotation(),
		druidpredicates.LastOperationNotSuccessful(),
		predicateutils.IsDeleting(),
	)
}
