package utils

import configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"

// AtLeaseOneEnabled returns true if at least one webhook is enabled.
// NOTE for contributors: For every new webhook, add a disjunction condition with the webhook's Enabled field.
func AtLeaseOneEnabled(config configv1alpha1.WebhookConfiguration) bool {
	return config.EtcdComponentProtection.Enabled
}
