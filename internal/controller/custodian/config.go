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
	"github.com/gardener/etcd-druid/controllers/utils"

	flag "github.com/spf13/pflag"
)

const (
	workersFlagName = "custodian-workers"

	defaultCustodianWorkers = 3
)

// Config contains configuration for the Custodian Controller.
type Config struct {
	// Workers denotes the number of worker threads for the custodian controller.
	Workers int
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, workersFlagName, defaultCustodianWorkers,
		"Number of worker threads for the custodian controller.")
}

// Validate validates the config.
func (cfg *Config) Validate() error {
	return utils.MustBeGreaterThan(workersFlagName, 0, cfg.Workers)
}
