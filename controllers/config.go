package controllers

import (
	"flag"

	"github.com/gardener/etcd-druid/controllers/secretcontroller"
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
	// DisableLeaseCache specifies whether to disable cache for lease.coordination.k8s.io resources
	DisableLeaseCache bool
	// IgnoreOperationAnnotation specifies whether to ignore or honour the operation annotation on resources to be reconciled.
	IgnoreOperationAnnotation bool
	SecretControllerConfig    *secretcontroller.Config
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
	secretcontroller.InitFromFlags(fs, config.SecretControllerConfig)
}
