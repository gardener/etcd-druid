// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OperatorConfiguration defines the configuration for the etcd-druid operator.
type OperatorConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	// ClientConnection specifies the client connection configuration used to communicate to the Kubernetes API server.
	ClientConnection ClientConnectionConfiguration `json:"clientConnection"`
	// LeaderElection defines the configuration for the leader election of the controller-manager.
	LeaderElection LeaderElectionConfiguration `json:"leaderElection"`
	// Server defines the configuration for the HTTP server.
	Server ServerConfiguration `json:"server"`
	// Controllers defines the configuration for the controllers.
	Controllers ControllerConfiguration `json:"controllers"`
	// Webhooks defines the configuration for the admission webhooks.
	Webhooks WebhookConfiguration `json:"webhooks"`
	// FeatureGates is a map of feature gates (alpha/beta) to its enabled state.
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
	// Logging defines the configuration for logging.
	Logging LogConfiguration `json:"logging"`
}

// ClientConnectionConfiguration defines the configuration for constructing a client.
type ClientConnectionConfiguration struct {
	// QPS controls the number of queries per second allowed for a connection.
	QPS float32 `json:"qps"`
	// Burst allows extra queries to accumulate when a client is exceeding its rate.
	Burst int `json:"burst"`
	// ContentType is the content type used when sending data to the server from this client.
	ContentType string `json:"contentType"`
	// AcceptContentTypes defines the Accept header sent by clients when connecting to the server,
	// overriding the default value of 'application/json'. This field will control all connections
	// to the server used by a particular client.
	AcceptContentTypes string `json:"acceptContentTypes"`
}

// LeaderElectionConfiguration defines the configuration for the leader election.
type LeaderElectionConfiguration struct {
	// Enabled specifies whether leader election is enabled. Set this
	// to true when running replicated instances of the operator for high availability.
	Enabled bool `json:"enabled"`
	// LeaseDuration is the duration that non-leader candidates will wait
	// after observing a leadership renewal until attempting to acquire
	// leadership of the occupied but un-renewed leader slot. This is effectively the
	// maximum duration that a leader can be stopped before it is replaced
	// by another candidate. This is only applicable if leader election is
	// enabled.
	LeaseDuration metav1.Duration `json:"leaseDuration"`
	// RenewDeadline is the interval between attempts by the acting leader to
	// renew its leadership before it stops leading. This must be less than or
	// equal to the lease duration.
	// This is only applicable if leader election is enabled.
	RenewDeadline metav1.Duration `json:"renewDeadline"`
	// RetryPeriod is the duration leader elector clients should wait
	// between attempting acquisition and renewal of leadership.
	// This is only applicable if leader election is enabled.
	RetryPeriod metav1.Duration `json:"retryPeriod"`
	// ResourceLock determines which resource lock to use for leader election.
	// This is only applicable if leader election is enabled.
	ResourceLock string `json:"resourceLock"`
	// ResourceName determines the name of the resource that leader election
	// will use for holding the leader lock.
	// This is only applicable if leader election is enabled.
	ResourceName string `json:"resourceName"`
}

// ServerConfiguration contains the server configurations.
type ServerConfiguration struct {
	// Webhooks is the configuration for the TLS webhook server.
	// +optional
	Webhooks *TLSServer `json:"webhooks"`
	// Metrics is the configuration for serving the metrics endpoint.
	// +optional
	Metrics *Server `json:"metrics"`
}

// TLSServer is the configuration for a TLS enabled server.
type TLSServer struct {
	// Server is the configuration for the bind address and the port.
	Server `json:",inline"`
	// ServerCertDir is the path to a directory containing the server's TLS certificate and key (the files must be
	// named tls.crt and tls.key respectively).
	ServerCertDir string `json:"serverCertDir"`
}

// Server contains information for HTTP(S) server configuration.
type Server struct {
	// BindAddress is the IP address on which to listen for the specified port.
	BindAddress string `json:"bindAddress"`
	// Port is the port on which to serve unsecured, unauthenticated access.
	Port int `json:"port"`
}

// ControllerConfiguration defines the configuration for the controllers.
type ControllerConfiguration struct {
	// DisableLeaseCache disables the cache for lease.coordination.k8s.io resources.
	// Deprecated: This field will be eventually removed. It is recommended to not use this.
	// It has only been introduced to allow for backward compatibility with the old CLI flags.
	// +optional
	DisableLeaseCache bool `json:"disableLeaseCache"`
	// Etcd is the configuration for the Etcd controller.
	Etcd EtcdControllerConfiguration `json:"etcd"`
	// Compaction is the configuration for the compaction controller.
	Compaction CompactionControllerConfiguration `json:"compaction"`
	// EtcdCopyBackupsTask is the configuration for the EtcdCopyBackupsTask controller.
	EtcdCopyBackupsTask EtcdCopyBackupsTaskControllerConfiguration `json:"etcdCopyBackupsTask"`
	// Secret is the configuration for the Secret controller.
	Secret SecretControllerConfiguration `json:"secret"`
}

// EtcdControllerConfiguration defines the configuration for the Etcd controller.
type EtcdControllerConfiguration struct {
	// ConcurrentSyncs is the max number of concurrent workers that can be run, each worker servicing a reconcile request.
	// +optional
	ConcurrentSyncs *int `json:"concurrentSyncs,omitempty"`
	// EnableEtcdSpecAutoReconcile controls how the Etcd Spec is reconciled. If set to true, then any change in Etcd spec
	// will automatically trigger a reconciliation of the Etcd resource. If set to false, then an operator needs to
	// explicitly set gardener.cloud/operation=reconcile annotation on the Etcd resource to trigger reconciliation
	// of the Etcd spec.
	// NOTE: Decision to enable it should be carefully taken as spec updates could potentially result in rolling update
	// of the StatefulSet which will cause a minor downtime for a single node etcd cluster and can potentially cause a
	// downtime for a multi-node etcd cluster.
	EnableEtcdSpecAutoReconcile bool `json:"enableEtcdSpecAutoReconcile"`
	// DisableEtcdServiceAccountAutomount controls the auto-mounting of service account token for etcd StatefulSets.
	DisableEtcdServiceAccountAutomount bool `json:"disableEtcdServiceAccountAutomount"`
	// EtcdStatusSyncPeriod is the duration after which an event will be re-queued ensuring etcd status synchronization.
	EtcdStatusSyncPeriod metav1.Duration `json:"etcdStatusSyncPeriod"`
	// EtcdMember holds configuration related to etcd members.
	EtcdMember EtcdMemberConfiguration `json:"etcdMember"`
}

// EtcdMemberConfiguration holds configuration related to etcd members.
type EtcdMemberConfiguration struct {
	// NotReadyThreshold is the duration after which an etcd member's state is considered `NotReady`.
	NotReadyThreshold metav1.Duration `json:"notReadyThreshold"`
	// UnknownThreshold is the duration after which an etcd member's state is considered `Unknown`.
	UnknownThreshold metav1.Duration `json:"unknownThreshold"`
}

// SecretControllerConfiguration defines the configuration for the Secret controller.
type SecretControllerConfiguration struct {
	// ConcurrentSyncs is the max number of concurrent workers that can be run, each worker servicing a reconcile request.
	// +optional
	ConcurrentSyncs *int `json:"concurrentSyncs,omitempty"`
}

// CompactionControllerConfiguration defines the configuration for the compaction controller.
type CompactionControllerConfiguration struct {
	// Enabled specifies whether backup compaction should be enabled.
	Enabled bool `json:"enabled"`
	// ConcurrentSyncs is the max number of concurrent workers that can be run, each worker servicing a reconcile request.
	// +optional
	ConcurrentSyncs *int `json:"concurrentSyncs,omitempty"`
	// EventsThreshold denotes total number of etcd events to be reached upon which a backup compaction job is triggered.
	EventsThreshold int64 `json:"eventsThreshold"`
	// ActiveDeadlineDuration is the duration after which a running compaction job will be killed.
	ActiveDeadlineDuration metav1.Duration `json:"activeDeadlineDuration"`
	// MetricsScrapeWaitDuration is the duration to wait for after compaction job is completed, to allow Prometheus metrics to be scraped
	MetricsScrapeWaitDuration metav1.Duration `json:"metricsScrapeWaitDuration"`
}

// EtcdCopyBackupsTaskControllerConfiguration defines the configuration for the EtcdCopyBackupsTask controller.
type EtcdCopyBackupsTaskControllerConfiguration struct {
	// Enabled specifies whether EtcdCopyBackupsTaskController should be enabled.
	Enabled bool `json:"enabled"`
	// ConcurrentSyncs is the max number of concurrent workers that can be run, each worker servicing a reconcile request.
	ConcurrentSyncs *int `json:"concurrentSyncs,omitempty"`
}

// WebhookConfiguration defines the configuration for admission webhooks.
type WebhookConfiguration struct {
	// EtcdComponentProtection is the configuration for EtcdComponentProtection webhook.
	EtcdComponentProtection EtcdComponentProtectionWebhookConfiguration `json:"etcdComponentProtection"`
}

// EtcdComponentProtectionWebhookConfiguration defines the configuration for EtcdComponentProtection webhook.
// NOTE: At least one of ReconcilerServiceAccountFQDN or ServiceAccountInfo must be set. It is recommended to switch to ServiceAccountInfo.
type EtcdComponentProtectionWebhookConfiguration struct {
	// Enabled indicates whether the EtcdComponentProtection webhook is enabled.
	Enabled bool `json:"enabled"`
	// ReconcilerServiceAccountFQDN is the FQDN of the reconciler service account used by the etcd-druid operator.
	// Deprecated: Please use ServiceAccountInfo instead and ensure that both Name and Namespace are set via projected volumes and downward API in the etcd-druid deployment spec.
	ReconcilerServiceAccountFQDN *string `json:"reconcilerServiceAccountFQDN,omitempty"`
	// ServiceAccountInfo contains paths to gather etcd-druid service account information.
	ServiceAccountInfo *ServiceAccountInfo `json:"serviceAccountInfo"`
	// ExemptServiceAccounts is a list of service accounts that are exempt from Etcd Components Webhook checks.
	ExemptServiceAccounts []string `json:"exemptServiceAccounts"`
}

// ServiceAccountInfo contains paths to gather etcd-druid service account information.
// Usually downward API and projected volumes are used in the deployment specification of etcd-druid to provide this information as mounted volume files.
type ServiceAccountInfo struct {
	// Name is the name of the service account associated with etcd-druid deployment.
	Name string `json:"name"`
	// Namespace is the namespace in which the service account has been deployed.
	// Usually this information is usually available at /var/run/secrets/kubernetes.io/serviceaccount/namespace.
	// However, if automountServiceAccountToken is set to false then this file will not be available.
	Namespace string `json:"namespace"`
}

// LogFormat is the format of the log.
type LogFormat string

// LogLevel represents the level for logging.
type LogLevel string

const (
	// LogLevelDebug is the debug log level, i.e. the most verbose.
	LogLevelDebug LogLevel = "debug"
	// LogLevelInfo is the default log level.
	LogLevelInfo LogLevel = "info"
	// LogLevelError is a log level where only errors are logged.
	LogLevelError LogLevel = "error"
	// LogFormatJSON is the JSON log format.
	LogFormatJSON LogFormat = "json"
	// LogFormatText is the text log format.
	LogFormatText LogFormat = "text"
)

var (
	// AllLogLevels is a slice of all available log levels.
	AllLogLevels = []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelError}
	// AllLogFormats is a slice of all available log formats.
	AllLogFormats = []LogFormat{LogFormatJSON, LogFormatText}
)

// LogConfiguration contains the configuration for logging.
type LogConfiguration struct {
	// LogLevel is the level/severity for the logs. Must be one of [info,debug,error].
	LogLevel LogLevel `json:"logLevel"`
	// LogFormat is the output format for the logs. Must be one of [text,json].
	LogFormat LogFormat `json:"logFormat"`
}
