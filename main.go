// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file

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

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/gardener/etcd-druid/controllers"
	controllersconfig "github.com/gardener/etcd-druid/controllers/config"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"

	coordinationv1 "k8s.io/api/coordination/v1"
	coordinationv1beta1 "k8s.io/api/coordination/v1beta1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

const (
	// General flags
	flagMetricsAddr                           = "metrics-addr"
	flagEnableLeaderElection                  = "enable-leader-election"
	flagLeaderElectionID                      = "leader-election-id"
	flagLeaderElectionResourceLock            = "leader-election-resource-lock"
	flagIgnoreOperationAnnotation             = "ignore-operation-annotation"
	flagDisableEtcdServiceAccountAutomount    = "disable-etcd-serviceaccount-automount"
	flagDisableLeaseCache                     = "disable-lease-cache"
	defaultMetricsAddr                        = ":8080"
	defaultEnableLeaderElection               = false
	defaultLeaderElectionID                   = "druid-leader-election"
	defaultLeaderElectionResourceLock         = resourcelock.LeasesResourceLock
	defaultIgnoreOperationAnnotation          = true
	defaultDisableEtcdServiceAccountAutomount = false
	defaultDisableLeaseCache                  = false

	// Etcd controller flags
	flagEtcdWorkers    = "workers"
	defaultEtcdWorkers = 3

	// Custodian controller flags
	flagCustodianWorkers               = "custodian-workers"
	flagCustodianSyncPeriod            = "custodian-sync-period"
	flagEtcdMemberNotReadyThreshold    = "etcd-member-notready-threshold"
	flagEtcdMemberUnknownThreshold     = "etcd-member-unknown-threshold"
	defaultCustodianWorkers            = 3
	defaultCustodianSyncPeriod         = 30 * time.Second
	defaultEtcdMemberNotReadyThreshold = 5 * time.Minute
	defaultEtcdMemberUnknownThreshold  = 1 * time.Minute

	// Compaction lease controller flags
	flagEnableBackupCompaction    = "enable-backup-compaction"
	flagCompactionWorkers         = "compaction-workers"
	flagEventsThreshold           = "etcd-events-threshold"
	flagActiveDeadlineDuration    = "active-deadline-duration"
	defaultEnableBackupCompaction = false
	defaultCompactionWorkers      = 3
	defaultEventsThreshold        = 1000000
	defaultActiveDeadlineDuration = 3 * time.Hour

	// Secrets controller flags
	flagSecretWorkers    = "secret-workers"
	defaultSecretWorkers = 10

	// Etcd copy backups task controller flags
	flagEtcdCopyBackupsTaskWorkers    = "etcd-copy-backups-task-workers"
	defaultEtcdCopyBackupsTaskWorkers = 3
)

var setupLog = ctrl.Log.WithName("setup")

func main() {
	var (
		metricsAddr                        string
		enableLeaderElection               bool
		leaderElectionID                   string
		leaderElectionResourceLock         string
		ignoreOperationAnnotation          bool
		disableEtcdServiceAccountAutomount bool
		disableLeaseCache                  bool

		etcdWorkers int

		custodianWorkers            int
		custodianSyncPeriod         time.Duration
		etcdMemberNotReadyThreshold time.Duration
		etcdMemberUnknownThreshold  time.Duration

		enableBackupCompaction bool
		compactionWorkers      int
		eventsThreshold        int64
		activeDeadlineDuration time.Duration

		secretWorkers int

		etcdCopyBackupsTaskWorkers int
	)

	// General flags
	flag.StringVar(&metricsAddr, flagMetricsAddr, defaultMetricsAddr, "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, flagEnableLeaderElection, defaultEnableLeaderElection, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionID, flagLeaderElectionID, defaultLeaderElectionID, fmt.Sprintf("Name of the resource that leader election will use for holding the leader lock. Defaults to '%s'.", defaultLeaderElectionID))
	flag.StringVar(&leaderElectionResourceLock, flagLeaderElectionResourceLock, defaultLeaderElectionResourceLock, "Which resource type to use for leader election. Supported options are 'endpoints', 'configmaps', 'leases', 'endpointsleases' and 'configmapsleases'.")
	flag.BoolVar(&ignoreOperationAnnotation, flagIgnoreOperationAnnotation, defaultIgnoreOperationAnnotation, "Ignore the operation annotation or not.")
	flag.BoolVar(&disableEtcdServiceAccountAutomount, flagDisableEtcdServiceAccountAutomount, defaultDisableEtcdServiceAccountAutomount, "If true then .automountServiceAccountToken will be set to false for the ServiceAccount created for etcd statefulsets.")
	flag.BoolVar(&disableLeaseCache, flagDisableLeaseCache, defaultDisableLeaseCache, "Disable cache for lease.coordination.k8s.io resources.")

	// Etcd controller flags
	flag.IntVar(&etcdWorkers, flagEtcdWorkers, defaultEtcdWorkers, "Number of worker threads of the etcd controller.")

	// Custodian controller flags
	flag.IntVar(&custodianWorkers, flagCustodianWorkers, defaultCustodianWorkers, "Number of worker threads of the custodian controller.")
	flag.DurationVar(&custodianSyncPeriod, flagCustodianSyncPeriod, defaultCustodianSyncPeriod, "Sync period of the custodian controller.")
	flag.DurationVar(&etcdMemberNotReadyThreshold, flagEtcdMemberNotReadyThreshold, defaultEtcdMemberNotReadyThreshold, "Threshold after which an etcd member is considered not ready if the status was unknown before.")
	flag.DurationVar(&etcdMemberUnknownThreshold, flagEtcdMemberUnknownThreshold, defaultEtcdMemberUnknownThreshold, "Threshold after which an etcd member is considered unknown.")

	// Compaction lease controller flags
	flag.BoolVar(&enableBackupCompaction, flagEnableBackupCompaction, defaultEnableBackupCompaction, "Enable automatic compaction of etcd backups.")
	flag.IntVar(&compactionWorkers, flagCompactionWorkers, defaultCompactionWorkers, "Number of worker threads of the CompactionJob controller. The controller creates a backup compaction job if a certain etcd event threshold is reached. Setting this flag to 0 disables the controller.")
	flag.Int64Var(&eventsThreshold, flagEventsThreshold, defaultEventsThreshold, "Total number of etcd events that can be allowed before a backup compaction job is triggered.")
	flag.DurationVar(&activeDeadlineDuration, flagActiveDeadlineDuration, defaultActiveDeadlineDuration, "Duration after which a running backup compaction job will be killed (Ex: \"300ms\", \"20s\", \"-1.5h\" or \"2h45m\").")

	// Secrets controller flags
	flag.IntVar(&secretWorkers, flagSecretWorkers, defaultSecretWorkers, "Number of worker threads of the secret controller.")

	// Etcd copy backups task controller flags
	flag.IntVar(&etcdCopyBackupsTaskWorkers, flagEtcdCopyBackupsTaskWorkers, defaultEtcdCopyBackupsTaskWorkers, "Number of worker threads of the EtcdCopyBackupsTask controller.")

	flag.Parse()

	ctrl.SetLogger(zap.New(buildDefaultLoggerOpts()...))

	setupLog.Info("Running with",
		flagMetricsAddr, metricsAddr,
		flagEnableLeaderElection, enableLeaderElection,
		flagLeaderElectionID, leaderElectionID,
		flagLeaderElectionResourceLock, leaderElectionResourceLock,
		flagIgnoreOperationAnnotation, ignoreOperationAnnotation,
		flagDisableEtcdServiceAccountAutomount, disableEtcdServiceAccountAutomount,
		flagDisableLeaseCache, disableLeaseCache,

		flagEtcdWorkers, etcdWorkers,
		flagCustodianWorkers, custodianWorkers,
		flagCustodianSyncPeriod, custodianSyncPeriod,
		flagEtcdMemberNotReadyThreshold, etcdMemberNotReadyThreshold,
		flagEtcdMemberUnknownThreshold, etcdMemberUnknownThreshold,

		flagEnableBackupCompaction, enableBackupCompaction,
		flagCompactionWorkers, compactionWorkers,
		flagEventsThreshold, eventsThreshold,
		flagActiveDeadlineDuration, activeDeadlineDuration,

		flagSecretWorkers, secretWorkers,

		flagEtcdCopyBackupsTaskWorkers, etcdCopyBackupsTaskWorkers,
	)

	ctx := ctrl.SetupSignalHandler()

	// TODO this can be removed once we have an improved informer, see https://github.com/gardener/etcd-druid/issues/215
	uncachedObjects := controllers.UncachedObjectList
	if disableLeaseCache {
		uncachedObjects = append(uncachedObjects, &coordinationv1.Lease{}, &coordinationv1beta1.Lease{})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		ClientDisableCacheFor:      uncachedObjects,
		Scheme:                     kubernetes.Scheme,
		MetricsBindAddress:         metricsAddr,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           leaderElectionID,
		LeaderElectionResourceLock: leaderElectionResourceLock,
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	etcd, err := controllers.NewEtcdReconcilerWithImageVector(mgr, disableEtcdServiceAccountAutomount)
	if err != nil {
		setupLog.Error(err, "Unable to initialize etcd controller with image vector")
		os.Exit(1)
	}

	if err := etcd.SetupWithManager(mgr, etcdWorkers, ignoreOperationAnnotation); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "Etcd")
		os.Exit(1)
	}

	secret := controllers.NewSecret(mgr)

	if err := secret.SetupWithManager(mgr, secretWorkers); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "Secret")
		os.Exit(1)
	}

	custodian := controllers.NewEtcdCustodian(mgr, controllersconfig.EtcdCustodianController{
		EtcdMember: controllersconfig.EtcdMemberConfig{
			EtcdMemberNotReadyThreshold: etcdMemberNotReadyThreshold,
			EtcdMemberUnknownThreshold:  etcdMemberUnknownThreshold,
		},
		SyncPeriod: custodianSyncPeriod,
	})

	if err := custodian.SetupWithManager(ctx, mgr, custodianWorkers, ignoreOperationAnnotation); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "Etcd Custodian")
		os.Exit(1)
	}

	etcdCopyBackupsTask, err := controllers.NewEtcdCopyBackupsTaskReconcilerWithImageVector(mgr)
	if err != nil {
		setupLog.Error(err, "Unable to initialize controller with image vector")
		os.Exit(1)
	}

	if err := etcdCopyBackupsTask.SetupWithManager(mgr, etcdCopyBackupsTaskWorkers); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "EtcdCopyBackupsTask")
	}

	lc, err := controllers.NewCompactionLeaseControllerWithImageVector(mgr, controllersconfig.CompactionLeaseConfig{
		CompactionEnabled:      enableBackupCompaction,
		EventsThreshold:        eventsThreshold,
		ActiveDeadlineDuration: activeDeadlineDuration,
	})

	if err != nil {
		setupLog.Error(err, "Unable to initialize lease controller")
		os.Exit(1)
	}

	if err := lc.SetupWithManager(mgr, compactionWorkers); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "Lease")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

func buildDefaultLoggerOpts() []zap.Opts {
	var opts []zap.Opts
	opts = append(opts, zap.UseDevMode(false))
	opts = append(opts, zap.JSONEncoder(func(encoderConfig *zapcore.EncoderConfig) {
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	}))
	return opts
}
