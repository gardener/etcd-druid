// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var (
	zeroDuration = metav1.Duration{}
)

const (
	// DefaultLeaderElectionResourceLock is the default resource lock type for leader election.
	DefaultLeaderElectionResourceLock = "leases"
	// DefaultLeaderElectionResourceName is the default resource name for leader election.
	DefaultLeaderElectionResourceName = "druid-leader-election"
	// DefaultLeaderElectionLeaseDuration is the default lease duration for leader election.
	DefaultLeaderElectionLeaseDuration = 15 * time.Second
	// DefaultLeaderElectionRenewDeadline is the default renew deadline for leader election.j
	DefaultLeaderElectionRenewDeadline = 10 * time.Second
	// DefaultLeaderElectionRetryPeriod is the default retry period for leader election.
	DefaultLeaderElectionRetryPeriod = 2 * time.Second
)

// SetDefaults_LeaderElectionConfiguration sets defaults for leader election configuration.
// Borrowed defaults from: https://github.com/kubernetes/component-base/blob/master/config/v1alpha1/defaults.go
func SetDefaults_LeaderElectionConfiguration(leaderElectionConfig *LeaderElectionConfiguration) {
	// Set default values for leader election configuration if it has been enabled.
	if !leaderElectionConfig.Enabled {
		return
	}
	if leaderElectionConfig.ResourceLock == "" {
		leaderElectionConfig.ResourceLock = DefaultLeaderElectionResourceLock
	}
	if leaderElectionConfig.ResourceName == "" {
		leaderElectionConfig.ResourceName = DefaultLeaderElectionResourceName
	}
	if leaderElectionConfig.LeaseDuration == zeroDuration {
		leaderElectionConfig.LeaseDuration = metav1.Duration{Duration: DefaultLeaderElectionLeaseDuration}
	}
	if leaderElectionConfig.RenewDeadline == zeroDuration {
		leaderElectionConfig.RenewDeadline = metav1.Duration{Duration: DefaultLeaderElectionRenewDeadline}
	}
	if leaderElectionConfig.RetryPeriod == zeroDuration {
		leaderElectionConfig.RetryPeriod = metav1.Duration{Duration: DefaultLeaderElectionRetryPeriod}
	}
}

// SetDefaults_ClientConnectionConfiguration sets defaults for the client connection configuration.
func SetDefaults_ClientConnectionConfiguration(clientConnConfig *ClientConnectionConfiguration) {
	if clientConnConfig.QPS == 0.0 {
		clientConnConfig.QPS = 100.0
	}
	if clientConnConfig.Burst == 0 {
		clientConnConfig.Burst = 120
	}
}

const (
	// DefaultWebhooksServerTLSServerCertDir is the default directory for the webhook server TLS certificate.
	DefaultWebhooksServerTLSServerCertDir = "/etc/webhook-server-tls"
	// DefaultWebhooksServerPort is the default port for the webhook server.
	DefaultWebhooksServerPort = 9443
	// DefaultMetricsServerPort is the default port for the metrics server.
	DefaultMetricsServerPort = 8080
)

// SetDefaults_ServerConfiguration sets defaults for the server configuration.
func SetDefaults_ServerConfiguration(serverConfig *ServerConfiguration) {
	if serverConfig.Webhooks == nil {
		serverConfig.Webhooks = &TLSServer{}
	}
	if serverConfig.Webhooks.Port == 0 {
		serverConfig.Webhooks.Port = DefaultWebhooksServerPort
	}
	if serverConfig.Webhooks.ServerCertDir == "" {
		serverConfig.Webhooks.ServerCertDir = DefaultWebhooksServerTLSServerCertDir
	}
	if serverConfig.Metrics == nil {
		serverConfig.Metrics = &Server{}
	}
	if serverConfig.Metrics.Port == 0 {
		serverConfig.Metrics.Port = DefaultMetricsServerPort
	}
}

const (
	// DefaultEtcdControllerConcurrentSyncs is the default number of concurrent syncs for the etcd controller.
	DefaultEtcdControllerConcurrentSyncs = 3
	// DefaultEtcdStatusSyncPeriod is the default period for syncing etcd status.
	DefaultEtcdStatusSyncPeriod = 15 * time.Second
	// DefaultEtcdNotReadyThreshold is the default threshold for etcd not ready status.
	DefaultEtcdNotReadyThreshold = 5 * time.Minute
	// DefaultEtcdUnknownThreshold is the default threshold for etcd unknown status.
	DefaultEtcdUnknownThreshold = 1 * time.Minute
)

// SetDefaults_EtcdControllerConfiguration sets defaults for the etcd controller configuration.
func SetDefaults_EtcdControllerConfiguration(etcdCtrlConfig *EtcdControllerConfiguration) {
	if etcdCtrlConfig.ConcurrentSyncs == nil {
		etcdCtrlConfig.ConcurrentSyncs = ptr.To(DefaultEtcdControllerConcurrentSyncs)
	}
	if etcdCtrlConfig.EtcdStatusSyncPeriod == zeroDuration {
		etcdCtrlConfig.EtcdStatusSyncPeriod = metav1.Duration{Duration: DefaultEtcdStatusSyncPeriod}
	}
	if etcdCtrlConfig.EtcdMember.NotReadyThreshold == zeroDuration {
		etcdCtrlConfig.EtcdMember.NotReadyThreshold = metav1.Duration{Duration: DefaultEtcdNotReadyThreshold}
	}
	if etcdCtrlConfig.EtcdMember.UnknownThreshold == zeroDuration {
		etcdCtrlConfig.EtcdMember.UnknownThreshold = metav1.Duration{Duration: DefaultEtcdUnknownThreshold}
	}
}

const (
	// DefaultCompactionConcurrentSyncs is the default number of concurrent syncs for the compaction controller.
	DefaultCompactionConcurrentSyncs = 3
	// DefaultCompactionEventsThreshold is the default event threshold trigger compaction.
	DefaultCompactionEventsThreshold = 1000000
	// DefaultCompactionActiveDeadlineDuration is the default active deadline duration for compaction.
	DefaultCompactionActiveDeadlineDuration = 3 * time.Hour
)

// SetDefaults_CompactionControllerConfiguration sets defaults for the compaction controller configuration.
func SetDefaults_CompactionControllerConfiguration(compactionCtrlConfig *CompactionControllerConfiguration) {
	if !compactionCtrlConfig.Enabled {
		return
	}
	if compactionCtrlConfig.ConcurrentSyncs == nil {
		compactionCtrlConfig.ConcurrentSyncs = ptr.To(DefaultCompactionConcurrentSyncs)
	}
	if compactionCtrlConfig.EventsThreshold == 0 {
		compactionCtrlConfig.EventsThreshold = DefaultCompactionEventsThreshold
	}
	if compactionCtrlConfig.ActiveDeadlineDuration == zeroDuration {
		compactionCtrlConfig.ActiveDeadlineDuration = metav1.Duration{Duration: DefaultCompactionActiveDeadlineDuration}
	}
}

// DefaultEtcdCopyBackupsTaskConcurrentSyncs is the default number of concurrent syncs for the etcd copy backups task controller.
const DefaultEtcdCopyBackupsTaskConcurrentSyncs = 3

// SetDefaults_EtcdCopyBackupsTaskControllerConfiguration sets defaults for the etcd copy backups task controller configuration.
func SetDefaults_EtcdCopyBackupsTaskControllerConfiguration(etcdCopyBackupsTaskCtrlConfig *EtcdCopyBackupsTaskControllerConfiguration) {
	if !etcdCopyBackupsTaskCtrlConfig.Enabled {
		return
	}
	if etcdCopyBackupsTaskCtrlConfig.ConcurrentSyncs == nil {
		etcdCopyBackupsTaskCtrlConfig.ConcurrentSyncs = ptr.To(DefaultEtcdCopyBackupsTaskConcurrentSyncs)
	}
}

// DefaultSecretControllerConcurrentSyncs is the default number of concurrent syncs for the secret controller.
const DefaultSecretControllerConcurrentSyncs = 10

// SetDefaults_SecretControllerConfiguration sets defaults for the secret controller configuration.
func SetDefaults_SecretControllerConfiguration(secretCtrlConfig *SecretControllerConfiguration) {
	if secretCtrlConfig.ConcurrentSyncs == nil {
		secretCtrlConfig.ConcurrentSyncs = ptr.To(DefaultSecretControllerConcurrentSyncs)
	}
}

// SetDefaults_LogConfiguration sets defaults for the log configuration.
func SetDefaults_LogConfiguration(logConfig *LogConfiguration) {
	if logConfig.LogLevel == "" {
		logConfig.LogLevel = LogLevelInfo
	}
	if logConfig.LogFormat == "" {
		logConfig.LogFormat = LogFormatJSON
	}
}
