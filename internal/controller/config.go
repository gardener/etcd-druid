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

// ManagerConfig defines the configuration for the controller manager.
type ManagerConfig struct {
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
	// EtcdControllerConfig is the configuration required for etcd controller.
	EtcdControllerConfig *etcd.Config
	// CompactionControllerConfig is the configuration required for compaction controller.
	CompactionControllerConfig *compaction.Config
	// EtcdCopyBackupsTaskControllerConfig is the configuration required for etcd-copy-backup-tasks controller.
	EtcdCopyBackupsTaskControllerConfig *etcdcopybackupstask.Config
	// SecretControllerConfig is the configuration required for secret controller.
	SecretControllerConfig *secret.Config
	// SentinelWebhookConfig is the configuration required for sentinel webhook.
	SentinelWebhookConfig *sentinel.Config
}

// InitFromFlags initializes the controller manager config from the provided CLI flag set.
func (cfg *ManagerConfig) InitFromFlags(fs *flag.FlagSet) error {
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

	cfg.EtcdControllerConfig = &etcd.Config{}
	etcd.InitFromFlags(fs, cfg.EtcdControllerConfig)

	cfg.CompactionControllerConfig = &compaction.Config{}
	compaction.InitFromFlags(fs, cfg.CompactionControllerConfig)

	cfg.EtcdCopyBackupsTaskControllerConfig = &etcdcopybackupstask.Config{}
	etcdcopybackupstask.InitFromFlags(fs, cfg.EtcdCopyBackupsTaskControllerConfig)

	cfg.SecretControllerConfig = &secret.Config{}
	secret.InitFromFlags(fs, cfg.SecretControllerConfig)

	cfg.SentinelWebhookConfig = &sentinel.Config{}
	sentinel.InitFromFlags(fs, cfg.SentinelWebhookConfig)

	return nil
}

// initFeatureGates initializes feature gates from the provided CLI flag set.
func (cfg *ManagerConfig) initFeatureGates(fs *flag.FlagSet) error {
	featureGates := featuregate.NewFeatureGate()
	if err := featureGates.Add(features.GetDefaultFeatures()); err != nil {
		return fmt.Errorf("error adding features to the featuregate: %v", err)
	}
	featureGates.AddFlag(fs)

	cfg.FeatureGates = featureGates

	return nil
}

// populateControllersFeatureGates adds relevant feature gates to every controller configuration
func (cfg *ManagerConfig) populateControllersFeatureGates() {
	// Feature gates populated only for controllers that use feature gates

	// Add etcd controller feature gates
	cfg.EtcdControllerConfig.CaptureFeatureActivations(cfg.FeatureGates)

	// Add compaction controller feature gates
	cfg.CompactionControllerConfig.CaptureFeatureActivations(cfg.FeatureGates)

	// Add etcd-copy-backups-task controller feature gates
	cfg.EtcdCopyBackupsTaskControllerConfig.CaptureFeatureActivations(cfg.FeatureGates)
}

// Validate validates the controller manager config.
func (cfg *ManagerConfig) Validate() error {
	if err := utils.ShouldBeOneOfAllowedValues("ResourceLock", getAllowedLeaderElectionResourceLocks(), cfg.LeaderElection.ResourceLock); err != nil {
		return err
	}

	if cfg.SentinelWebhookConfig.Enabled {
		if cfg.Server.Webhook.Port == 0 {
			return fmt.Errorf("webhook port cannot be 0")
		}
		if cfg.Server.Webhook.TLSConfig.ServerCertDir == "" {
			return fmt.Errorf("webhook server cert dir cannot be empty")
		}
	}

	if err := cfg.EtcdControllerConfig.Validate(); err != nil {
		return err
	}

	if err := cfg.CompactionControllerConfig.Validate(); err != nil {
		return err
	}

	if err := cfg.EtcdCopyBackupsTaskControllerConfig.Validate(); err != nil {
		return err
	}

	return cfg.SecretControllerConfig.Validate()
}

// getAllowedLeaderElectionResourceLocks returns the allowed resource type to be used for leader election.
// TODO: This function should be removed as we have now marked 'leader-election-resource-lock' as deprecated.
// TODO: We will keep the validations till we have the CLI argument. Once that is removed we can also remove this function.
func getAllowedLeaderElectionResourceLocks() []string {
	return []string{
		"leases",
	}
}
