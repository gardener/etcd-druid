// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package sentinel

import (
	"fmt"

	flag "github.com/spf13/pflag"
)

const (
	enableSentinelWebhookFlagName    = "enable-sentinel-webhook"
	reconcilerServiceAccountFlagName = "reconciler-service-account"
	exemptServiceAccountsFlagName    = "sentinel-exempt-service-accounts"

	defaultEnableSentinelWebhook    = false
	defaultReconcilerServiceAccount = "system:serviceaccount:default:etcd-druid"
)

var (
	defaultExemptServiceAccounts []string
)

// Config defines the configuration for the Sentinel Webhook.
type Config struct {
	// Enabled indicates whether the Sentinel Webhook is enabled.
	Enabled bool
	// ReconcilerServiceAccount is the name of the service account used by etcd-druid for reconciling etcd resources.
	ReconcilerServiceAccount string
	// ExemptServiceAccounts is a list of service accounts that are exempt from Sentinel Webhook checks.
	ExemptServiceAccounts []string
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.BoolVar(&cfg.Enabled, enableSentinelWebhookFlagName, defaultEnableSentinelWebhook,
		"Enable Sentinel Webhook to prevent unintended changes to resources managed by etcd-druid.")
	fs.StringVar(&cfg.ReconcilerServiceAccount, reconcilerServiceAccountFlagName, defaultReconcilerServiceAccount,
		fmt.Sprintf("The fully qualified name of the service account used by etcd-druid for reconciling etcd resources. Default: %s", defaultReconcilerServiceAccount))
	fs.StringSliceVar(&cfg.ExemptServiceAccounts, exemptServiceAccountsFlagName, defaultExemptServiceAccounts,
		"The comma-separated list of fully qualified names of service accounts that are exempt from Sentinel Webhook checks.")
}
