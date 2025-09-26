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

	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	defaultTimeout = time.Minute
)

// Register registers all etcd-druid controllers with the controller manager.
func Register(mgr ctrl.Manager, config *Config) error {
	var err error

	// Add etcd reconciler to the manager
	etcdReconciler, err := etcd.NewReconciler(mgr, config.Etcd)
	if err != nil {
		return err
	}
	if err = etcdReconciler.RegisterWithManager(mgr, etcd.ControllerName); err != nil {
		return err
	}

	// Add compaction reconciler to the manager if the CLI flag enable-backup-compaction is true.
	if config.Compaction.EnableBackupCompaction {
		compactionReconciler, err := compaction.NewReconciler(mgr, config.Compaction)
		if err != nil {
			return err
		}
		if err = compactionReconciler.RegisterWithManager(mgr); err != nil {
			return err
		}
	}

	// Add etcd-copy-backups-task reconciler to the manager
	etcdCopyBackupsTaskReconciler, err := etcdcopybackupstask.NewReconciler(mgr, config.EtcdCopyBackupsTask)
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
		config.Secret,
	).RegisterWithManager(ctx, mgr)
}
