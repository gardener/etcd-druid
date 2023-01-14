// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package flags

import (
	"flag"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/gardener/etcd-druid/pkg/config"
)

const (
	// General flags
	flagMetricsAddr                        = "metrics-addr"
	flagEnableLeaderElection               = "enable-leader-election"
	flagLeaderElectionID                   = "leader-election-id"
	flagLeaderElectionResourceLock         = "leader-election-resource-lock"
	flagIgnoreOperationAnnotation          = "ignore-operation-annotation"
	flagDisableEtcdServiceAccountAutomount = "disable-etcd-serviceaccount-automount"
	flagDisableLeaseCache                  = "disable-lease-cache"

	// Etcd controller flags
	flagEtcdWorkers = "workers"

	// Custodian controller flags
	flagCustodianWorkers            = "custodian-workers"
	flagCustodianSyncPeriod         = "custodian-sync-period"
	flagEtcdMemberNotReadyThreshold = "etcd-member-notready-threshold"
	flagEtcdMemberUnknownThreshold  = "etcd-member-unknown-threshold"

	// Compaction lease controller flags
	flagEnableBackupCompaction = "enable-backup-compaction"
	flagCompactionWorkers      = "compaction-workers"
	flagEventsThreshold        = "etcd-events-threshold"
	flagActiveDeadlineDuration = "active-deadline-duration"

	// Secrets controller flags
	flagSecretWorkers = "secret-workers"

	// Etcd copy backups task controller flags
	flagEtcdCopyBackupsTaskWorkers = "etcd-copy-backups-task-workers"
)

func AddControllerManagerFlags(c *config.ControllerManagerConfig) {
	flag.StringVar(&c.MetricsAddr, flagMetricsAddr, c.MetricsAddr,
		"The address the metric endpoint binds to.")
	flag.BoolVar(&c.EnableLeaderElection, flagEnableLeaderElection, c.EnableLeaderElection,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&c.LeaderElectionID, flagLeaderElectionID, c.LeaderElectionID,
		"Name of the resource that leader election will use for holding the leader lock")
	flag.StringVar(&c.LeaderElectionResourceLock, flagLeaderElectionResourceLock, c.LeaderElectionResourceLock,
		"Specifies which resource type to use for leader election. Supported options are 'endpoints', 'configmaps', 'leases', 'endpointsleases' and 'configmapsleases'.")
	flag.BoolVar(&c.IgnoreOperationAnnotation, flagIgnoreOperationAnnotation, c.IgnoreOperationAnnotation,
		"Specifies whether to ignore or honour the operation annotation on resources to be reconciled.")
	flag.BoolVar(&c.DisableEtcdServiceAccountAutomount, flagDisableEtcdServiceAccountAutomount, c.DisableEtcdServiceAccountAutomount,
		"If true then .automountServiceAccountToken will be set to false for the ServiceAccount created for etcd statefulsets.")
	flag.BoolVar(&c.DisableLeaseCache, flagDisableLeaseCache, c.DisableLeaseCache,
		"Disable cache for lease.coordination.k8s.io resources.")
}

func AddEtcdControllerFlags(c *config.EtcdControllerConfig) {
	flag.IntVar(&c.Workers, flagEtcdWorkers, c.Workers,
		"Number of worker threads for the etcd controller.")
}

func AddCustodianControllerFlags(c *config.CustodianControllerConfig) {
	flag.IntVar(&c.Workers, flagCustodianWorkers, c.Workers,
		"Number of worker threads for the custodian controller.")
	flag.DurationVar(&c.SyncPeriod, flagCustodianSyncPeriod, c.SyncPeriod,
		"Sync period of the custodian controller.")
	flag.DurationVar(&c.EtcdMember.EtcdMemberNotReadyThreshold, flagEtcdMemberNotReadyThreshold, c.EtcdMember.EtcdMemberNotReadyThreshold,
		"Threshold after which an etcd member is considered not ready if the status was unknown before.")
	flag.DurationVar(&c.EtcdMember.EtcdMemberUnknownThreshold, flagEtcdMemberUnknownThreshold, c.EtcdMember.EtcdMemberUnknownThreshold,
		"Threshold after which an etcd member is considered unknown.")
}

func AddCompactionLeaseController(c *config.CompactionLeaseControllerConfig) {
	flag.BoolVar(&c.EnableBackupCompaction, flagEnableBackupCompaction, c.EnableBackupCompaction,
		"Enable automatic compaction of etcd backups.")
	flag.IntVar(&c.Workers, flagCompactionWorkers, c.Workers,
		"Number of worker threads for the compaction lease controller. Setting this flag to 0 disables the controller.")
	flag.Int64Var(&c.EventsThreshold, flagEventsThreshold, c.EventsThreshold,
		"Total number of etcd events to be reached upon which a backup compaction job is triggered.")
	flag.DurationVar(&c.ActiveDeadlineDuration, flagActiveDeadlineDuration, c.ActiveDeadlineDuration,
		"Duration after which a running backup compaction job will be killed.")
}

func AddSecretController(c *config.SecretControllerConfig) {
	flag.IntVar(&c.Workers, flagSecretWorkers, c.Workers,
		"Number of worker threads for the secrets controller.")
}

func AddEtcdCopyBackupsTaskController(c *config.EtcdCopyBackupsTaskControllerConfig) {
	flag.IntVar(&c.Workers, flagEtcdCopyBackupsTaskWorkers, c.Workers,
		"Number of worker threads for the EtcdCopyBackupsTask controller.")
}

func AddAllFlags(
	controllerManagerConfig *config.ControllerManagerConfig,
	etcdControllerConfig *config.EtcdControllerConfig,
	custodianControllerConfig *config.CustodianControllerConfig,
	compactionLeaseControllerConfig *config.CompactionLeaseControllerConfig,
	secretControllerConfig *config.SecretControllerConfig,
	etcdCopyBackupsTaskControllerConfig *config.EtcdCopyBackupsTaskControllerConfig,
) {
	AddControllerManagerFlags(controllerManagerConfig)
	AddEtcdControllerFlags(etcdControllerConfig)
	AddCustodianControllerFlags(custodianControllerConfig)
	AddCompactionLeaseController(compactionLeaseControllerConfig)
	AddSecretController(secretControllerConfig)
	AddEtcdCopyBackupsTaskController(etcdCopyBackupsTaskControllerConfig)
}

func ParseFlags() {
	flag.Parse()
}

func PrintFlags(logger logr.Logger) {
	var flagsToPrint string
	flag.VisitAll(func(f *flag.Flag) {
		flagsToPrint += fmt.Sprintf("%s: %s, ", f.Name, f.Value)
	})

	logger.Info(fmt.Sprintf("Running with flags: %s", flagsToPrint[:len(flagsToPrint)-2]))
}
