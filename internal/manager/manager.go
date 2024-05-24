// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	druidcontroller "github.com/gardener/etcd-druid/internal/controller"
	druidwebhook "github.com/gardener/etcd-druid/internal/webhook"

	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"golang.org/x/exp/slog"
	coordinationv1 "k8s.io/api/coordination/v1"
	coordinationv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	defaultTimeout = time.Minute
)

// InitializeManager creates a controller manager and adds all the controllers and webhooks to the controller-manager using the passed in Config.
func InitializeManager(config *Config) (ctrl.Manager, error) {
	var (
		err error
		mgr ctrl.Manager
	)
	config.captureFeatureActivations()
	if mgr, err = createManager(config); err != nil {
		return nil, err
	}
	slog.Info("registering controllers and webhooks with manager")
	time.Sleep(10 * time.Second)
	if err = druidcontroller.Register(mgr, config.Controllers); err != nil {
		return nil, err
	}
	if err = druidwebhook.Register(mgr, config.Webhooks); err != nil {
		return nil, err
	}
	if err = registerHealthAndReadyEndpoints(mgr, config); err != nil {
		return nil, err
	}
	return mgr, nil
}

func createManager(config *Config) (ctrl.Manager, error) {
	// TODO: this can be removed once we have an improved informer, see https://github.com/gardener/etcd-druid/issues/215
	// list of objects which should not be cached.
	uncachedObjects := []client.Object{
		&corev1.Event{},
		&eventsv1beta1.Event{},
		&eventsv1.Event{},
	}

	if config.DisableLeaseCache {
		uncachedObjects = append(uncachedObjects, &coordinationv1.Lease{}, &coordinationv1beta1.Lease{})
	}

	// TODO: remove this check once `--metrics-addr` flag is removed, and directly compute the address:port when setting managerOptions.Metrics.BindAddress
	if !strings.Contains(config.Server.Metrics.BindAddress, ":") {
		config.Server.Metrics.BindAddress = net.JoinHostPort(config.Server.Metrics.BindAddress, strconv.Itoa(config.Server.Metrics.Port))
	}

	return ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: uncachedObjects,
			},
		},
		Scheme: kubernetes.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: config.Server.Metrics.BindAddress,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    config.Server.Webhook.BindAddress,
			Port:    config.Server.Webhook.Port,
			CertDir: config.Server.Webhook.TLSConfig.ServerCertDir,
		}),
		LeaderElection:             config.LeaderElection.Enabled,
		LeaderElectionID:           config.LeaderElection.ID,
		LeaderElectionResourceLock: config.LeaderElection.ResourceLock,
	})
}

func registerHealthAndReadyEndpoints(mgr ctrl.Manager, config *Config) error {
	slog.Info("Registering ping health check endpoint")
	// Add a health check which always returns true when it is checked
	if err := mgr.AddHealthzCheck("ping", func(req *http.Request) error { return nil }); err != nil {
		return err
	}
	// Add a readiness check which will pass only when all informers have synced.
	if err := mgr.AddReadyzCheck("informer-sync", func(req *http.Request) error {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return errors.New("informers not synced yet")
		}
		return nil
	}); err != nil {
		return err
	}
	// Add a readiness check for the webhook server
	if config.Webhooks.AtLeaseOneEnabled() {
		slog.Info("Registering webhook-server readiness check endpoint")
		if err := mgr.AddReadyzCheck("webhook-server", mgr.GetWebhookServer().StartedChecker()); err != nil {
			return err
		}
	}
	return nil
}
