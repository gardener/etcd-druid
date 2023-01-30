package custodian

import (
	"flag"
	"time"
)

const (
	custodianWorkersFlagName            = "custodian-workers"
	custodianSyncPeriodFlagName         = "custodian-sync-period"
	etcdMemberNotReadyThresholdFlagName = "etcd-member-notready-threshold"
	etcdMemberUnknownThresholdFlagName  = "etcd-member-unknown-threshold"

	defaultCustodianWorkers            = 3
	defaultCustodianSyncPeriod         = 30 * time.Second
	defaultEtcdMemberNotReadyThreshold = 5 * time.Minute
	defaultEtcdMemberUnknownThreshold  = 1 * time.Minute
)

// Config contains configuration for the etcd custodian controller.
type Config struct {
	// Workers denotes the number of worker threads for the custodian controller.
	Workers int
	// SyncPeriod is the duration after which re-enqueuing happens.
	SyncPeriod time.Duration
	// EtcdMember holds configuration related to etcd members.
	EtcdMember EtcdMemberConfig
}

// EtcdMemberConfig holds configuration related to etcd members.
type EtcdMemberConfig struct {
	// NotReadyThreshold is the duration after which an etcd member's state is considered `NotReady`.
	NotReadyThreshold time.Duration
	// UnknownThreshold is the duration after which an etcd member's state is considered `Unknown`.
	UnknownThreshold time.Duration
}

func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, custodianWorkersFlagName, cfg.Workers,
		"Number of worker threads for the custodian controller.")
	fs.DurationVar(&cfg.SyncPeriod, custodianSyncPeriodFlagName, cfg.SyncPeriod,
		"Sync period of the custodian controller.")
	fs.DurationVar(&cfg.EtcdMember.NotReadyThreshold, etcdMemberNotReadyThresholdFlagName, cfg.EtcdMember.NotReadyThreshold,
		"Threshold after which an etcd member is considered not ready if the status was unknown before.")
	fs.DurationVar(&cfg.EtcdMember.UnknownThreshold, etcdMemberUnknownThresholdFlagName, cfg.EtcdMember.UnknownThreshold,
		"Threshold after which an etcd member is considered unknown.")
}
