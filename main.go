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
	"os"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers"
	controllersconfig "github.com/gardener/etcd-druid/controllers/config"

	coordinationv1 "k8s.io/api/coordination/v1"
	coordinationv1beta1 "k8s.io/api/coordination/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	schemev1 "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(schemev1.AddToScheme(scheme))
	utilruntime.Must(druidv1alpha1.AddToScheme(scheme))

	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr                string
		enableLeaderElection       bool
		leaderElectionID           string
		leaderElectionResourceLock string
		etcdWorkers                int
		custodianWorkers           int
		etcdCopyBackupsTaskWorkers int
		custodianSyncPeriod        time.Duration
		disableLeaseCache          bool
		compactionWorkers          int
		eventsThreshold            int64
		activeDeadlineDuration     time.Duration
		ignoreOperationAnnotation  bool

		etcdMemberNotReadyThreshold time.Duration

		// TODO: migrate default to `leases` in one of the next releases
		defaultLeaderElectionResourceLock = resourcelock.ConfigMapsLeasesResourceLock
		defaultLeaderElectionID           = "druid-leader-election"
	)

	flag.IntVar(&etcdWorkers, "workers", 3, "Number of worker threads of the etcd controller.")
	flag.IntVar(&custodianWorkers, "custodian-workers", 3, "Number of worker threads of the custodian controller.")
	flag.IntVar(&etcdCopyBackupsTaskWorkers, "etcd-copy-backups-task-workers", 3, "Number of worker threads of the EtcdCopyBackupsTask controller.")
	flag.DurationVar(&custodianSyncPeriod, "custodian-sync-period", 30*time.Second, "Sync period of the custodian controller.")
	flag.IntVar(&compactionWorkers, "compaction-workers", 3, "Number of worker threads of the CompactionJob controller. The controller creates a backup compaction job if a certain etcd event threshold is reached. Setting this flag to 0 disabled the controller.")
	flag.Int64Var(&eventsThreshold, "etcd-events-threshold", 1000000, "Total number of etcd events that can be allowed before a backup compaction job is triggered.")
	flag.DurationVar(&activeDeadlineDuration, "active-deadline-duration", 3*time.Hour, "Duration after which a running backup compaction job will be killed (Ex: \"300ms\", \"20s\", \"-1.5h\" or \"2h45m\").")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionID, "leader-election-id", defaultLeaderElectionID, "Name of the resource that leader election will use for holding the leader lock. "+
		"Defaults to 'druid-leader-election'.")
	flag.StringVar(&leaderElectionResourceLock, "leader-election-resource-lock", defaultLeaderElectionResourceLock, "Which resource type to use for leader election. "+
		"Supported options are 'endpoints', 'configmaps', 'leases', 'endpointsleases' and 'configmapsleases'.")
	flag.BoolVar(&disableLeaseCache, "disable-lease-cache", false, "Disable cache for lease.coordination.k8s.io resources.")
	flag.BoolVar(&ignoreOperationAnnotation, "ignore-operation-annotation", true, "Ignore the operation annotation or not.")
	flag.DurationVar(&etcdMemberNotReadyThreshold, "etcd-member-notready-threshold", 5*time.Minute, "Threshold after which an etcd member is considered not ready if the status was unknown before.")

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	ctx := ctrl.SetupSignalHandler()

	// TODO this can be removed once we have an improved informer, see https://github.com/gardener/etcd-druid/issues/215
	uncachedObjects := controllers.UncachedObjectList
	if disableLeaseCache {
		uncachedObjects = append(uncachedObjects, &coordinationv1.Lease{}, &coordinationv1beta1.Lease{})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		ClientDisableCacheFor:      uncachedObjects,
		Scheme:                     scheme,
		MetricsBindAddress:         metricsAddr,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           leaderElectionID,
		LeaderElectionResourceLock: leaderElectionResourceLock,
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	etcd, err := controllers.NewEtcdReconcilerWithImageVector(mgr)
	if err != nil {
		setupLog.Error(err, "Unable to initialize etcd controller with image vector")
		os.Exit(1)
	}

	if err := etcd.SetupWithManager(mgr, etcdWorkers, ignoreOperationAnnotation); err != nil {
		setupLog.Error(err, "Unable to create controller", "Controller", "Etcd")
		os.Exit(1)
	}

	custodian := controllers.NewEtcdCustodian(mgr, controllersconfig.EtcdCustodianController{
		EtcdMember: controllersconfig.EtcdMemberConfig{
			EtcdMemberNotReadyThreshold: etcdMemberNotReadyThreshold,
		},
		SyncPeriod: custodianSyncPeriod,
	})

	if err := custodian.SetupWithManager(ctx, mgr, custodianWorkers); err != nil {
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
