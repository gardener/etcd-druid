// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"time"

	"github.com/gardener/etcd-druid/controllers/compaction"
	"github.com/gardener/etcd-druid/controllers/custodian"
	"github.com/gardener/etcd-druid/controllers/etcd"
	"github.com/gardener/etcd-druid/controllers/etcdcopybackupstask"
	"github.com/gardener/etcd-druid/controllers/secret"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"

	coordinationv1 "k8s.io/api/coordination/v1"
	coordinationv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
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
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: uncachedObjects,
			},
		},
		Scheme: kubernetes.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: config.MetricsAddr,
		},
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
	if err = etcdReconciler.RegisterWithManager(mgr, config.IgnoreOperationAnnotation); err != nil {
		return err
	}

	// Add custodian reconciler to the manager
	custodianReconciler := custodian.NewReconciler(mgr, config.CustodianControllerConfig)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err = custodianReconciler.RegisterWithManager(ctx, mgr, config.IgnoreOperationAnnotation); err != nil {
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
	return secret.NewReconciler(
		mgr,
		config.SecretControllerConfig,
	).RegisterWithManager(ctx, mgr)
}
