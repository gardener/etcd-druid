package sentinel

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// handlerName is the name of the webhook handler.
	handlerName = "sentinel-webhook"
	// WebhookPath is the path at which the handler should be registered.
	webhookPath = "/webhooks/sentinel"
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
