// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"

	"k8s.io/utils/ptr"
)

// AtLeaseOneEnabled returns true if at least one webhook is enabled.
// NOTE for contributors: For every new webhook, add a disjunction condition with the webhook's Enabled field.
func AtLeaseOneEnabled(config druidconfigv1alpha1.WebhookConfiguration) bool {
	return ptr.Deref(config.EtcdComponentProtection.Enabled, false)
}
