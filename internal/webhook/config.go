package webhook

import (
	"github.com/gardener/etcd-druid/internal/webhook/sentinel"

	flag "github.com/spf13/pflag"
	"k8s.io/component-base/featuregate"
)

// Config defines the configuration for etcd-druid webhooks.
type Config struct {
	// Sentinel is the configuration required for sentinel webhook.
	Sentinel *sentinel.Config
}

// InitFromFlags initializes the webhook config from the provided CLI flag set.
func (cfg *Config) InitFromFlags(fs *flag.FlagSet) {
	cfg.Sentinel = &sentinel.Config{}
	sentinel.InitFromFlags(fs, cfg.Sentinel)
}

// Validate validates the webhook config.
func (cfg *Config) Validate() error { return nil }

// CaptureFeatureActivations captures feature gate activations for every webhook configuration.
// Feature gates are captured only for webhooks that use feature gates.
func (cfg *Config) CaptureFeatureActivations(_ featuregate.MutableFeatureGate) {}

// AtLeaseOneEnabled returns true if at least one webhook is enabled.
// NOTE for contributors: For every new webhook, add a disjunction condition with the webhook's Enabled field.
func (cfg *Config) AtLeaseOneEnabled() bool {
	return cfg.Sentinel.Enabled
}
