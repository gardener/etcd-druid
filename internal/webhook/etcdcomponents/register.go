// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdcomponents

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// handlerName is the name of the webhook handler.
	handlerName = "etcd-components-webhook"
	// WebhookPath is the path at which the handler should be registered.
	webhookPath = "/webhooks/etcdcomponents"
)

// RegisterWithManager registers Handler to the given manager.
func (h *Handler) RegisterWithManager(mgr manager.Manager) error {
	webhook := &admission.Webhook{
		Handler:      h,
		RecoverPanic: true,
	}

	mgr.GetWebhookServer().Register(webhookPath, webhook)
	return nil
}
