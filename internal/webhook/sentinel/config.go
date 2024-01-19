package sentinel

import flag "github.com/spf13/pflag"

const (
	enableSentinelWebhookFlagName = "enable-sentinel-webhook"

	defaultEnableSentinelWebhook = false
)

// Config defines the configuration for the Sentinel Webhook.
type Config struct {
	// Enabled indicates whether the Sentinel Webhook is enabled.
	Enabled bool
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.BoolVar(&cfg.Enabled, enableSentinelWebhookFlagName, defaultEnableSentinelWebhook,
		"Enable Sentinel Webhook to prevent unintended changes to resources managed by etcd-druid.")
}
