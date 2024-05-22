package webhook

import (
	"github.com/gardener/etcd-druid/internal/webhook/sentinel"

	"golang.org/x/exp/slog"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Register registers all etcd-druid webhooks with the controller manager.
func Register(mgr ctrl.Manager, config *Config) error {
	// Add sentinel webhook to the manager
	if config.Sentinel.Enabled {
		sentinelWebhook, err := sentinel.NewHandler(
			mgr,
			config.Sentinel,
		)
		if err != nil {
			return err
		}
		slog.Info("Registering Sentinel Webhook with manager")
		return sentinelWebhook.RegisterWithManager(mgr)
	}
	return nil
}
