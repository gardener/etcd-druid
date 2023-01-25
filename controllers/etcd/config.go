package etcd

import "flag"

const (
	defaultWorkers                             = 3
	defaultDisabledEtcdServiceAccountAutomount = false
)

// Config defines the configuration for the etcd controller.
type Config struct {
	// Workers is the number of workers concurrently processing reconciliation requests.
	Workers int
	// DisableEtcdServiceAccountAutomount controls the auto-mounting of service account token for etcd stateful sets for etcd stateful sets for etcd stateful sets.
	DisableEtcdServiceAccountAutomount bool
}

func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, "workers", defaultWorkers,
		"Number of worker threads of the etcd controller.")
	fs.BoolVar(&cfg.DisableEtcdServiceAccountAutomount, "disable-etcd-serviceaccount-automount", defaultDisabledEtcdServiceAccountAutomount,
		"If true then .automountServiceAccountToken will be set to false for the ServiceAccount created for etcd statefulsets.")
}
