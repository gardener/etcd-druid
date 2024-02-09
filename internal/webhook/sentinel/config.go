package sentinel

import (
	"fmt"
	flag "github.com/spf13/pflag"
)

const (
	enableSentinelWebhookFlagName    = "enable-sentinel-webhook"
	reconcilerServiceAccountFlagName = "reconciler-service-account"

	defaultEnableSentinelWebhook    = false
	defaultReconcilerServiceAccount = "system:serviceaccount:default:etcd-druid"
)

// Config defines the configuration for the Sentinel Webhook.
type Config struct {
	// Enabled indicates whether the Sentinel Webhook is enabled.
	Enabled bool
	// ReconcilerServiceAccount is the name of the service account used by etcd-druid for reconciling etcd resources.
	ReconcilerServiceAccount string
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.BoolVar(&cfg.Enabled, enableSentinelWebhookFlagName, defaultEnableSentinelWebhook,
		"Enable Sentinel Webhook to prevent unintended changes to resources managed by etcd-druid.")
	fs.StringVar(&cfg.ReconcilerServiceAccount, reconcilerServiceAccountFlagName, defaultReconcilerServiceAccount,
		fmt.Sprintf("The fully qualified name of the service account used by etcd-druid for reconciling etcd resources. Default: %s", defaultReconcilerServiceAccount))
}
