// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	gardencore "github.com/gardener/gardener/pkg/apis/core"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GardenletConfiguration defines the configuration for the Gardenlet.
type GardenletConfiguration struct {
	metav1.TypeMeta
	// GardenClientConnection specifies the kubeconfig file and the client connection settings
	// for the proxy server to use when communicating with the garden apiserver.
	GardenClientConnection *GardenClientConnection
	// SeedClientConnection specifies the client connection settings for the proxy server
	// to use when communicating with the seed apiserver.
	SeedClientConnection *SeedClientConnection
	// ShootClientConnection specifies the client connection settings for the proxy server
	// to use when communicating with the shoot apiserver.
	ShootClientConnection *ShootClientConnection
	// Controllers defines the configuration of the controllers.
	Controllers *GardenletControllerConfiguration
	// Resources defines the total capacity for seed resources and the amount reserved for use by Gardener.
	Resources *ResourcesConfiguration
	// LeaderElection defines the configuration of leader election client.
	LeaderElection *LeaderElectionConfiguration
	// LogLevel is the level/severity for the logs. Must be one of [info,debug,error].
	LogLevel *string
	// KubernetesLogLevel is the log level used for Kubernetes' k8s.io/klog functions.
	KubernetesLogLevel *klog.Level
	// Server defines the configuration of the HTTP server.
	Server *ServerConfiguration
	// FeatureGates is a map of feature names to bools that enable or disable alpha/experimental
	// features. This field modifies piecemeal the built-in default values from
	// "github.com/gardener/gardener/pkg/gardenlet/features/features.go".
	// Default: nil
	FeatureGates map[string]bool
	// SeedConfig contains configuration for the seed cluster. Must not be set if seed selector is set.
	// In this case the gardenlet creates the `Seed` object itself based on the provided config.
	SeedConfig *SeedConfig
	// SeedSelector contains an optional list of labels on `Seed` resources that shall be managed by
	// this gardenlet instance. In this case the `Seed` object is not managed by the Gardenlet and must
	// be created by an operator/administrator.
	SeedSelector *metav1.LabelSelector
	// Logging contains an optional configurations for the logging stack deployed
	// by the Gardenlet in the seed clusters.
	Logging *Logging
	// SNI contains an optional configuration for the APIServerSNI feature used
	// by the Gardenlet in the seed clusters.
	SNI *SNI
	// ExposureClassHandlers is a list of optional of exposure class handlers.
	ExposureClassHandlers []ExposureClassHandler
}

// GardenClientConnection specifies the kubeconfig file and the client connection settings
// for the proxy server to use when communicating with the garden apiserver.
type GardenClientConnection struct {
	componentbaseconfig.ClientConnectionConfiguration
	// GardenClusterAddress is the external address that the gardenlets can use to remotely connect to the Garden
	// cluster. It is needed in case the gardenlet deploys itself into shooted seeds.
	GardenClusterAddress *string
	// GardenClusterCACert is the external address that the gardenlets can use to remotely connect to the Garden
	// cluster. It is needed in case the gardenlet deploys itself into shooted seeds.
	GardenClusterCACert []byte
	// BootstrapKubeconfig is a reference to a secret that contains a data key 'kubeconfig' whose value
	// is a kubeconfig that can be used for bootstrapping. If `kubeconfig` is given then only this kubeconfig
	// will be considered.
	BootstrapKubeconfig *corev1.SecretReference
	// KubeconfigSecret is the reference to a secret object that stores the gardenlet's kubeconfig that
	// it uses to communicate with the garden cluster. If `kubeconfig` is given then only this kubeconfig
	// will be considered.
	KubeconfigSecret *corev1.SecretReference
}

// SeedClientConnection specifies the client connection settings
// for the proxy server to use when communicating with the seed apiserver.
type SeedClientConnection struct {
	componentbaseconfig.ClientConnectionConfiguration
}

// ShootClientConnection specifies the client connection settings
// for the proxy server to use when communicating with the shoot apiserver.
type ShootClientConnection struct {
	componentbaseconfig.ClientConnectionConfiguration
}

// GardenletControllerConfiguration defines the configuration of the controllers.
type GardenletControllerConfiguration struct {
	// BackupBucket defines the configuration of the BackupBucket controller.
	BackupBucket *BackupBucketControllerConfiguration
	// BackupEntry defines the configuration of the BackupEntry controller.
	BackupEntry *BackupEntryControllerConfiguration
	// ControllerInstallation defines the configuration of the ControllerInstallation controller.
	ControllerInstallation *ControllerInstallationControllerConfiguration
	// ControllerInstallationCare defines the configuration of the ControllerInstallationCare controller.
	ControllerInstallationCare *ControllerInstallationCareControllerConfiguration
	// ControllerInstallationRequired defines the configuration of the ControllerInstallationRequired controller.
	ControllerInstallationRequired *ControllerInstallationRequiredControllerConfiguration
	// Seed defines the configuration of the Seed controller.
	Seed *SeedControllerConfiguration
	// Shoot defines the configuration of the Shoot controller.
	Shoot *ShootControllerConfiguration
	// ShootCare defines the configuration of the ShootCare controller.
	ShootCare *ShootCareControllerConfiguration
	// ShootStateSync defines the configuration of the ShootState controller.
	ShootStateSync *ShootStateSyncControllerConfiguration
	// SeedAPIServerNetworkPolicy defines the configuration of the SeedAPIServerNetworkPolicy controller.
	SeedAPIServerNetworkPolicy *SeedAPIServerNetworkPolicyControllerConfiguration
	// ManagedSeedControllerConfiguration the configuration of the ManagedSeed controller.
	ManagedSeed *ManagedSeedControllerConfiguration
}

// BackupBucketControllerConfiguration defines the configuration of the BackupBucket
// controller.
type BackupBucketControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on events.
	ConcurrentSyncs *int
}

// BackupEntryControllerConfiguration defines the configuration of the BackupEntry
// controller.
type BackupEntryControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on events.
	ConcurrentSyncs *int
	// DeletionGracePeriodHours holds the period in number of hours to delete the BackupEntry after deletion timestamp is set.
	// If value is set to 0 then the BackupEntryController will trigger deletion immediately.
	DeletionGracePeriodHours *int
	// DeletionGracePeriodShootPurposes is a list of shoot purposes for which the deletion grace period applies. All
	// BackupEntries corresponding to Shoots with different purposes will be deleted immediately.
	DeletionGracePeriodShootPurposes []gardencore.ShootPurpose
}

// ControllerInstallationControllerConfiguration defines the configuration of the
// ControllerInstallation controller.
type ControllerInstallationControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on
	// events.
	ConcurrentSyncs *int
}

// ControllerInstallationCareControllerConfiguration defines the configuration of the ControllerInstallationCare
// controller.
type ControllerInstallationCareControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on
	// events.
	ConcurrentSyncs *int
	// SyncPeriod is the duration how often the existing resources are reconciled (how
	// often the health check of ControllerInstallations is performed.
	SyncPeriod *metav1.Duration
}

// ControllerInstallationRequiredControllerConfiguration defines the configuration of the ControllerInstallationRequired
// controller.
type ControllerInstallationRequiredControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on
	// events.
	ConcurrentSyncs *int
}

// SeedControllerConfiguration defines the configuration of the Seed controller.
type SeedControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on
	// events.
	ConcurrentSyncs *int
	// SyncPeriod is the duration how often the existing resources are reconciled.
	SyncPeriod *metav1.Duration
}

// ShootControllerConfiguration defines the configuration of the Shoot
// controller.
type ShootControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on
	// events.
	ConcurrentSyncs *int
	// ProgressReportPeriod is the period how often the progress of a shoot operation will be reported in the
	// Shoot's `.status.lastOperation` field. By default, the progress will be reported immediately after a task of the
	// respective flow has been completed. If you set this to a value > 0 (e.g., 5s) then it will be only reported every
	// 5 seconds. Any tasks that were completed in the meantime will not be reported.
	ProgressReportPeriod *metav1.Duration
	// ReconcileInMaintenanceOnly determines whether Shoot reconciliations happen only
	// during its maintenance time window.
	ReconcileInMaintenanceOnly *bool
	// RespectSyncPeriodOverwrite determines whether a sync period overwrite of a
	// Shoot (via annotation) is respected or not. Defaults to false.
	RespectSyncPeriodOverwrite *bool
	// RetryDuration is the maximum duration how often a reconciliation will be retried
	// in case of errors.
	RetryDuration *metav1.Duration
	// SyncPeriod is the duration how often the existing resources are reconciled.
	SyncPeriod *metav1.Duration
	// DNSEntryTTLSeconds is the TTL in seconds that is being used for DNS entries when reconciling shoots.
	// Default: 120s
	DNSEntryTTLSeconds *int64
}

// ShootCareControllerConfiguration defines the configuration of the ShootCare
// controller.
type ShootCareControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on
	// events.
	ConcurrentSyncs *int
	// SyncPeriod is the duration how often the existing resources are reconciled (how
	// often the health check of Shoot clusters is performed (only if no operation is
	// already running on them).
	SyncPeriod *metav1.Duration
	// StaleExtensionHealthChecks defines the configuration of the check for stale extension health checks.
	StaleExtensionHealthChecks *StaleExtensionHealthChecks
	// ConditionThresholds defines the condition threshold per condition type.
	ConditionThresholds []ConditionThreshold
}

// StaleExtensionHealthChecks defines the configuration of the check for stale extension health checks.
type StaleExtensionHealthChecks struct {
	// Enabled specifies whether the check for stale extensions health checks is enabled.
	// Defaults to true.
	Enabled bool
	// Threshold configures the threshold when gardenlet considers a health check report of an extension CRD as outdated.
	// The threshold should have some leeway in case a Gardener extension is temporarily unavailable.
	// Defaults to 5m.
	Threshold *metav1.Duration
}

// ConditionThreshold defines the duration how long a flappy condition stays in progressing state.
type ConditionThreshold struct {
	// Type is the type of the condition to define the threshold for.
	Type string
	// Duration is the duration how long the condition can stay in the progressing state.
	Duration metav1.Duration
}

// ShootStateSyncControllerConfiguration defines the configuration of the ShootState Sync controller.
type ShootStateSyncControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on
	// events.
	ConcurrentSyncs *int
	// SyncPeriod is the duration how often the existing extension resources are synced to the ShootState resource
	SyncPeriod *metav1.Duration
}

// SeedAPIServerNetworkPolicyControllerConfiguration defines the configuration of the SeedAPIServerNetworkPolicy
// controller.
type SeedAPIServerNetworkPolicyControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on events.
	ConcurrentSyncs *int
}

// ManagedSeedControllerConfiguration defines the configuration of the ManagedSeed controller.
type ManagedSeedControllerConfiguration struct {
	// ConcurrentSyncs is the number of workers used for the controller to work on
	// events.
	ConcurrentSyncs *int
	// SyncPeriod is the duration how often the existing resources are reconciled.
	SyncPeriod *metav1.Duration
	// WaitSyncPeriod is the duration how often an existing resource is reconciled when the controller is waiting for an event.
	WaitSyncPeriod *metav1.Duration
	// SyncJitterPeriod is a jitter duration for the reconciler sync that can be used to distribute the syncs randomly.
	// If its value is greater than 0 then the managed seeds will not be enqueued immediately but only after a random
	// duration between 0 and the configured value. It is defaulted to 5m.
	SyncJitterPeriod *metav1.Duration
}

// ResourcesConfiguration defines the total capacity for seed resources and the amount reserved for use by Gardener.
type ResourcesConfiguration struct {
	// Capacity defines the total resources of a seed.
	Capacity corev1.ResourceList
	// Reserved defines the resources of a seed that are reserved for use by Gardener.
	// Defaults to 0.
	Reserved corev1.ResourceList
}

// LeaderElectionConfiguration defines the configuration of leader election
// clients for components that can run with leader election enabled.
type LeaderElectionConfiguration struct {
	componentbaseconfig.LeaderElectionConfiguration
	// LockObjectNamespace defines the namespace of the lock object.
	LockObjectNamespace *string
	// LockObjectName defines the lock object name.
	LockObjectName *string
}

// SeedConfig contains configuration for the seed cluster.
type SeedConfig struct {
	gardencore.SeedTemplate
}

// FluentBit contains configuration for Fluent Bit.
type FluentBit struct {
	// ServiceSection defines [SERVICE] configuration for the fluent-bit.
	// If it is nil, fluent-bit uses default service configuration.
	ServiceSection *string
	// InputSection defines [INPUT] configuration for the fluent-bit.
	// If it is nil, fluent-bit uses default input configuration.
	InputSection *string
	// OutputSection defines [OUTPUT] configuration for the fluent-bit.
	// If it is nil, fluent-bit uses default output configuration.
	OutputSection *string
}

// Loki contains configuration for the Loki.
type Loki struct {
	// Garden contains configuration for the Loki in garden namespace.
	Garden *GardenLoki
}

// GardenLoki contains configuration for the Loki in garden namespace.
type GardenLoki struct {
	// Priority is the priority value for the Loki
	Priority *int32
}

// Logging contains configuration for the logging stack.
type Logging struct {
	// FluentBit contains configurations for the fluent-bit
	FluentBit *FluentBit
	// Loki contains configuration for the Loki
	Loki *Loki
}

// ServerConfiguration contains details for the HTTP(S) servers.
type ServerConfiguration struct {
	// HTTPS is the configuration for the HTTPS server.
	HTTPS HTTPSServer
}

// Server contains information for HTTP(S) server configuration.
type Server struct {
	// BindAddress is the IP address on which to listen for the specified port.
	BindAddress string
	// Port is the port on which to serve unsecured, unauthenticated access.
	Port int
}

// HTTPSServer is the configuration for the HTTPSServer server.
type HTTPSServer struct {
	// Server is the configuration for the bind address and the port.
	Server
	// TLSServer contains information about the TLS configuration for a HTTPS server. If empty then a proper server
	// certificate will be self-generated during startup.
	TLS *TLSServer
}

// TLSServer contains information about the TLS configuration for a HTTPS server.
type TLSServer struct {
	// ServerCertPath is the path to the server certificate file.
	ServerCertPath string
	// ServerKeyPath is the path to the private key file.
	ServerKeyPath string
}

// SNI contains an optional configuration for the APIServerSNI feature used
// by the Gardenlet in the seed clusters.
type SNI struct {
	// Ingress is the ingressgateway configuration.
	Ingress *SNIIngress
}

// SNIIngress contains configuration of the ingressgateway.
type SNIIngress struct {
	// ServiceName is the name of the ingressgateway Service.
	// Defaults to "istio-ingressgateway".
	ServiceName *string
	// Namespace is the namespace in which the ingressgateway is deployed in.
	// Defaults to "istio-ingress".
	Namespace *string
	// Labels of the ingressgateway
	// Defaults to "istio: ingressgateway".
	Labels map[string]string
}

// ExposureClassHandler contains configuration for an exposure class handler.
type ExposureClassHandler struct {
	// Name is the name of the exposure class handler.
	Name string
	// LoadBalancerService contains configuration which is used to configure the underlying
	// load balancer to apply the control plane endpoint exposure strategy.
	LoadBalancerService LoadBalancerServiceConfig
	// SNI contains optional configuration for a dedicated ingressgateway belonging to
	// an exposure class handler. This is only required in context of the APIServerSNI feature of the gardenlet.
	SNI *SNI
}

// LoadBalancerService contains configuration which is used to configure the underlying
// load balancer to apply the control plane endpoint exposure strategy.
type LoadBalancerServiceConfig struct {
	// Annotations is a key value map to annotate the underlying load balancer services.
	Annotations map[string]string
}
