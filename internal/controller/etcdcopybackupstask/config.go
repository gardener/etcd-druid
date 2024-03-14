// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdcopybackupstask

import (
	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/features"

	flag "github.com/spf13/pflag"
	"k8s.io/component-base/featuregate"
)

// featureList holds the feature gate names that are relevant for the EtcdCopyBackupTask Controller.
var featureList = []featuregate.Feature{
	features.UseEtcdWrapper,
}

const (
	workersFlagName = "etcd-copy-backups-task-workers"

	defaultWorkers = 3
)

// Config defines the configuration for the EtcdCopyBackupsTask Controller.
type Config struct {
	// Workers is the number of workers concurrently processing reconciliation requests.
	Workers int
	// FeatureGates contains the feature gates to be used by EtcdCopyBackupTask Controller.
	FeatureGates map[featuregate.Feature]bool
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, workersFlagName, defaultWorkers,
		"Number of worker threads for the etcdcopybackupstask controller.")
}

// Validate validates the config.
func (cfg *Config) Validate() error {
	return utils.MustBeGreaterThanOrEqualTo(workersFlagName, 0, cfg.Workers)
}

// CaptureFeatureActivations captures all feature gates required by the controller into controller config
func (cfg *Config) CaptureFeatureActivations(fg featuregate.FeatureGate) {
	if cfg.FeatureGates == nil {
		cfg.FeatureGates = make(map[featuregate.Feature]bool)
	}
	for _, feature := range featureList {
		cfg.FeatureGates[feature] = fg.Enabled(feature)
	}
}
