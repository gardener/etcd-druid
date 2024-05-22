package controller

import (
	"github.com/gardener/etcd-druid/internal/controller/compaction"
	"github.com/gardener/etcd-druid/internal/controller/etcd"
	"github.com/gardener/etcd-druid/internal/controller/etcdcopybackupstask"
	"github.com/gardener/etcd-druid/internal/controller/secret"

	flag "github.com/spf13/pflag"
	"k8s.io/component-base/featuregate"
)

// Config defines the configuration for etcd-druid controllers.
type Config struct {
	// Etcd is the configuration required for etcd controller.
	Etcd *etcd.Config
	// Compaction is the configuration required for compaction controller.
	Compaction *compaction.Config
	// EtcdCopyBackupsTask is the configuration required for etcd-copy-backup-tasks controller.
	EtcdCopyBackupsTask *etcdcopybackupstask.Config
	// Secret is the configuration required for secret controller.
	Secret *secret.Config
}

// InitFromFlags initializes the controller config from the provided CLI flag set.
func (cfg *Config) InitFromFlags(fs *flag.FlagSet) {
	cfg.Etcd = &etcd.Config{}
	cfg.Etcd.InitFromFlags(fs)

	cfg.Compaction = &compaction.Config{}
	cfg.Compaction.InitFromFlags(fs)

	cfg.EtcdCopyBackupsTask = &etcdcopybackupstask.Config{}
	cfg.EtcdCopyBackupsTask.InitFromFlags(fs)

	cfg.Secret = &secret.Config{}
	cfg.Secret.InitFromFlags(fs)
}

// Validate validates the controller config.
func (cfg *Config) Validate() error {
	if err := cfg.Etcd.Validate(); err != nil {
		return err
	}

	if err := cfg.Compaction.Validate(); err != nil {
		return err
	}

	if err := cfg.EtcdCopyBackupsTask.Validate(); err != nil {
		return err
	}

	return cfg.Secret.Validate()
}

// CaptureFeatureActivations captures feature gate activations for every controller configuration.
// Feature gates are captured only for controllers that use feature gates.
func (cfg *Config) CaptureFeatureActivations(featureGates featuregate.MutableFeatureGate) {
	// Add etcd controller feature gates
	cfg.Etcd.CaptureFeatureActivations(featureGates)

	// Add compaction controller feature gates
	cfg.Compaction.CaptureFeatureActivations(featureGates)

	// Add etcd-copy-backups-task controller feature gates
	cfg.EtcdCopyBackupsTask.CaptureFeatureActivations(featureGates)
}
