// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	"github.com/gardener/etcd-druid/internal/controller/compaction"
	"github.com/gardener/etcd-druid/internal/controller/etcd"
	"github.com/gardener/etcd-druid/internal/controller/etcdcopybackupstask"
	"github.com/gardener/etcd-druid/internal/controller/secret"

	configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	defaultTimeout = time.Minute
)

// Register registers all etcd-druid controllers with the controller manager.
func Register(mgr ctrl.Manager, controllerConfig configv1alpha1.ControllerConfiguration) error {
	var err error

	// Add etcd reconciler to the manager
	etcdReconciler, err := etcd.NewReconciler(mgr, controllerConfig.Etcd)
	if err != nil {
		return err
	}
	if err = etcdReconciler.RegisterWithManager(mgr, etcd.ControllerName); err != nil {
		return err
	}

	// Add compaction reconciler to the manager if the CLI flag enable-backup-compaction is true.
	if controllerConfig.Compaction.Enabled {
		compactionReconciler, err := compaction.NewReconciler(mgr, controllerConfig.Compaction)
		if err != nil {
			return err
		}
		if err = compactionReconciler.RegisterWithManager(mgr); err != nil {
			return err
		}
	}

	// Add etcd-copy-backups-task reconciler to the manager
	etcdCopyBackupsTaskReconciler, err := etcdcopybackupstask.NewReconciler(mgr, controllerConfig.EtcdCopyBackupsTask)
	if err != nil {
		return err
	}
	if err = etcdCopyBackupsTaskReconciler.RegisterWithManager(mgr); err != nil {
		return err
	}

	// Add secret reconciler to the manager
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	return secret.NewReconciler(
		mgr,
		controllerConfig.Secret,
	).RegisterWithManager(ctx, mgr)
}
