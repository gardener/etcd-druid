package controllers

import (
	"github.com/gardener/etcd-druid/controllers/secretcontroller"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	coordinationv1 "k8s.io/api/coordination/v1"
	coordinationv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateAndAddToManager(config *ManagerConfig) (ctrl.Manager, error) {
	var (
		err error
		mgr ctrl.Manager
	)

	if mgr, err = createManager(config); err != nil {
		return nil, err
	}

	if err = (&secretcontroller.SecretReconciler{
		Config: config.SecretControllerConfig,
	}).AddToManager(mgr); err != nil {
		return nil, err
	}

	return mgr, nil
}

func createManager(config *ManagerConfig) (ctrl.Manager, error) {

	// TODO: this can be removed once we have an improved informer, see https://github.com/gardener/etcd-druid/issues/215
	// list of objects which should not be cached.
	uncachedObjects := []client.Object{
		&corev1.Event{},
		&eventsv1beta1.Event{},
		&eventsv1.Event{},
	}

	if config.DisableLeaseCache {
		uncachedObjects = append(uncachedObjects, &coordinationv1.Lease{}, &coordinationv1beta1.Lease{})
	}

	return ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		ClientDisableCacheFor:      uncachedObjects,
		Scheme:                     kubernetes.Scheme,
		MetricsBindAddress:         config.MetricsAddr,
		LeaderElection:             config.EnableLeaderElection,
		LeaderElectionID:           config.LeaderElectionID,
		LeaderElectionResourceLock: config.LeaderElectionResourceLock,
	})
}
