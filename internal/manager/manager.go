// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	"k8s.io/utils/ptr"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	druidcontroller "github.com/gardener/etcd-druid/internal/controller"
	druidwebhook "github.com/gardener/etcd-druid/internal/webhook"
	druidwebhookutils "github.com/gardener/etcd-druid/internal/webhook/utils"

	"golang.org/x/exp/slog"
	coordinationv1 "k8s.io/api/coordination/v1"
	coordinationv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// InitializeManager creates a controller manager and adds all the controllers and webhooks to the controller-manager using the passed in Config.
func InitializeManager(config *configv1alpha1.OperatorConfiguration) (ctrl.Manager, error) {
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

func createManager(config *configv1alpha1.OperatorConfiguration) (ctrl.Manager, error) {
	// TODO: this can be removed once we have an improved informer, see https://github.com/gardener/etcd-druid/issues/215
	// list of objects which should not be cached.
	uncachedObjects := []client.Object{
		&corev1.Event{},
		&eventsv1beta1.Event{},
		&eventsv1.Event{},
	}

	if config.Controllers.DisableLeaseCache {
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
		LeaderElection:                config.LeaderElection.Enabled,
		LeaderElectionID:              config.LeaderElection.ResourceName,
		LeaderElectionResourceLock:    config.LeaderElection.ResourceLock,
		LeaderElectionReleaseOnCancel: true,
		LeaseDuration:                 &config.LeaderElection.LeaseDuration.Duration,
		RenewDeadline:                 &config.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:                   &config.LeaderElection.RetryPeriod.Duration,
		Controller: ctrlconfig.Controller{
			RecoverPanic: ptr.To(true),
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    config.Server.Webhook.BindAddress,
			Port:    config.Server.Webhook.Port,
			CertDir: config.Server.Webhook.TLSConfig.ServerCertDir,
		}),
	})
}

func registerHealthAndReadyEndpoints(mgr ctrl.Manager, config *configv1alpha1.OperatorConfiguration) error {
	slog.Info("Registering ping health check endpoint")
	// Add a health check which always returns true when it is checked
	if err := mgr.AddHealthzCheck("ping", func(_ *http.Request) error { return nil }); err != nil {
		return err
	}

	// Add a readiness check which will pass only when all informers have synced.
	// Typically one would call `HasSync` but that is not exposed out of controller-runtime `cache.Informers`. Instead,
	// give it a context with a very short timeout so that it causes the call to ` cache.WaitForCacheSync` to get executed once.
	// We do not wish to wait longer as the readiness checks should be fast. Once all the cache informers have synced then the
	// readiness check would succeed.
	if err := mgr.AddReadyzCheck("informer-sync", func(_ *http.Request) error {
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
	if druidwebhookutils.AtLeaseOneEnabled(config.Webhooks) {
		slog.Info("Registering webhook-server readiness check endpoint")
		if err := mgr.AddReadyzCheck("webhook-server", mgr.GetWebhookServer().StartedChecker()); err != nil {
			return err
		}
	}
	return nil
}
