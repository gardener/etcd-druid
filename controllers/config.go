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
	"flag"

	"github.com/gardener/etcd-druid/controllers/compaction"
	"github.com/gardener/etcd-druid/controllers/custodian"
	"github.com/gardener/etcd-druid/controllers/etcd"
	"github.com/gardener/etcd-druid/controllers/etcdcopybackupstask"
	"github.com/gardener/etcd-druid/controllers/secret"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

const (
	metricsAddrFlagName                = "metrics-addr"
	enableLeaderElectionFlagName       = "enable-leader-election"
	leaderElectionIDFlagName           = "leader-election-id"
	leaderElectionResourceLockFlagName = "leader-election-resource-lock"
	ignoreOperationAnnotationFlagName  = "ignore-operation-annotation"
	disableLeaseCacheFlagName          = "disable-lease-cache"

	defaultMetricsAddr                = ":8080"
	defaultEnableLeaderElection       = false
	defaultLeaderElectionID           = "druid-leader-election"
	defaultLeaderElectionResourceLock = resourcelock.LeasesResourceLock
	defaultIgnoreOperationAnnotation  = false
	defaultDisableLeaseCache          = false
)

type LeaderElectionConfig struct {
	// EnableLeaderElection specifies whether to enable leader election for controller manager.
	EnableLeaderElection bool
	// LeaderElectionID is the name of the resource that leader election will use for holding the leader lock.
	LeaderElectionID string
	// LeaderElectionResourceLock specifies which resource type to use for leader election.
	LeaderElectionResourceLock string
}

type ManagerConfig struct {
	// MetricsAddr is the address the metric endpoint binds to.
	MetricsAddr string
	LeaderElectionConfig
	// DisableLeaseCache specifies whether to disable cache for lease.coordination.k8s.io resources.
	DisableLeaseCache bool
	// IgnoreOperationAnnotation specifies whether to ignore or honour the operation annotation on resources to be reconciled.
	IgnoreOperationAnnotation bool
	// EtcdControllerConfig is the configuration required for etcd controller.
	EtcdControllerConfig *etcd.Config
	// CustodianControllerConfig is the configuration required for custodian controller.
	CustodianControllerConfig *custodian.Config
	// CompactionControllerConfig is the configuration required for compaction controller.
	CompactionControllerConfig *compaction.Config
	// EtcdCopyBackupsTaskControllerConfig is the configuration required for etcd-copy-backup-tasks controller.
	EtcdCopyBackupsTaskControllerConfig *etcdcopybackupstask.Config
	// SecretControllerConfig is the configuration required for secret controller.
	SecretControllerConfig *secret.Config
}

func InitFromFlags(fs *flag.FlagSet, config *ManagerConfig) {
	flag.StringVar(&config.MetricsAddr, metricsAddrFlagName, defaultMetricsAddr, ""+
		"The address the metric endpoint binds to.")
	flag.BoolVar(&config.EnableLeaderElection, enableLeaderElectionFlagName, defaultEnableLeaderElection,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&config.LeaderElectionID, leaderElectionIDFlagName, defaultLeaderElectionID,
		"Name of the resource that leader election will use for holding the leader lock")
	flag.StringVar(&config.LeaderElectionResourceLock, leaderElectionResourceLockFlagName, defaultLeaderElectionResourceLock,
		"Specifies which resource type to use for leader election. Supported options are 'endpoints', 'configmaps', 'leases', 'endpointsleases' and 'configmapsleases'.")
	flag.BoolVar(&config.DisableLeaseCache, disableLeaseCacheFlagName, defaultDisableLeaseCache,
		"Disable cache for lease.coordination.k8s.io resources.")
	flag.BoolVar(&config.IgnoreOperationAnnotation, ignoreOperationAnnotationFlagName, defaultIgnoreOperationAnnotation,
		"Specifies whether to ignore or honour the operation annotation on resources to be reconciled.")

	etcd.InitFromFlags(fs, config.EtcdControllerConfig)
	custodian.InitFromFlags(fs, config.CustodianControllerConfig)
	compaction.InitFromFlags(fs, config.CompactionControllerConfig)
	etcdcopybackupstask.InitFromFlags(fs, config.EtcdCopyBackupsTaskControllerConfig)
	secret.InitFromFlags(fs, config.SecretControllerConfig)
}
