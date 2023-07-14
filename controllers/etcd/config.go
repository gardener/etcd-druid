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
	"flag"

	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/features"
)

const (
	workersFlagName                            = "workers"
	disableEtcdServiceAccountAutomountFlagName = "disable-etcd-serviceaccount-automount"

	defaultWorkers                            = 3
	defaultDisableEtcdServiceAccountAutomount = false
)

// RelevantFeatures holds the feature flag names that are relevant for the Etcd Controller.
// TODO: come up with better name for this variable
// TODO: write getter method on Config struct for this var
var RelevantFeatures = []string{
	string(features.UseEtcdWrapper),
}

// Config defines the configuration for the Etcd Controller.
type Config struct {
	// Workers is the number of workers concurrently processing reconciliation requests.
	Workers int
	// DisableEtcdServiceAccountAutomount controls the auto-mounting of service account token for etcd statefulsets.
	DisableEtcdServiceAccountAutomount bool
	// FeatureFlags contains the feature flags to be used by Etcd Controller.
	// TODO: set this from managerConfig (filtered using RelevantFeatures) before running the reconciler
	FeatureFlags map[string]bool
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
	if err := utils.MustBeGreaterThan(workersFlagName, 0, cfg.Workers); err != nil {
		return err
	}
	return nil
}
