// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

// Constants for image keys
const (
	// Etcd is the key for the etcd image in the image vector.
	Etcd = "etcd"
	// BackupRestore is the key for the etcd-backup-restore image in the image vector.
	BackupRestore = "etcd-backup-restore"
	// EtcdWrapper is the key for the etcd image in the image vector.
	EtcdWrapper = "etcd-wrapper"
	// BackupRestoreDistroless is the key for the etcd-backup-restore image in the image vector.
	BackupRestoreDistroless = "etcd-backup-restore-distroless"
	// Alpine is the key for the alpine image in the image vector.
	Alpine = "alpine"
)

const (
	// DefaultImageVectorFilePath is the path to the default image vector file.
	DefaultImageVectorFilePath = "charts/images.yaml"
	// FinalizerName is the name of the etcd finalizer.
	FinalizerName = "druid.gardener.cloud/etcd-druid"
)

// Constants for environment variables
const (
	// EnvPodName is the environment variable key for the pod name.
	EnvPodName = "POD_NAME"
	// EnvPodNamespace is the environment variable key for the pod namespace.
	EnvPodNamespace = "POD_NAMESPACE"
	// EnvStorageContainer is the environment variable key for the storage container.
	EnvStorageContainer = "STORAGE_CONTAINER"
	// EnvSourceStorageContainer is the environment variable key for the source storage container.
	EnvSourceStorageContainer = "SOURCE_STORAGE_CONTAINER"
	// EnvAWSApplicationCredentials is the environment variable key for AWS application credentials.
	EnvAWSApplicationCredentials = "AWS_APPLICATION_CREDENTIALS"
	// EnvAzureApplicationCredentials is the environment variable key for Azure application credentials.
	EnvAzureApplicationCredentials = "AZURE_APPLICATION_CREDENTIALS"
	// EnvGoogleApplicationCredentials is the environment variable key for Google application credentials.
	EnvGoogleApplicationCredentials = "GOOGLE_APPLICATION_CREDENTIALS"
	// EnvGoogleStorageAPIEndpoint is the environment variable key for Google storage API endpoint override.
	EnvGoogleStorageAPIEndpoint = "GOOGLE_STORAGE_API_ENDPOINT"
	// EnvOpenstackApplicationCredentials is the environment variable key for OpenStack application credentials.
	EnvOpenstackApplicationCredentials = "OPENSTACK_APPLICATION_CREDENTIALS"
	// EnvAlicloudApplicationCredentials is the environment variable key for Alicloud application credentials.
	EnvAlicloudApplicationCredentials = "ALICLOUD_APPLICATION_CREDENTIALS"
	// EnvOpenshiftApplicationCredentials is the environment variable key for OpenShift application credentials.
	EnvOpenshiftApplicationCredentials = "OPENSHIFT_APPLICATION_CREDENTIALS"
	// EnvECSEndpoint is the environment variable key for Dell ECS endpoint.
	EnvECSEndpoint = "ECS_ENDPOINT"
	// EnvECSAccessKeyID is the environment variable key for Dell ECS access key ID.
	EnvECSAccessKeyID = "ECS_ACCESS_KEY_ID"
	// EnvECSSecretAccessKey is the environment variable key for Dell ECS secret access key.
	EnvECSSecretAccessKey = "ECS_SECRET_ACCESS_KEY"
)

// Constants for values to be set against druidv1alpha1.LabelComponentKey
const (
	// ClientServiceComponentName is the component name for client service resource.
	ClientServiceComponentName = "etcd-client-service"
	// ConfigMapComponentName is the component  name for config map resource.
	ConfigMapComponentName = "etcd-config"
	// MemberLeaseComponentName is the component name for member lease resource.
	MemberLeaseComponentName = "etcd-member-lease"
	// SnapshotLeaseComponentName is the component name for snapshot lease resource.
	SnapshotLeaseComponentName = "etcd-snapshot-lease"
	// PeerServiceComponentName is the component name for peer service resource.
	PeerServiceComponentName = "etcd-peer-service"
	// PodDisruptionBudgetComponentName is the component name for pod disruption budget resource.
	PodDisruptionBudgetComponentName = "etcd-pdb"
	// RoleComponentName is the component name for role resource.
	RoleComponentName = "etcd-druid-role"
	// RoleBindingComponentName is the component name for role binding resource.
	RoleBindingComponentName = "druid-role-binding"
	// ServiceAccountComponentName is the component name for service account resource.
	ServiceAccountComponentName = "druid-service-account"
	// StatefulSetComponentName is the component name for statefulset resource.
	StatefulSetComponentName = "etcd-sts"
	// CompactionJobComponentName is the component name for compaction job resource.
	CompactionJobComponentName = "etcd-compaction-job"
	// EtcdCopyBackupTaskComponentName is the component name for copy-backup task resource.
	EtcdCopyBackupTaskComponentName = "etcd-copy-backup-task"
)

const (
	// ConfigMapCheckSumKey is the key that is set by a configmap operator and used by StatefulSet operator to
	// place an annotation on the StatefulSet pods. The value contains the check-sum of the latest configmap that
	// should be reflected on the pods.
	ConfigMapCheckSumKey = "checksum/etcd-configmap"
)

// Constants for container names
const (
	// EtcdContainerName is the name of the etcd container.
	EtcdContainerName = "etcd"
	// EtcdBackupRestoreContainerName is the name of the backup-restore container.
	EtcdBackupRestoreContainerName = "backup-restore"
	// ChangePermissionsInitContainerName is the name of the change permissions init container.
	ChangePermissionsInitContainerName = "change-permissions"
	// ChangeBackupBucketPermissionsInitContainerName is the name of the change backup bucket permissions init container.
	ChangeBackupBucketPermissionsInitContainerName = "change-backup-bucket-permissions"
)

// Constants for volume names
const (
	// EtcdCAVolumeName is the name of the volume that contains the CA certificate bundle and CA certificate key used to sign certificates for client communication.
	EtcdCAVolumeName = "etcd-ca"
	// EtcdServerTLSVolumeName is the name of the volume that contains the server certificate-key pair used to set up the etcd server and etcd-wrapper HTTP server.
	EtcdServerTLSVolumeName = "etcd-server-tls"
	// EtcdClientTLSVolumeName is the name of the volume that contains the client certificate-key pair used by the client to communicate to the etcd server and etcd-wrapper HTTP server.
	EtcdClientTLSVolumeName = "etcd-client-tls"
	// EtcdPeerCAVolumeName is the name of the volume that contains the CA certificate bundle and CA certificate key used to sign certificates for peer communication.
	EtcdPeerCAVolumeName = "etcd-peer-ca"
	// EtcdPeerServerTLSVolumeName is the name of the volume that contains the server certificate-key pair used to set up the peer server.
	EtcdPeerServerTLSVolumeName = "etcd-peer-server-tls"
	// BackupRestoreCAVolumeName is the name of the volume that contains the CA certificate bundle and CA certificate key used to sign certificates for backup-restore communication.
	BackupRestoreCAVolumeName = "backup-restore-ca"
	// BackupRestoreServerTLSVolumeName is the name of the volume that contains the server certificate-key pair used to set up the backup-restore server.
	BackupRestoreServerTLSVolumeName = "backup-restore-server-tls"
	// BackupRestoreClientTLSVolumeName is the name of the volume that contains the client certificate-key pair used by the client to communicate to the backup-restore server.
	BackupRestoreClientTLSVolumeName = "backup-restore-client-tls"

	// EtcdConfigVolumeName is the name of the volume that contains the etcd configuration file.
	EtcdConfigVolumeName = "etcd-config-file"
	// LocalBackupVolumeName is the name of the volume that contains the local backup.
	LocalBackupVolumeName = "local-backup"
	// ProviderBackupSecretVolumeName is the name of the volume that contains the provider backup secret.
	ProviderBackupSecretVolumeName = "etcd-backup-secret"
)

// constants for volume mount paths
const (
	// EtcdCAVolumeMountPath is the path on a container where the CA certificate bundle and CA certificate key used to sign certificates for client communication are mounted.
	EtcdCAVolumeMountPath = "/var/etcd/ssl/ca"
	// EtcdServerTLSVolumeMountPath is the path on a container where the server certificate-key pair used to set up the etcd server and etcd-wrapper HTTP server is mounted.
	EtcdServerTLSVolumeMountPath = "/var/etcd/ssl/server"
	// EtcdClientTLSVolumeMountPath is the path on a container where the client certificate-key pair used by the client to communicate to the etcd server and etcd-wrapper HTTP server is mounted.
	EtcdClientTLSVolumeMountPath = "/var/etcd/ssl/client"
	// EtcdPeerCAVolumeMountPath is the path on a container where the CA certificate bundle and CA certificate key used to sign certificates for peer communication are mounted.
	EtcdPeerCAVolumeMountPath = "/var/etcd/ssl/peer/ca"
	// EtcdPeerServerTLSVolumeMountPath is the path on a container where the server certificate-key pair used to set up the peer server is mounted.
	EtcdPeerServerTLSVolumeMountPath = "/var/etcd/ssl/peer/server"
	// BackupRestoreCAVolumeMountPath is the path on a container where the CA certificate bundle and CA certificate key used to sign certificates for backup-restore communication are mounted.
	BackupRestoreCAVolumeMountPath = "/var/etcdbr/ssl/ca"
	// BackupRestoreServerTLSVolumeMountPath is the path on a container where the server certificate-key pair used to set up the backup-restore server is mounted.
	BackupRestoreServerTLSVolumeMountPath = "/var/etcdbr/ssl/server"
	// BackupRestoreClientTLSVolumeMountPath is the path on a container where the client certificate-key pair used by the client to communicate to the backup-restore server is mounted.
	BackupRestoreClientTLSVolumeMountPath = "/var/etcdbr/ssl/client"

	// GCSBackupVolumeMountPath is the path on a container where the GCS backup secret is mounted.
	GCSBackupVolumeMountPath = "/var/.gcp/"
	// NonGCSProviderBackupVolumeMountPath is the path on a container where the non-GCS provider backup secret is mounted.
	NonGCSProviderBackupVolumeMountPath = "/var/etcd-backup"

	// EtcdDataVolumeMountPath is the path on a container where the etcd data directory is hosted.
	EtcdDataVolumeMountPath = "/var/etcd/data"
)
