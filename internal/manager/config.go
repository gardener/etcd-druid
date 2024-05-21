// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	"github.com/gardener/etcd-druid/internal/controller/compaction"
	"github.com/gardener/etcd-druid/internal/controller/etcd"
	"github.com/gardener/etcd-druid/internal/controller/etcdcopybackupstask"
	"github.com/gardener/etcd-druid/internal/controller/secret"
	"github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/features"
	"github.com/gardener/etcd-druid/internal/webhook/sentinel"

	flag "github.com/spf13/pflag"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/component-base/featuregate"
)

const (
	metricsAddrFlagName                = "metrics-addr"
	metricsBindAddressFlagName         = "metrics-bind-address"
	metricsPortFlagName                = "metrics-port"
	webhookServerBindAddressFlagName   = "webhook-server-bind-address"
	webhookServerPortFlagName          = "webhook-server-port"
	webhookServerTLSServerCertDir      = "webhook-server-tls-server-cert-dir"
	enableLeaderElectionFlagName       = "enable-leader-election"
	leaderElectionIDFlagName           = "leader-election-id"
	leaderElectionResourceLockFlagName = "leader-election-resource-lock"
	disableLeaseCacheFlagName          = "disable-lease-cache"

	defaultMetricsAddr                = ""
	defaultMetricsBindAddress         = ""
	defaultMetricsPort                = 8080
	defaultWebhookServerBindAddress   = ""
	defaultWebhookServerPort          = 9443
	defaultWebhookServerTLSServerCert = "/etc/webhook-server-tls"
	defaultEnableLeaderElection       = false
	defaultLeaderElectionID           = "druid-leader-election"
	defaultLeaderElectionResourceLock = resourcelock.LeasesResourceLock
	defaultDisableLeaseCache          = false
)

// ServerConfig contains details for the HTTP(S) servers.
type ServerConfig struct {
	// Webhook is the configuration for the HTTPS webhook server.
	Webhook HTTPSServer
	// Metrics is the configuration for serving the metrics endpoint.
	Metrics *Server
}

// HTTPSServer is the configuration for the HTTPSServer server.
type HTTPSServer struct {
	// Server is the configuration for the bind address and the port.
	Server
	// TLSConfig contains information about the TLS configuration for an HTTPS server.
	TLSConfig TLSServerConfig
}

// TLSServerConfig contains information about the TLS configuration for an HTTPS server.
type TLSServerConfig struct {
	// ServerCertDir is the path to a directory containing the server's TLS certificate and key (the files must be
	// named tls.crt and tls.key respectively).
	ServerCertDir string
}

// Server contains information for HTTP(S) server configuration.
type Server struct {
	// BindAddress is the IP address on which to listen for the specified port.
	BindAddress string
	// Port is the port on which to serve unsecured, unauthenticated access.
	Port int
}

// LeaderElectionConfig defines the configuration for the leader election for the controller manager.
type LeaderElectionConfig struct {
	// Enabled specifies whether to enable leader election for controller manager.
	Enabled bool
	// ID is the name of the resource that leader election will use for holding the leader lock.
	ID string
	// ResourceLock specifies which resource type to use for leader election.
	// Deprecated: K8S Leases will be used for leader election. No other resource type would be permitted.
	// This configuration option will be removed eventually. It is advisable to not use this option any longer.
	ResourceLock string
}

// Config defines the configuration for the controller manager.
type Config struct {
	// MetricsAddr is the address the metric endpoint binds to.
	// Deprecated: This field will be eventually removed. Please use Server.Metrics.BindAddress instead.
	MetricsAddr string
	// Server is the configuration for the HTTP server.
	Server         *ServerConfig
	LeaderElection LeaderElectionConfig
	// DisableLeaseCache specifies whether to disable cache for lease.coordination.k8s.io resources.
	DisableLeaseCache bool
	// FeatureGates contains the feature gates to be used by etcd-druid.
	FeatureGates featuregate.MutableFeatureGate
	// Controllers defines the configuration for etcd-druid controllers.
	Controllers Controllers
	// Webhooks defines the configuration for etcd-druid webhooks.
	Webhooks Webhooks
}

// Controllers defines the configuration for etcd druid controllers.
type Controllers struct {
	// Etcd is the configuration required for etcd controller.
	Etcd *etcd.Config
	// Compaction is the configuration required for compaction controller.
	Compaction *compaction.Config
	// EtcdCopyBackupsTask is the configuration required for etcd-copy-backup-tasks controller.
	EtcdCopyBackupsTask *etcdcopybackupstask.Config
	// Secret is the configuration required for secret controller.
	Secret *secret.Config
}

// Webhooks defines the configuration for etcd-druid webhooks.
type Webhooks struct {
	// Sentinel is the configuration required for sentinel webhook.
	Sentinel *sentinel.Config
}

// AtLeaseOneEnabled returns true if at least one webhook is enabled.
// NOTE for contributors: For every new webhook, add a disjunction condition with the webhook's AtLeaseOneEnabled field.
func (w Webhooks) AtLeaseOneEnabled() bool {
	return w.Sentinel.Enabled
}

// InitFromFlags initializes the controller manager config from the provided CLI flag set.
func (cfg *Config) InitFromFlags(fs *flag.FlagSet) error {
	cfg.Server = &ServerConfig{}
	cfg.Server.Metrics = &Server{}
	cfg.Server.Webhook = HTTPSServer{}
	cfg.Server.Webhook.Server = Server{}

	flag.StringVar(&cfg.Server.Metrics.BindAddress, metricsBindAddressFlagName, defaultMetricsBindAddress,
		"The IP address that the metrics endpoint binds to.")
	flag.IntVar(&cfg.Server.Metrics.Port, metricsPortFlagName, defaultMetricsPort,
		"The port used for the metrics endpoint.")
	flag.StringVar(&cfg.Server.Metrics.BindAddress, metricsAddrFlagName, defaultMetricsAddr,
		fmt.Sprintf("The fully qualified address:port that the metrics endpoint binds to. Deprecated: this field will be eventually removed. Please use %s and %s instead.", metricsBindAddressFlagName, metricsPortFlagName))
	flag.StringVar(&cfg.Server.Webhook.Server.BindAddress, webhookServerBindAddressFlagName, defaultWebhookServerBindAddress,
		"The IP address on which to listen for the HTTPS webhook server.")
	flag.IntVar(&cfg.Server.Webhook.Server.Port, webhookServerPortFlagName, defaultWebhookServerPort,
		"The port on which to listen for the HTTPS webhook server.")
	flag.StringVar(&cfg.Server.Webhook.TLSConfig.ServerCertDir, webhookServerTLSServerCertDir, defaultWebhookServerTLSServerCert,
		"The path to a directory containing the server's TLS certificate and key (the files must be named tls.crt and tls.key respectively).")
	flag.BoolVar(&cfg.LeaderElection.Enabled, enableLeaderElectionFlagName, defaultEnableLeaderElection,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&cfg.LeaderElection.ID, leaderElectionIDFlagName, defaultLeaderElectionID,
		"Name of the resource that leader election will use for holding the leader lock.")
	flag.StringVar(&cfg.LeaderElection.ResourceLock, leaderElectionResourceLockFlagName, defaultLeaderElectionResourceLock,
		"Specifies which resource type to use for leader election. Supported options are 'endpoints', 'configmaps', 'leases', 'endpointsleases' and 'configmapsleases'. Deprecated: will be removed in the future in favour of using only `leases` as the leader election resource lock for the controller manager.")
	flag.BoolVar(&cfg.DisableLeaseCache, disableLeaseCacheFlagName, defaultDisableLeaseCache,
		"Disable cache for lease.coordination.k8s.io resources.")

	if err := cfg.initFeatureGates(fs); err != nil {
		return err
	}

	cfg.Controllers.Etcd = &etcd.Config{}
	etcd.InitFromFlags(fs, cfg.Controllers.Etcd)

	cfg.Controllers.Compaction = &compaction.Config{}
	compaction.InitFromFlags(fs, cfg.Controllers.Compaction)

	cfg.Controllers.EtcdCopyBackupsTask = &etcdcopybackupstask.Config{}
	etcdcopybackupstask.InitFromFlags(fs, cfg.Controllers.EtcdCopyBackupsTask)

	cfg.Controllers.Secret = &secret.Config{}
	secret.InitFromFlags(fs, cfg.Controllers.Secret)

	cfg.Webhooks.Sentinel = &sentinel.Config{}
	sentinel.InitFromFlags(fs, cfg.Webhooks.Sentinel)

	return nil
}

// initFeatureGates initializes feature gates from the provided CLI flag set.
func (cfg *Config) initFeatureGates(fs *flag.FlagSet) error {
	featureGates := featuregate.NewFeatureGate()
	if err := featureGates.Add(features.GetDefaultFeatures()); err != nil {
		return fmt.Errorf("error adding features to the featuregate: %v", err)
	}
	featureGates.AddFlag(fs)

	cfg.FeatureGates = featureGates

	return nil
}

// populateControllersFeatureGates adds relevant feature gates to every controller configuration
func (cfg *Config) populateControllersFeatureGates() {
	// Feature gates populated only for controllers that use feature gates

	// Add etcd controller feature gates
	cfg.Controllers.Etcd.CaptureFeatureActivations(cfg.FeatureGates)

	// Add compaction controller feature gates
	cfg.Controllers.Compaction.CaptureFeatureActivations(cfg.FeatureGates)

	// Add etcd-copy-backups-task controller feature gates
	cfg.Controllers.EtcdCopyBackupsTask.CaptureFeatureActivations(cfg.FeatureGates)
}

// Validate validates the controller manager config.
func (cfg *Config) Validate() error {
	if err := utils.ShouldBeOneOfAllowedValues("ResourceLock", getAllowedLeaderElectionResourceLocks(), cfg.LeaderElection.ResourceLock); err != nil {
		return err
	}

	if cfg.Webhooks.Sentinel.Enabled {
		if cfg.Server.Webhook.Port == 0 {
			return fmt.Errorf("webhook port cannot be 0")
		}
		if cfg.Server.Webhook.TLSConfig.ServerCertDir == "" {
			return fmt.Errorf("webhook server cert dir cannot be empty")
		}
	}

	if err := cfg.Controllers.Etcd.Validate(); err != nil {
		return err
	}

	if err := cfg.Controllers.Compaction.Validate(); err != nil {
		return err
	}

	if err := cfg.Controllers.EtcdCopyBackupsTask.Validate(); err != nil {
		return err
	}

	return cfg.Controllers.Secret.Validate()
}

// getAllowedLeaderElectionResourceLocks returns the allowed resource type to be used for leader election.
// TODO: This function should be removed as we have now marked 'leader-election-resource-lock' as deprecated.
// TODO: We will keep the validations till we have the CLI argument. Once that is removed we can also remove this function.
func getAllowedLeaderElectionResourceLocks() []string {
	return []string{
		"leases",
	}
}
