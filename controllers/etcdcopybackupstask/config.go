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
	FeatureGates map[string]bool
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, workersFlagName, defaultWorkers,
		"Number of worker threads for the etcdcopybackupstask controller.")
}

// Validate validates the config.
func (cfg *Config) Validate() error {
	if err := utils.MustBeGreaterThanOrEqualTo(workersFlagName, 0, cfg.Workers); err != nil {
		return err
	}
	return nil
}

// GetFeatureList returns feature gates relevant to the EtcdCopyBackupTask controller
func (cfg *Config) GetFeatureList() []featuregate.Feature {
	return featureList
}

// CaptureFeatureActivations captures all feature gates required by the controller into controller config
func (cfg *Config) CaptureFeatureActivations(fg featuregate.FeatureGate) {
	if cfg.FeatureGates == nil {
		cfg.FeatureGates = make(map[string]bool)
	}
	for _, feature := range cfg.GetFeatureList() {
		cfg.FeatureGates[string(feature)] = fg.Enabled(feature)
	}
}
