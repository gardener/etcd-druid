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

package etcd

import (
	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/features"

	flag "github.com/spf13/pflag"
	"k8s.io/component-base/featuregate"
)

const (
	workersFlagName                            = "workers"
	disableEtcdServiceAccountAutomountFlagName = "disable-etcd-serviceaccount-automount"

	defaultWorkers                            = 3
	defaultDisableEtcdServiceAccountAutomount = false
)

// featureList holds the feature gate names that are relevant for the Etcd Controller.
var featureList = []featuregate.Feature{
	features.UseEtcdWrapper,
}

// Config defines the configuration for the Etcd Controller.
type Config struct {
	// Workers is the number of workers concurrently processing reconciliation requests.
	Workers int
	// DisableEtcdServiceAccountAutomount controls the auto-mounting of service account token for etcd statefulsets.
	DisableEtcdServiceAccountAutomount bool
	// FeatureGates contains the feature gates to be used by Etcd Controller.
	FeatureGates map[featuregate.Feature]bool
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, workersFlagName, defaultWorkers,
		"Number of worker threads of the etcd controller.")
	fs.BoolVar(&cfg.DisableEtcdServiceAccountAutomount, disableEtcdServiceAccountAutomountFlagName, defaultDisableEtcdServiceAccountAutomount,
		"If true then .automountServiceAccountToken will be set to false for the ServiceAccount created for etcd statefulsets.")
}

// Validate validates the config.
func (cfg *Config) Validate() error {
	return utils.MustBeGreaterThan(workersFlagName, 0, cfg.Workers)
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
