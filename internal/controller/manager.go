// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/controller/compaction"
	"github.com/gardener/etcd-druid/internal/controller/etcd"
	"github.com/gardener/etcd-druid/internal/controller/etcdcopybackupstask"
	"github.com/gardener/etcd-druid/internal/controller/secret"
	"github.com/gardener/etcd-druid/internal/webhook/sentinel"

	coordinationv1 "k8s.io/api/coordination/v1"
	coordinationv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	defaultTimeout = time.Minute
)

// CreateManagerWithControllers creates a controller manager and adds all the controllers to the controller-manager using the passed in ManagerConfig.
func CreateManagerWithControllers(config *ManagerConfig) (ctrl.Manager, error) {
	var (
		err error
		mgr ctrl.Manager
	)

	config.populateControllersFeatureGates()

	if mgr, err = createManager(config); err != nil {
		return nil, err
	}
	if err = registerControllersWithManager(mgr, config); err != nil {
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
		MetricsBindAddress:         config.Server.Metrics.BindAddress,
		Host:                       config.Server.Webhook.BindAddress,
		Port:                       config.Server.Webhook.Port,
		CertDir:                    config.Server.Webhook.TLS.ServerCertDir,
		LeaderElection:             config.EnableLeaderElection,
		LeaderElectionID:           config.LeaderElectionID,
		LeaderElectionResourceLock: config.LeaderElectionResourceLock,
	})
}

func registerControllersWithManager(mgr ctrl.Manager, config *ManagerConfig) error {
	var err error

	// Add etcd reconciler to the manager
	etcdReconciler, err := etcd.NewReconciler(mgr, config.EtcdControllerConfig)
	if err != nil {
		return err
	}
	if err = etcdReconciler.RegisterWithManager(mgr); err != nil {
		return err
	}

	// Add compaction reconciler to the manager if the CLI flag enable-backup-compaction is true.
	if config.CompactionControllerConfig.EnableBackupCompaction {
		compactionReconciler, err := compaction.NewReconciler(mgr, config.CompactionControllerConfig)
		if err != nil {
			return err
		}
		if err = compactionReconciler.RegisterWithManager(mgr); err != nil {
			return err
		}
	}

	// Add etcd-copy-backups-task reconciler to the manager
	etcdCopyBackupsTaskReconciler, err := etcdcopybackupstask.NewReconciler(mgr, config.EtcdCopyBackupsTaskControllerConfig)
	if err != nil {
		return err
	}
	if err = etcdCopyBackupsTaskReconciler.RegisterWithManager(mgr); err != nil {
		return err
	}

	// Add secret reconciler to the manager
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err = secret.NewReconciler(
		mgr,
		config.SecretControllerConfig,
	).RegisterWithManager(ctx, mgr); err != nil {
		return err
	}

	// Add sentinel webhook to the manager
	if config.SentinelWebhookConfig.Enabled {
		var sentinelWebhook *sentinel.Handler
		if sentinelWebhook, err = sentinel.NewHandler(
			mgr,
			config.SentinelWebhookConfig,
		); err != nil {
			return err
		}
		return sentinelWebhook.RegisterWithManager(mgr)
	}

	return nil
}
