package secretcontroller

import (
	"flag"
)

const (
	workersFlagName = "secret-workers"
	defaultWorkers  = 10
)

// Config defines the configuration for the SecretController.
type Config struct {
	// Workers is the number of workers concurrently processing reconciliation requests.
	Workers int
}

func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, workersFlagName, defaultWorkers, "Number of worker threads for the secrets controller.")
}
