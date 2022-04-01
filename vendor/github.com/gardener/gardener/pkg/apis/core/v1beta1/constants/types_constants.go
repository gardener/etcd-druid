// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package constants

const (
	// SecretNameCACluster is a constant for the name of a Kubernetes secret object that contains the CA
	// certificate of a shoot cluster.
	SecretNameCACluster = "ca"
	// SecretNameCAETCD is a constant for the name of a Kubernetes secret object that contains the CA
	// certificate of the etcd of a shoot cluster.
	SecretNameCAETCD = "ca-etcd"
	// SecretNameCAFrontProxy is a constant for the name of a Kubernetes secret object that contains the CA
	// certificate of the kube-aggregator a shoot cluster.
	SecretNameCAFrontProxy = "ca-front-proxy"
	// SecretNameCAKubelet is a constant for the name of a Kubernetes secret object that contains the CA
	// certificate of the kubelet of a shoot cluster.
	SecretNameCAKubelet = "ca-kubelet"
	// SecretNameCAMetricsServer is a constant for the name of a Kubernetes secret object that contains the CA
	// certificate of the metrics-server of a shoot cluster.
	SecretNameCAMetricsServer = "ca-metrics-server"
	// SecretNameCAVPN is a constant for the name of a Kubernetes secret object that contains the CA
	// certificate of the VPN components of a shoot cluster.
	SecretNameCAVPN = "ca-vpn"
	// SecretNameCloudProvider is a constant for the name of a Kubernetes secret object that contains the provider
	// specific credentials that shall be used to create/delete the shoot.
	SecretNameCloudProvider = "cloudprovider"
	// SecretNameSSHKeyPair is a constant for the name of a Kubernetes secret object that contains the SSH key pair
	// (public and private key) that can be used to SSH into the shoot nodes.
	SecretNameSSHKeyPair = "ssh-keypair"
	// SecretNameServiceAccountKey is a constant for the name of a Kubernetes secret object that contains a
	// PEM-encoded private RSA or ECDSA key used by the Kube Controller Manager to sign service account tokens
	SecretNameServiceAccountKey = "service-account-key"

	// SecretNameGardener is a constant for the name of a Kubernetes secret object that contains the client
	// certificate and a kubeconfig for a shoot cluster. It is used by Gardener and can be used by extension
	// controllers in order to communicate with the shoot's API server. The client certificate has administrator
	// privileges.
	SecretNameGardener = "gardener"
	// SecretNameGardenerInternal is a constant for the name of a Kubernetes secret object that contains the client
	// certificate and a kubeconfig for a shoot cluster. It is used by Gardener and can be used by extension
	// controllers in order to communicate with the shoot's API server. The client certificate has administrator
	// privileges. The difference to the "gardener" secret is that is contains the in-cluster endpoint as address to
	// for the shoot API server instead the DNS name or load balancer address.
	SecretNameGardenerInternal = "gardener-internal"

	// SecretNameGenericTokenKubeconfig is a constant for the name of the kubeconfig used by the shoot controlplane
	// components to authenticate against the shoot Kubernetes API server.
	// Use `pkg/extensions.GenericTokenKubeconfigSecretNameFromCluster` instead.
	SecretNameGenericTokenKubeconfig = "generic-token-kubeconfig"
	// AnnotationKeyGenericTokenKubeconfigSecretName is a constant for the key of an annotation on
	// extensions.gardener.cloud/v1alpha1.Cluster resources whose value contains the name of the generic token
	// kubeconfig secret in the seed cluster.
	AnnotationKeyGenericTokenKubeconfigSecretName = "generic-token-kubeconfig.secret.gardener.cloud/name"

	// SecretPrefixGeneratedBackupBucket is a constant for the prefix of a secret name in the garden cluster related to
	// BackpuBuckets.
	SecretPrefixGeneratedBackupBucket = "generated-bucket-"

	// DeploymentNameClusterAutoscaler is a constant for the name of a Kubernetes deployment object that contains
	// the cluster-autoscaler pod.
	DeploymentNameClusterAutoscaler = "cluster-autoscaler"
	// DeploymentNameKubeAPIServer is a constant for the name of a Kubernetes deployment object that contains
	// the kube-apiserver pod.
	DeploymentNameKubeAPIServer = "kube-apiserver"
	// DeploymentNameKubeControllerManager is a constant for the name of a Kubernetes deployment object that contains
	// the kube-controller-manager pod.
	DeploymentNameKubeControllerManager = "kube-controller-manager"
	// DeploymentNameGardenlet is a constant for the name of a Kubernetes deployment object that contains
	// the Gardenlet pod.
	DeploymentNameGardenlet = "gardenlet"

	// DeploymentNameVPNSeedServer is a constant for the name of a Kubernetes deployment object that contains
	// the vpn-seed-server pod.
	DeploymentNameVPNSeedServer = "vpn-seed-server"

	// DeploymentNameKubeScheduler is a constant for the name of a Kubernetes deployment object that contains
	// the kube-scheduler pod.
	DeploymentNameKubeScheduler = "kube-scheduler"
	// DeploymentNameGardenerResourceManager is a constant for the name of a Kubernetes deployment object that contains
	// the gardener-resource-manager pod.
	DeploymentNameGardenerResourceManager = "gardener-resource-manager"
	// DeploymentNameGrafanaOperators is a constant for the name of a Kubernetes deployment object that contains
	// the grafana-operators pod.
	DeploymentNameGrafanaOperators = "grafana-operators"
	// DeploymentNameGrafanaUsers is a constant for the name of a Kubernetes deployment object that contains
	// the grafana-users pod.
	DeploymentNameGrafanaUsers = "grafana-users"
	// DeploymentNameKubeStateMetricsShoot is a constant for the name of a Kubernetes deployment object that contains
	// the kube-state-metrics pod.
	DeploymentNameKubeStateMetricsShoot = "kube-state-metrics"

	// DeploymentNameVPAAdmissionController is a constant for the name of the VPA admission controller deployment.
	DeploymentNameVPAAdmissionController = "vpa-admission-controller"
	// DeploymentNameVPAExporter is a constant for the name of the VPA exporter deployment.
	DeploymentNameVPAExporter = "vpa-exporter"
	// DeploymentNameVPARecommender is a constant for the name of the VPA recommender deployment.
	DeploymentNameVPARecommender = "vpa-recommender"
	// DeploymentNameVPAUpdater is a constant for the name of the VPA updater deployment.
	DeploymentNameVPAUpdater = "vpa-updater"

	// DeploymentNameMachineControllerManager is a constant for the name of a Kubernetes deployment object that contains
	// the machine-controller-manager pod.
	DeploymentNameMachineControllerManager = "machine-controller-manager"

	// StatefulSetNameAlertManager is a constant for the name of a Kubernetes stateful set object that contains
	// the alertmanager pod.
	StatefulSetNameAlertManager = "alertmanager"
	// ETCDRoleMain is a constant for the main etcd role.
	ETCDRoleMain = "main"
	// ETCDRoleEvents is a constant for the events etcd role.
	ETCDRoleEvents = "events"
	// ETCDMain is a constant for the name of etcd-main Etcd object.
	ETCDMain = "etcd-" + ETCDRoleMain
	// ETCDEvents is a constant for the name of etcd-events Etcd object.
	ETCDEvents = "etcd-" + ETCDRoleEvents
	// StatefulSetNameLoki is a constant for the name of a Kubernetes stateful set object that contains
	// the loki pod.
	StatefulSetNameLoki = "loki"
	// StatefulSetNamePrometheus is a constant for the name of a Kubernetes stateful set object that contains
	// the prometheus pod.
	StatefulSetNamePrometheus = "prometheus"

	// GardenerPurpose is a constant for the key in a label describing the purpose of the respective object.
	GardenerPurpose = "gardener.cloud/purpose"
	// GardenerDescription is a constant for a key in an annotation describing what the resource is used for.
	GardenerDescription = "gardener.cloud/description"

	// GardenCreatedBy is the key for an annotation of a Shoot cluster whose value indicates contains the username
	// of the user that created the resource.
	GardenCreatedBy = "gardener.cloud/created-by"
	// GardenerOperation is a constant for an annotation on a resource that describes a desired operation.
	GardenerOperation = "gardener.cloud/operation"
	// GardenerOperationReconcile is a constant for the value of the operation annotation describing a reconcile
	// operation.
	GardenerOperationReconcile = "reconcile"
	// GardenerTimestamp is a constant for an annotation on a resource that describes the timestamp when a reconciliation has been requested.
	// It is only used to guarantee an update event for watching clients in case the operation-annotation is already present.
	GardenerTimestamp = "gardener.cloud/timestamp"
	// GardenerOperationMigrate is a constant for the value of the operation annotation describing a migration
	// operation.
	GardenerOperationMigrate = "migrate"
	// GardenerOperationRestore is a constant for the value of the operation annotation describing a restoration
	// operation.
	GardenerOperationRestore = "restore"
	// GardenerOperationWaitForState is a constant for the value of the operation annotation describing a wait
	// operation.
	GardenerOperationWaitForState = "wait-for-state"
	// GardenerOperationKeepalive is a constant for the value of the operation annotation describing an
	// operation that extends the lifetime of the object having the operation annotation.
	GardenerOperationKeepalive = "keepalive"

	// DeprecatedGardenRole is the key for an annotation on a Kubernetes object indicating what it is used for.
	//
	// Deprecated: Use `GardenRole` instead.
	DeprecatedGardenRole = "garden.sapcloud.io/role"
	// GardenRole is a constant for a label that describes a role.
	GardenRole = "gardener.cloud/role"
	// GardenRoleExtension is a constant for a label that describes the 'extensions' role.
	GardenRoleExtension = "extension"
	// GardenRoleGarden is the value of the GardenRole key indicating type 'garden'.
	GardenRoleGarden = "garden"
	// GardenRoleSeed is the value of the GardenRole key indicating type 'seed'.
	GardenRoleSeed = "seed"
	// GardenRoleShoot is the value of the GardenRole key indicating type 'shoot'.
	GardenRoleShoot = "shoot"
	// GardenRoleLogging is the value of the GardenRole key indicating type 'logging'.
	GardenRoleLogging = "logging"
	// GardenRoleProject is the value of GardenRole key indicating type 'project'.
	GardenRoleProject = "project"
	// GardenRoleControlPlane is the value of the GardenRole key indicating type 'controlplane'.
	GardenRoleControlPlane = "controlplane"
	// GardenRoleSystemComponent is the value of the GardenRole key indicating type 'system-component'.
	GardenRoleSystemComponent = "system-component"
	// GardenRoleMonitoring is the value of the GardenRole key indicating type 'monitoring'.
	GardenRoleMonitoring = "monitoring"
	// GardenRoleOptionalAddon is the value of the GardenRole key indicating type 'optional-addon'.
	GardenRoleOptionalAddon = "optional-addon"
	// GardenRoleCloudConfig is the value of the GardenRole key indicating type 'cloud-config'.
	GardenRoleCloudConfig = "cloud-config"
	// GardenRoleKubeconfig is the value of the GardenRole key indicating type 'kubeconfig'.
	GardenRoleKubeconfig = "kubeconfig"
	// GardenRoleCACluster is the value of the GardenRole key indicating type 'ca-cluster'.
	GardenRoleCACluster = "ca-cluster"
	// GardenRoleSSHKeyPair is the value of the GardenRole key indicating type 'ssh-keypair'.
	GardenRoleSSHKeyPair = "ssh-keypair"
	// GardenRoleDefaultDomain is the value of the GardenRole key indicating type 'default-domain'.
	GardenRoleDefaultDomain = "default-domain"
	// GardenRoleInternalDomain is the value of the GardenRole key indicating type 'internal-domain'.
	GardenRoleInternalDomain = "internal-domain"
	// GardenRoleOpenVPNDiffieHellman is the value of the GardenRole key indicating type 'openvpn-diffie-hellman'.
	GardenRoleOpenVPNDiffieHellman = "openvpn-diffie-hellman"
	// GardenRoleGlobalMonitoring is the value of the GardenRole key indicating type 'global-monitoring'
	GardenRoleGlobalMonitoring = "global-monitoring"
	// GardenRoleGlobalShootRemoteWriteMonitoring is the value of the GardenRole key indicating type 'global-shoot-remote-write-monitoring'
	GardenRoleGlobalShootRemoteWriteMonitoring = "global-shoot-remote-write-monitoring"
	// GardenRoleAlerting is the value of GardenRole key indicating type 'alerting'.
	GardenRoleAlerting = "alerting"
	// GardenRoleHvpa is the value of GardenRole key indicating type 'hvpa'.
	GardenRoleHvpa = "hvpa"
	// GardenRoleControlPlaneWildcardCert is the value of the GardenRole key indicating type 'controlplane-cert'.
	// It refers to a wildcard tls certificate which can be used for services exposed under the corresponding domain.
	GardenRoleControlPlaneWildcardCert = "controlplane-cert"

	// ShootUID is an annotation key for the shoot namespace in the seed cluster,
	// which value will be the value of `shoot.status.uid`
	ShootUID = "shoot.gardener.cloud/uid"
	// ShootPurpose is a constant for the shoot purpose.
	ShootPurpose = "shoot.gardener.cloud/purpose"
	// ShootSyncPeriod is a constant for an annotation on a Shoot which may be used to overwrite the global Shoot controller sync period.
	// The value must be a duration. It can also be used to disable the reconciliation at all by setting it to 0m. Disabling the reconciliation
	// does only mean that the period reconciliation is disabled. However, when the Gardener is restarted/redeployed or the specification is
	// changed then the reconciliation flow will be executed.
	ShootSyncPeriod = "shoot.gardener.cloud/sync-period"
	// ShootIgnore is a constant for an annotation on a Shoot which may be used to tell the Gardener that the Shoot with this name should be
	// ignored completely. That means that the Shoot will never reach the reconciliation flow (independent of the operation (create/update/
	// delete)).
	ShootIgnore = "shoot.gardener.cloud/ignore"
	// ShootNoCleanup is a constant for a label on a resource indicating that the Gardener cleaner should not delete this
	// resource when cleaning a shoot during the deletion flow.
	ShootNoCleanup = "shoot.gardener.cloud/no-cleanup"
	// ShootAlphaScalingAPIServerClass is a constant for an annotation on the shoot stating the initial API server class.
	// It influences the size of the initial resource requests/limits.
	// Possible values are [small, medium, large, xlarge, 2xlarge].
	// Note that this annotation is alpha and can be removed anytime without further notice. Only use it if you know
	// what you do.
	ShootAlphaScalingAPIServerClass = "alpha.kube-apiserver.scaling.shoot.gardener.cloud/class"
	// ShootAlphaControlPlaneScaleDownDisabled is a constant for an annotation on the Shoot resource stating that the
	// automatic scale-down shall be disabled for the etcd, kube-apiserver, kube-controller-manager.
	// Note that this annotation is alpha and can be removed anytime without further notice. Only use it if you know
	// what you do.
	ShootAlphaControlPlaneScaleDownDisabled = "alpha.control-plane.scaling.shoot.gardener.cloud/scale-down-disabled"
	// ShootExpirationTimestamp is an annotation on a Shoot resource whose value represents the time when the Shoot lifetime
	// is expired. The lifetime can be extended, but at most by the minimal value of the 'clusterLifetimeDays' property
	// of referenced quotas.
	ShootExpirationTimestamp = "shoot.gardener.cloud/expiration-timestamp"
	// ShootStatus is a constant for a label on a Shoot resource indicating that the Shoot's health.
	ShootStatus = "shoot.gardener.cloud/status"
	// FailedShootNeedsRetryOperation is a constant for an annotation on a Shoot in a failed state indicating that a retry operation should be triggered during the next maintenance time window.
	FailedShootNeedsRetryOperation = "maintenance.shoot.gardener.cloud/needs-retry-operation"
	// ShootTasks is a constant for an annotation on a Shoot which states that certain tasks should be done.
	ShootTasks = "shoot.gardener.cloud/tasks"
	// ShootTaskDeployInfrastructure is a name for a Shoot's infrastructure deployment task. It indicates that the
	// Infrastructure extension resource shall be reconciled.
	ShootTaskDeployInfrastructure = "deployInfrastructure"
	// ShootTaskDeployDNSRecordInternal is a name for a Shoot's internal DNS record deployment task. It indicates that
	// the internal DNSRecord extension resources shall be reconciled.
	ShootTaskDeployDNSRecordInternal = "deployDNSRecordInternal"
	// ShootTaskDeployDNSRecordExternal is a name for a Shoot's external DNS record deployment task. It indicates that
	// the external DNSRecord extension resources shall be reconciled.
	ShootTaskDeployDNSRecordExternal = "deployDNSRecordExternal"
	// ShootTaskDeployDNSRecordIngress is a name for a Shoot's ingress DNS record deployment task. It indicates that
	// the ingress DNSRecord extension resources shall be reconciled.
	ShootTaskDeployDNSRecordIngress = "deployDNSRecordIngress"
	// ShootTaskRestartControlPlanePods is a name for a Shoot task which is dedicated to restart related control plane pods.
	ShootTaskRestartControlPlanePods = "restartControlPlanePods"
	// ShootTaskRestartCoreAddons is a name for a Shoot task which is dedicated to restart some core addons.
	ShootTaskRestartCoreAddons = "restartCoreAddons"
	// ShootOperationMaintain is a constant for an annotation on a Shoot indicating that the Shoot maintenance shall be
	// executed as soon as possible.
	ShootOperationMaintain = "maintain"
	// ShootOperationRetry is a constant for an annotation on a Shoot indicating that a failed Shoot reconciliation shall be
	// retried.
	ShootOperationRetry = "retry"
	// ShootOperationRotateKubeconfigCredentials is a constant for an annotation on a Shoot indicating that the credentials
	// contained in the kubeconfig that is handed out to the user shall be rotated.
	ShootOperationRotateKubeconfigCredentials = "rotate-kubeconfig-credentials"
	// ShootOperationRotateSSHKeypair is a constant for an annotation on a Shoot indicating that the SSH keypair for the shoot
	// nodes shall be rotated.
	ShootOperationRotateSSHKeypair = "rotate-ssh-keypair"
	// ShootOperationRotateCAStart is a constant for an annotation on a Shoot indicating that the rotation of the
	// certificate authorities shall be started.
	ShootOperationRotateCAStart = "rotate-ca-start"
	// ShootOperationRotateCAComplete is a constant for an annotation on a Shoot indicating that the rotation of the
	// certificate authorities shall be completed.
	ShootOperationRotateCAComplete = "rotate-ca-complete"

	// SeedResourceManagerClass is the resource-class managed by the Gardener-Resource-Manager
	// instance in the garden namespace on the seeds.
	SeedResourceManagerClass = "seed"
	// LabelBackupProvider is used to identify the backup provider.
	LabelBackupProvider = "backup.gardener.cloud/provider"
	// LabelSeedProvider is used to identify the seed provider.
	LabelSeedProvider = "seed.gardener.cloud/provider"
	// LabelShootProvider is used to identify the shoot provider.
	LabelShootProvider = "shoot.gardener.cloud/provider"
	// LabelShootProviderPrefix is used to prefix label that indicates the provider type.
	// The label key is in the form provider.shoot.gardener.cloud/<type>.
	LabelShootProviderPrefix = "provider.shoot.gardener.cloud/"
	// LabelNetworkingProvider is used to identify the networking provider for the cni plugin.
	LabelNetworkingProvider = "networking.shoot.gardener.cloud/provider"
	// LabelExtensionPrefix is used to prefix extension specific labels.
	LabelExtensionPrefix = "extensions.gardener.cloud/"
	// LabelExtensionConfiguration is used to identify the provider's configuration which will be added to Gardener configuration
	LabelExtensionConfiguration = LabelExtensionPrefix + "configuration"
	// LabelLogging is a constant for a label for logging stack configurations
	LabelLogging = "logging"
	// LabelMonitoring is a constant for a label for monitoring stack configurations
	LabelMonitoring = "monitoring"

	// LabelExtensionExtensionTypePrefix is used to prefix extension label for extension types.
	LabelExtensionExtensionTypePrefix = "extensions.extensions.gardener.cloud/"
	// LabelExtensionProviderTypePrefix is used to prefix extension label for cloud provider types.
	LabelExtensionProviderTypePrefix = "provider.extensions.gardener.cloud/"
	// LabelExtensionDNSRecordTypePrefix is used to prefix extension label for DNS types.
	LabelExtensionDNSRecordTypePrefix = "dnsrecord.extensions.gardener.cloud/"
	// LabelExtensionNetworkingTypePrefix is used to prefix extension label for networking plugin types.
	LabelExtensionNetworkingTypePrefix = "networking.extensions.gardener.cloud/"
	// LabelExtensionOperatingSystemConfigTypePrefix is used to prefix extension label for OperatingSystemConfig types.
	LabelExtensionOperatingSystemConfigTypePrefix = "operatingsystemconfig.extensions.gardener.cloud/"
	// LabelExtensionContainerRuntimeTypePrefix is used to prefix extension label for ContainerRuntime types.
	LabelExtensionContainerRuntimeTypePrefix = "containerruntime.extensions.gardener.cloud/"

	// LabelNetworkPolicyToBlockedCIDRs allows Egress from pods labeled with 'networking.gardener.cloud/to-blocked-cidrs=allowed'.
	LabelNetworkPolicyToBlockedCIDRs = "networking.gardener.cloud/to-blocked-cidrs"
	// LabelNetworkPolicyToDNS allows Egress from pods labeled with 'networking.gardener.cloud/to-dns=allowed' to DNS running in 'kube-system'.
	// In practice, most of the Pods which require network Egress need this label.
	LabelNetworkPolicyToDNS = "networking.gardener.cloud/to-dns"
	// LabelNetworkPolicyToPrivateNetworks allows Egress from pods labeled with 'networking.gardener.cloud/to-private-networks=allowed' to the
	// private networks (RFC1918), Carrier-grade NAT (RFC6598) except for cloudProvider's specific metadata service IP, seed networks,
	// shoot networks.
	LabelNetworkPolicyToPrivateNetworks = "networking.gardener.cloud/to-private-networks"
	// LabelNetworkPolicyToPublicNetworks allows Egress from pods labeled with 'networking.gardener.cloud/to-public-networks=allowed' to all public
	// network IPs, except for private networks (RFC1918), carrier-grade NAT (RFC6598), cloudProvider's specific metadata service IP.
	// In practice, this blocks Egress traffic to all networks in the Seed cluster and only traffic to public IPv4 addresses.
	LabelNetworkPolicyToPublicNetworks = "networking.gardener.cloud/to-public-networks"
	// LabelNetworkPolicyToSeedAPIServer allows Egress from pods labeled with 'networking.gardener.cloud/to-seed-apiserver=allowed' to Seed's Kubernetes
	// API Server.
	LabelNetworkPolicyToSeedAPIServer = "networking.gardener.cloud/to-seed-apiserver"
	// LabelNetworkPolicyToShootAPIServer allows Egress from pods labeled with 'networking.gardener.cloud/to-shoot-apiserver=allowed' to talk to Shoot's
	// Kubernetes API Server.
	LabelNetworkPolicyToShootAPIServer = "networking.gardener.cloud/to-shoot-apiserver"
	// LabelNetworkPolicyToShootNetworks allows Egress from pods labeled with 'networking.gardener.cloud/to-shoot-networks=allowed' to IPv4 blocks belonging to the Shoot network.
	LabelNetworkPolicyToShootNetworks = "networking.gardener.cloud/to-shoot-networks"
	// LabelNetworkPolicyToAllShootAPIServers allows Egress from pods labeled with 'networking.gardener.cloud/to-all-shoot-apiservers=allowed' to talk to all
	// Shoots' Kubernetes API Servers.
	LabelNetworkPolicyToAllShootAPIServers = "networking.gardener.cloud/to-all-shoot-apiservers"
	// LabelNetworkPolicyFromShootAPIServer allows Egress from Shoot's Kubernetes API Server to talk to pods labeled with
	// 'networking.gardener.cloud/from-shoot-apiserver=allowed'.
	LabelNetworkPolicyFromShootAPIServer = "networking.gardener.cloud/from-shoot-apiserver"
	// LabelNetworkPolicyToAll disables all Ingress and Egress traffic into/from this namespace when set to "disallowed".
	LabelNetworkPolicyToAll = "networking.gardener.cloud/to-all"
	// LabelNetworkPolicyFromPrometheus allows Ingress from Prometheus to pods labeled with 'networking.gardener.cloud/from-prometheus=allowed' and ports
	// named 'metrics' in the PodSpecification.
	LabelNetworkPolicyFromPrometheus = "networking.gardener.cloud/from-prometheus"
	// LabelNetworkPolicyToAggregatePrometheus allows Egress traffic to the aggregate Prometheus.
	LabelNetworkPolicyToAggregatePrometheus = "networking.gardener.cloud/to-aggregate-prometheus"
	// LabelNetworkPolicyToSeedPrometheus allows Egress traffic to the seed Prometheus.
	LabelNetworkPolicyToSeedPrometheus = "networking.gardener.cloud/to-seed-prometheus"
	// LabelNetworkPolicyShootFromSeed allows Ingress traffic from the seed cluster (where the shoot's kube-apiserver
	// runs).
	LabelNetworkPolicyShootFromSeed = "networking.gardener.cloud/from-seed"
	// LabelNetworkPolicyShootToAPIServer allows Egress traffic to the shoot's API server.
	LabelNetworkPolicyShootToAPIServer = "networking.gardener.cloud/to-apiserver"
	// LabelNetworkPolicyShootToKubelet allows Egress traffic to the kubelets.
	LabelNetworkPolicyShootToKubelet = "networking.gardener.cloud/to-kubelet"
	// LabelNetworkPolicyAllowed is a constant for allowing a network policy.
	LabelNetworkPolicyAllowed = "allowed"
	// LabelNetworkPolicyDisallowed is a constant for disallowing a network policy.
	LabelNetworkPolicyDisallowed = "disallowed"

	// LabelApp is a constant for a label key.
	LabelApp = "app"
	// LabelRole is a constant for a label key.
	LabelRole = "role"
	// LabelKubernetes is a constant for a label for Kubernetes workload.
	LabelKubernetes = "kubernetes"
	// LabelAPIServer is a constant for a label for the kube-apiserver.
	LabelAPIServer = "apiserver"
	// LabelControllerManager is a constant for a label for the kube-controller-manager.
	LabelControllerManager = "controller-manager"
	// LabelScheduler is a constant for a label for the kube-scheduler.
	LabelScheduler = "scheduler"
	// LabelProxy is a constant for a label for the kube-proxy.
	LabelProxy = "proxy"
	// LabelExtensionProjectRole is a constant for a label value for extension project roles
	LabelExtensionProjectRole = "extension-project-role"

	// LabelAPIServerExposure is a constant for label key which gardener can add to various objects related
	// to kube-apiserver exposure.
	LabelAPIServerExposure = "core.gardener.cloud/apiserver-exposure"
	// LabelAPIServerExposureGardenerManaged is a constant for label value which gardener sets on the label key
	// "core.gardener.cloud/apiserver-exposure" to indicate that it's responsible for apiserver exposure (via SNI).
	LabelAPIServerExposureGardenerManaged = "gardener-managed"

	// GardenNamespace is the namespace in which the configuration and secrets for
	// the Gardener controller manager will be stored (e.g., secrets for the Seed clusters).
	// It is also used by the gardener-apiserver.
	GardenNamespace = "garden"

	// AnnotationShootUseAsSeed is a constant for an annotation on a Shoot resource indicating that the Shoot shall be registered as Seed in the
	// Garden cluster once successfully created.
	AnnotationShootUseAsSeed = "shoot.gardener.cloud/use-as-seed"
	// AnnotationManagedSeedAPIServer is a constant for an annotation on a Shoot resource containing the API server settings for a managed seed.
	AnnotationManagedSeedAPIServer = "shoot.gardener.cloud/managed-seed-api-server"
	// AnnotationShootIgnoreAlerts is the key for an annotation of a Shoot cluster whose value indicates
	// if alerts for this cluster should be ignored
	AnnotationShootIgnoreAlerts = "shoot.gardener.cloud/ignore-alerts"
	// AnnotationShootSkipCleanup is a key for an annotation on a Shoot resource that declares that the clean up steps should be skipped when the
	// cluster is deleted. Concretely, this will skip everything except the deletion of (load balancer) services and persistent volume resources.
	AnnotationShootSkipCleanup = "shoot.gardener.cloud/skip-cleanup"
	// AnnotationShootCleanupWebhooksFinalizeGracePeriodSeconds is a key for an annotation on a Shoot resource that
	// declares the grace period in seconds for finalizing the resources handled in the 'cleanup webhooks' step.
	// Concretely, after the specified seconds, all the finalizers of the affected resources are forcefully removed.
	AnnotationShootCleanupWebhooksFinalizeGracePeriodSeconds = "shoot.gardener.cloud/cleanup-webhooks-finalize-grace-period-seconds"
	// AnnotationShootCleanupExtendedAPIsFinalizeGracePeriodSeconds is a key for an annotation on a Shoot resource that
	// declares the grace period in seconds for finalizing the resources handled in the 'cleanup extended APIs' step.
	// Concretely, after the specified seconds, all the finalizers of the affected resources are forcefully removed.
	AnnotationShootCleanupExtendedAPIsFinalizeGracePeriodSeconds = "shoot.gardener.cloud/cleanup-extended-apis-finalize-grace-period-seconds"
	// AnnotationShootCleanupKubernetesResourcesFinalizeGracePeriodSeconds is a key for an annotation on a Shoot
	// resource that declares the grace period in seconds for finalizing the resources handled in the 'cleanup
	// Kubernetes resources' step. Concretely, after the specified seconds, all the finalizers of the affected resources
	// are forcefully removed.
	AnnotationShootCleanupKubernetesResourcesFinalizeGracePeriodSeconds = "shoot.gardener.cloud/cleanup-kubernetes-resources-finalize-grace-period-seconds"
	// AnnotationShootCleanupNamespaceResourcesFinalizeGracePeriodSeconds is a key for an annotation on a Shoot
	// resource that declares the grace period in seconds for finalizing the resources handled in the 'cleanup shoot
	// namespaces' step. Concretely, after the specified seconds, all the finalizers of the affected resources are
	// forcefully removed.
	AnnotationShootCleanupNamespaceResourcesFinalizeGracePeriodSeconds = "shoot.gardener.cloud/cleanup-namespaces-finalize-grace-period-seconds"
	// AnnotationShootInfrastructureCleanupWaitPeriodSeconds is a key for an annotation on a Shoot
	// resource that declares the wait period in seconds for infrastructure resources cleanup. Concretely,
	// Gardener will wait for the specified time after the Infrastructure extension object has been deleted to allow
	// controllers to gracefully cleanup everything (default behaviour is 300s).
	AnnotationShootInfrastructureCleanupWaitPeriodSeconds = "shoot.gardener.cloud/infrastructure-cleanup-wait-period-seconds"
	// AnnotationShootForceRestore is a key for an annotation on a Shoot or BackupEntry resource to trigger a forceful restoration to a different seed.
	AnnotationShootForceRestore = "shoot.gardener.cloud/force-restore"
	// AnnotationReversedVPN moves the vpn-server to the seed.
	AnnotationReversedVPN = "alpha.featuregates.shoot.gardener.cloud/reversed-vpn"
	// AnnotationNodeLocalDNS enables a per node dns cache on the shoot cluster.
	AnnotationNodeLocalDNS = "alpha.featuregates.shoot.gardener.cloud/node-local-dns"
	// AnnotationNodeLocalDNSForceTcpToClusterDns enforces upgrade to tcp connections for communication between node local and cluster dns.
	AnnotationNodeLocalDNSForceTcpToClusterDns = "alpha.featuregates.shoot.gardener.cloud/node-local-dns-force-tcp-to-cluster-dns"
	// AnnotationNodeLocalDNSForceTcpToUpstreamDns enforces upgrade to tcp connections for communication between node local and upstream dns.
	AnnotationNodeLocalDNSForceTcpToUpstreamDns = "alpha.featuregates.shoot.gardener.cloud/node-local-dns-force-tcp-to-upstream-dns"

	// AnnotationShootAPIServerSNIPodInjector is the key for an annotation of a Shoot cluster whose value indicates
	// if pod injection of 'KUBERNETES_SERVICE_HOST' environment variable should happen for clusters where APIServerSNI
	// featuregate is enabled.
	// Any value than 'disable' enables this feature.
	AnnotationShootAPIServerSNIPodInjector = "alpha.featuregates.shoot.gardener.cloud/apiserver-sni-pod-injector"
	// AnnotationShootAPIServerSNIPodInjectorDisableValue is the value of the
	// `alpha.featuregates.shoot.gardener.cloud/apiserver-sni-pod-injector` annotation that disables the pod injection.
	AnnotationShootAPIServerSNIPodInjectorDisableValue = "disable"

	// OperatingSystemConfigUnitNameKubeletService is a constant for a unit in the operating system config that contains the kubelet service.
	OperatingSystemConfigUnitNameKubeletService = "kubelet.service"
	// OperatingSystemConfigUnitNameDockerService is a constant for a unit in the operating system config that contains the docker service.
	OperatingSystemConfigUnitNameDockerService = "docker.service"
	// OperatingSystemConfigUnitNameContainerDService is a constant for a unit in the operating system config that contains the containerd service.
	OperatingSystemConfigUnitNameContainerDService = "containerd.service"
	// OperatingSystemConfigFilePathKernelSettings is a constant for a path to a file in the operating system config that contains some general kernel settings.
	OperatingSystemConfigFilePathKernelSettings = "/etc/sysctl.d/99-k8s-general.conf"
	// OperatingSystemConfigFilePathKubeletConfig is a constant for a path to a file in the operating system config that contains the kubelet configuration.
	OperatingSystemConfigFilePathKubeletConfig = "/var/lib/kubelet/config/kubelet"
	// OperatingSystemConfigUnitNamePromtailService is a constant for a unit in the operating system config that contains the promtail service.
	OperatingSystemConfigUnitNamePromtailService = "promtail.service"
	// OperatingSystemConfigFilePathPromtailConfig is a constant for a path to a file in the operating system config that contains the kubelet configuration.
	OperatingSystemConfigFilePathPromtailConfig = "/var/lib/promtail/config/config"
	// OperatingSystemConfigFilePathBinaries is a constant for a path to a directory in the operating system config that contains the binaries.
	OperatingSystemConfigFilePathBinaries = "/opt/bin"

	// FluentBitConfigMapKubernetesFilter is a constant for the Fluent Bit ConfigMap's section regarding Kubernetes filters
	FluentBitConfigMapKubernetesFilter = "filter-kubernetes.conf"
	// FluentBitConfigMapParser is a constant for the Fluent Bit ConfigMap's section regarding Parsers for common container types
	FluentBitConfigMapParser = "parsers.conf"
	// PrometheusConfigMapAlertingRules is a constant for the Prometheus alerting rules tag in provider-specific monitoring configuration
	PrometheusConfigMapAlertingRules = "alerting_rules"
	// PrometheusConfigMapScrapeConfig is a constant for the Prometheus scrape config tag in provider-specific monitoring configuration
	PrometheusConfigMapScrapeConfig = "scrape_config"
	// GrafanaConfigMapUserDashboard is a constant for the Grafana user dashboard tag in provider-specific monitoring configuration
	GrafanaConfigMapUserDashboard = "dashboard_users"
	// GrafanaConfigMapOperatorDashboard is a constant for the Grafana operator dashboard tag in provider-specific monitoring configuration
	GrafanaConfigMapOperatorDashboard = "dashboard_operators"

	// LabelControllerRegistrationName is the key of a label on extension namespaces that indicates the controller registration name.
	LabelControllerRegistrationName = "controllerregistration.core.gardener.cloud/name"
	// LabelPodMaintenanceRestart is a constant for a label that describes that a pod should be restarted during maintenance.
	LabelPodMaintenanceRestart = "maintenance.gardener.cloud/restart"
	// LabelWorkerPool is a constant for a label that indicates the worker pool the node belongs to
	LabelWorkerPool = "worker.gardener.cloud/pool"
	// LabelWorkerKubernetesVersion is a constant for a label that indicates the Kubernetes version used for the worker pool nodes.
	LabelWorkerKubernetesVersion = "worker.gardener.cloud/kubernetes-version"
	// LabelWorkerPoolDeprecated is a deprecated constant for a label that indicates the worker pool the node belongs to
	LabelWorkerPoolDeprecated = "worker.garden.sapcloud.io/group"
	// LabelWorkerPoolSystemComponents is a constant that indicates whether the worker pool should host system components
	LabelWorkerPoolSystemComponents = "worker.gardener.cloud/system-components"

	// EventResourceReferenced indicates that the resource deletion is in waiting mode because the resource is still
	// being referenced by at least one other resource (e.g. a SecretBinding is still referenced by a Shoot)
	EventResourceReferenced = "ResourceReferenced"

	// PriorityClassNameShootControlPlane is the name of a priority class for critical pods of a shoot control plane.
	PriorityClassNameShootControlPlane = "gardener-shoot-controlplane"

	// ReferencedResourcesPrefix is the prefix used when copying referenced resources to the Shoot namespace in the Seed,
	// to avoid naming collisions with resources managed by Gardener.
	ReferencedResourcesPrefix = "ref-"

	// ClusterIdentity is a constant equal to the name and data key (that stores the identity) of the cluster-identity ConfigMap
	ClusterIdentity = "cluster-identity"

	// SeedNginxIngressClass defines the ingress class for the seed nginx ingress controller
	SeedNginxIngressClass = "nginx-gardener"
	// SeedNginxIngressClass122 defines the ingress class for the seed nginx ingress controller for K8s >= 1.22
	SeedNginxIngressClass122 = "nginx-ingress-gardener"
	// IngressKindNginx defines nginx as kind as managed Seed ingress
	IngressKindNginx = "nginx"
	// NginxIngressClass defines the ingress class for the seed nginx ingress controller if the seed cluster is a non Gardener managed cluster.
	NginxIngressClass = "nginx"

	// SeedsGroup is the identity group for gardenlets when authenticating to the API server.
	SeedsGroup = "gardener.cloud:system:seeds"
	// SeedUserNamePrefix is the identity user name prefix for gardenlets when authenticating to the API server.
	SeedUserNamePrefix = "gardener.cloud:system:seed:"

	// ProjectName is the key of a label on namespaces whose value holds the project name.
	ProjectName = "project.gardener.cloud/name"
	// ProjectSkipStaleCheck is the key of an annotation on a project namespace that marks the associated Project to be
	// skipped by the stale project controller. If the project has already configured stale timestamps in its status
	// then they will be reset.
	ProjectSkipStaleCheck = "project.gardener.cloud/skip-stale-check"
	// NamespaceProject is the key of an annotation on namespace whose value holds the project uid.
	NamespaceProject = "namespace.gardener.cloud/project"
	// NamespaceKeepAfterProjectDeletion is a constant for an annotation on a `Namespace` resource that states that it
	// should not be deleted if the corresponding `Project` gets deleted. Please note that all project related labels
	// from the namespace will be removed when the project is being deleted.
	NamespaceKeepAfterProjectDeletion = "namespace.gardener.cloud/keep-after-project-deletion"
	// NamespaceCreatedByProjectController is a constant for annotation on a `Namespace` resource that states that it
	// was created by the project controller because either the Project's `spec.namespace` field was not specified
	// or the specified namespace was not present.
	NamespaceCreatedByProjectController = "namespace.gardener.cloud/created-by-project-controller"

	// DefaultVpnRange is the default network range for the vpn between seed and shoot cluster.
	DefaultVpnRange = "192.168.123.0/24"

	// BackupSecretName is the name of secret having credentials for etcd backups.
	BackupSecretName string = "etcd-backup"
	// DataKeyBackupBucketName is the name of a data key whose value contains the backup bucket name.
	DataKeyBackupBucketName string = "bucketName"
	// BackupSourcePrefix is the prefix for names of resources related to source backupentries when copying backups.
	BackupSourcePrefix = "source"

	// GardenerAudience is the identifier for Gardener controllers when interacting with the API Server
	GardenerAudience = "gardener"

	// DNSRecordInternalName is a constant for DNSRecord objects used for the internal domain name.
	DNSRecordInternalName = "internal"
	// DNSRecordExternalName is a constant for DNSRecord objects used for the external domain name.
	DNSRecordExternalName = "external"
	// DNSRecordOwnerName is a constant for DNSRecord objects used for the owner domain name.
	DNSRecordOwnerName = "owner"
)

// ControlPlaneSecretRoles contains all role values used for control plane secrets synced to the Garden cluster.
var ControlPlaneSecretRoles = []string{
	GardenRoleKubeconfig,
	GardenRoleSSHKeyPair,
	GardenRoleMonitoring,
}
