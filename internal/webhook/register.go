// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	"github.com/gardener/etcd-druid/internal/webhook/etcdcomponentprotection"

	"golang.org/x/exp/slog"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Register registers all etcd-druid webhooks with the controller manager.
func Register(mgr ctrl.Manager, config druidconfigv1alpha1.WebhookConfiguration) error {
	// Add Etcd Components webhook to the manager
	if ptr.Deref(config.EtcdComponentProtection.Enabled, false) {
		etcdComponentsWebhook, err := etcdcomponentprotection.NewHandler(
			mgr,
			config.EtcdComponentProtection,
		)
		if err != nil {
			return err
		}
		slog.Info("Registering EtcdComponents Webhook with manager")
		return etcdComponentsWebhook.RegisterWithManager(mgr)
	}
	return nil
}
