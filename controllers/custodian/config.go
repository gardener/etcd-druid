// Copyright 2023 SAP SE or an SAP affiliate company
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

package custodian

import (
	"time"

	"github.com/gardener/etcd-druid/controllers/utils"

	flag "github.com/spf13/pflag"
)

const (
	workersFlagName                     = "custodian-workers"
	syncPeriodFlagName                  = "custodian-sync-period"
	requeueIntervalFlagName             = "custodian-requeue-interval"
	etcdMemberNotReadyThresholdFlagName = "etcd-member-notready-threshold"
	etcdMemberUnknownThresholdFlagName  = "etcd-member-unknown-threshold"

	defaultCustodianWorkers            = 3
	defaultCustodianRequeueInterval    = 30 * time.Second
	defaultEtcdMemberNotReadyThreshold = 5 * time.Minute
	defaultEtcdMemberUnknownThreshold  = 1 * time.Minute
)

// Config contains configuration for the Custodian Controller.
type Config struct {
	// Workers denotes the number of worker threads for the custodian controller.
	Workers int
	// SyncPeriod is the duration after which re-enqueuing happens.
	// Deprecated: this field will be removed in the future. Please use RequeueInterval instead.
	// TODO (shreyas-s-rao): remove this in druid v0.22
	SyncPeriod time.Duration
	// RequeueInterval is the interval for custodian controller to requeue reconciliations.
	RequeueInterval time.Duration
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

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, workersFlagName, defaultCustodianWorkers,
		"Number of worker threads for the custodian controller.")
	// TODO (shreyas-s-rao): remove this in druid v0.22
	fs.DurationVar(&cfg.RequeueInterval, syncPeriodFlagName, defaultCustodianRequeueInterval,
		"Sync period of the custodian controller.\n"+
			"This flag is deprecated, and will be removed in the future. Please use --"+requeueIntervalFlagName+" instead.\n"+
			"If both --"+syncPeriodFlagName+" and --"+requeueIntervalFlagName+" are specified, the one provided later in the flags to druid, will be considered final.")
	fs.DurationVar(&cfg.RequeueInterval, requeueIntervalFlagName, cfg.RequeueInterval,
		"Requeue interval for custodian controller to requeue reconciliations.\n"+
			"If both --"+requeueIntervalFlagName+" and --"+syncPeriodFlagName+" are specified, the one provided later in the flags to druid, will be considered final.")
	fs.DurationVar(&cfg.EtcdMember.NotReadyThreshold, etcdMemberNotReadyThresholdFlagName, defaultEtcdMemberNotReadyThreshold,
		"Threshold after which an etcd member is considered not ready if the status was unknown before.")
	fs.DurationVar(&cfg.EtcdMember.UnknownThreshold, etcdMemberUnknownThresholdFlagName, defaultEtcdMemberUnknownThreshold,
		"Threshold after which an etcd member is considered unknown.")
}

// Validate validates the config.
func (cfg *Config) Validate() error {
	if err := utils.MustBeGreaterThan(workersFlagName, 0, cfg.Workers); err != nil {
		return err
	}
	if err := utils.MustBeGreaterThan(requeueIntervalFlagName, 0, cfg.RequeueInterval); err != nil {
		return err
	}
	if err := utils.MustBeGreaterThan(etcdMemberNotReadyThresholdFlagName, 0, cfg.EtcdMember.NotReadyThreshold); err != nil {
		return err
	}
	return utils.MustBeGreaterThan(etcdMemberUnknownThresholdFlagName, 0, cfg.EtcdMember.UnknownThreshold)
}
