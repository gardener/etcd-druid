// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"github.com/gardener/etcd-druid/internal/controller/utils"

	flag "github.com/spf13/pflag"
)

const (
	workersFlagName = "secret-workers"

	defaultWorkers = 10
)

// Config defines the configuration for the Secret Controller.
type Config struct {
	// Workers is the number of workers concurrently processing reconciliation requests.
	Workers int
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, workersFlagName, defaultWorkers,
		"Number of worker threads for the secrets controller.")
}

// Validate validates the config.
func (cfg *Config) Validate() error {
	return utils.MustBeGreaterThan(workersFlagName, 0, cfg.Workers)
}
