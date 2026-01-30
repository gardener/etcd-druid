// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

const (
	// CheckSumKeyConfigMap is the key that is set by a configmap component and used by StatefulSet component to
	// place an annotation on the StatefulSet pods. The value contains the check-sum of the latest configmap that
	// should be reflected on the pods.
	CheckSumKeyConfigMap = "checksum/etcd-configmap"
)

// LeaseAnnotationKeyPeerURLTLSEnabled is the annotation key present on the member lease.
// If its value is `true` then it indicates that the member is TLS enabled.
// If the annotation is not present or its value is `false` then it indicates that the member is not TLS enabled.
const LeaseAnnotationKeyPeerURLTLSEnabled = "member.etcd.gardener.cloud/tls-enabled"

// Constants for image keys
const (
	// ImageKeyEtcd is the key for the etcd image in the image vector.
	ImageKeyEtcd = "etcd"
	// ImageKeyEtcdBackupRestore is the key for the etcd-backup-restore image in the image vector.
	ImageKeyEtcdBackupRestore = "etcd-backup-restore"
	// ImageKeyEtcdWrapper is the key for the etcd image in the image vector.
	ImageKeyEtcdWrapper = "etcd-wrapper"
	// ImageKeyEtcdBackupRestoreDistroless is the key for the etcd-backup-restore image in the image vector.
	ImageKeyEtcdBackupRestoreDistroless = "etcd-backup-restore-distroless"
	// ImageKeyAlpine is the key for the alpine image in the image vector.
	ImageKeyAlpine = "alpine"
)

// Constants for container names
const (
	// ContainerNameEtcd is the name of the etcd container.
	ContainerNameEtcd = "etcd"
	// ContainerNameEtcdBackupRestore is the name of the backup-restore container.
	ContainerNameEtcdBackupRestore = "backup-restore"
	// InitContainerNameChangePermissions is the name of the change permissions init container.
	InitContainerNameChangePermissions = "change-permissions"
	// InitContainerNameChangeBackupBucketPermissions is the name of the change backup bucket permissions init container.
	InitContainerNameChangeBackupBucketPermissions = "change-backup-bucket-permissions"
)

// Constants for ports
const (
	// DefaultPortEtcdPeer is the default port for the etcd server used for peer communication.
	DefaultPortEtcdPeer int32 = 2380
	// DefaultPortEtcdClient is the default port for the etcd client.
	DefaultPortEtcdClient int32 = 2379
	// DefaultPortEtcdWrapper is the default port for the etcd-wrapper HTTP server.
	DefaultPortEtcdWrapper int32 = 9095
	// DefaultPortEtcdBackupRestore is the default port for the HTTP server in the etcd-backup-restore container.
	DefaultPortEtcdBackupRestore int32 = 8080
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
	EnvAzureApplicationCredentials = "AZURE_APPLICATION_CREDENTIALS" // #nosec G101 -- this is the name of an env var, and not the credential itself.
	// EnvGoogleApplicationCredentials is the environment variable key for Google application credentials.
	EnvGoogleApplicationCredentials = "GOOGLE_APPLICATION_CREDENTIALS" // #nosec G101 -- this is the name of an env var, and not the credential itself.
	// EnvOpenstackApplicationCredentials is the environment variable key for OpenStack application credentials.
	EnvOpenstackApplicationCredentials = "OPENSTACK_APPLICATION_CREDENTIALS" // #nosec G101 -- this is the name of an env var, and not the credential itself.
	// EnvAlicloudApplicationCredentials is the environment variable key for Alicloud application credentials.
	EnvAlicloudApplicationCredentials = "ALICLOUD_APPLICATION_CREDENTIALS" // #nosec G101 -- this is the name of an env var, and not the credential itself.
	// EnvOpenshiftApplicationCredentials is the environment variable key for OpenShift application credentials.
	EnvOpenshiftApplicationCredentials = "OPENSHIFT_APPLICATION_CREDENTIALS" // #nosec G101 -- this is the name of an env var, and not the credential itself.
	// EnvECSEndpoint is the environment variable key for Dell ECS endpoint.
	EnvECSEndpoint = "ECS_ENDPOINT"
	// EnvECSAccessKeyID is the environment variable key for Dell ECS access key ID.
	EnvECSAccessKeyID = "ECS_ACCESS_KEY_ID"
	// EnvECSSecretAccessKey is the environment variable key for Dell ECS secret access key.
	EnvECSSecretAccessKey = "ECS_SECRET_ACCESS_KEY" // #nosec G101 -- this is the name of an env var, and not the credential itself.
)

// Constants for values to be set against druidv1alpha1.LabelComponentKey
const (
	// ComponentNameClientService is the component name for client service resource.
	ComponentNameClientService = "etcd-client-service"
	// ComponentNameConfigMap is the component  name for config map resource.
	ComponentNameConfigMap = "etcd-configmap"
	// ComponentNameMemberLease is the component name for member lease resource.
	ComponentNameMemberLease = "etcd-member-lease"
	// ComponentNameSnapshotLease is the component name for snapshot lease resource.
	ComponentNameSnapshotLease = "etcd-snapshot-lease"
	// ComponentNamePeerService is the component name for peer service resource.
	ComponentNamePeerService = "etcd-peer-service"
	// ComponentNamePodDisruptionBudget is the component name for pod disruption budget resource.
	ComponentNamePodDisruptionBudget = "etcd-pdb"
	// ComponentNameRole is the component name for role resource.
	ComponentNameRole = "etcd-role"
	// ComponentNameRoleBinding is the component name for role binding resource.
	ComponentNameRoleBinding = "etcd-role-binding"
	// ComponentNameServiceAccount is the component name for service account resource.
	ComponentNameServiceAccount = "etcd-service-account"
	// ComponentNameStatefulSet is the component name for statefulset resource.
	ComponentNameStatefulSet = "etcd-statefulset"
	// ComponentNameSnapshotCompactionJob is the component name for snapshot compaction job resource.
	ComponentNameSnapshotCompactionJob = "etcd-snapshot-compaction-job"
	// ComponentNameEtcdCopyBackupsJob is the component name for copy-backup task resource.
	ComponentNameEtcdCopyBackupsJob = "etcd-copy-backups-job"
)

// Constants for volume names
const (
	// VolumeNameEtcdCA is the name of the volume that contains the CA certificate bundle and CA certificate key used to sign certificates for client communication.
	VolumeNameEtcdCA = "etcd-ca"
	// VolumeNameEtcdServerTLS is the name of the volume that contains the server certificate-key pair used to set up the etcd server and etcd-wrapper HTTP server.
	VolumeNameEtcdServerTLS = "etcd-server-tls"
	// VolumeNameEtcdClientTLS is the name of the volume that contains the client certificate-key pair used by the client to communicate to the etcd server and etcd-wrapper HTTP server.
	VolumeNameEtcdClientTLS = "etcd-client-tls"
	// VolumeNameEtcdPeerCA is the name of the volume that contains the CA certificate bundle and CA certificate key used to sign certificates for peer communication.
	VolumeNameEtcdPeerCA = "etcd-peer-ca"
	// OldVolumeNameEtcdPeerCA is the old name of the volume that contains the CA certificate bundle and CA certificate key used to sign certificates for peer communication.
	// TODO: (i062009) remove this when all clusters have started to use the new volume names.
	OldVolumeNameEtcdPeerCA = "peer-url-ca-etcd"
	// VolumeNameEtcdPeerServerTLS is the name of the volume that contains the server certificate-key pair used to set up the peer server.
	VolumeNameEtcdPeerServerTLS = "etcd-peer-server-tls"
	// OldVolumeNameEtcdPeerServerTLS is the old name of the volume that contains the server certificate-key pair used to set up the peer server.
	// TODO: (i062009) remove this when all clusters have started to use the new volume names.
	OldVolumeNameEtcdPeerServerTLS = "peer-url-etcd-server-tls"
	// VolumeNameBackupRestoreCA is the name of the volume that contains the CA certificate bundle and CA certificate key used to sign certificates for backup-restore communication.
	VolumeNameBackupRestoreCA = "backup-restore-ca"
	// VolumeNameBackupRestoreServerTLS is the name of the volume that contains the server certificate-key pair used to set up the backup-restore server.
	VolumeNameBackupRestoreServerTLS = "backup-restore-server-tls"
	// VolumeNameBackupRestoreClientTLS is the name of the volume that contains the client certificate-key pair used by the client to communicate to the backup-restore server.
	VolumeNameBackupRestoreClientTLS = "backup-restore-client-tls"

	// VolumeNameEtcdConfig is the name of the volume that contains the etcd configuration file.
	VolumeNameEtcdConfig = "etcd-config-file"
	// VolumeNameLocalBackup is the name of the volume that contains the local backup.
	VolumeNameLocalBackup = "local-backup"
	// VolumeNameProviderBackupSecret is the name of the volume that contains the provider backup secret.
	VolumeNameProviderBackupSecret = "etcd-backup-secret" // #nosec G101 -- this is the name of the mounted volume for backup secret, and not the credential itself.
)

// EtcdConfigFileName is the name of the etcd configuration file.
const EtcdConfigFileName = "etcd.conf.yaml"

// ModeOwnerReadWriteGroupRead is the file permissions used for volumes
const ModeOwnerReadWriteGroupRead int32 = 0640

// constants for volume mount paths
const (
	// VolumeMountPathEtcdCA is the path on a container where the CA certificate bundle and CA certificate key used to sign certificates for client communication are mounted.
	VolumeMountPathEtcdCA = "/var/etcd/ssl/ca"
	// VolumeMountPathEtcdServerTLS is the path on a container where the server certificate-key pair used to set up the etcd server and etcd-wrapper HTTP server is mounted.
	VolumeMountPathEtcdServerTLS = "/var/etcd/ssl/server"
	// VolumeMountPathEtcdClientTLS is the path on a container where the client certificate-key pair used by the client to communicate to the etcd server and etcd-wrapper HTTP server is mounted.
	VolumeMountPathEtcdClientTLS = "/var/etcd/ssl/client"
	// VolumeMountPathEtcdPeerCA is the path on a container where the CA certificate bundle and CA certificate key used to sign certificates for peer communication are mounted.
	VolumeMountPathEtcdPeerCA = "/var/etcd/ssl/peer/ca"
	// VolumeMountPathEtcdPeerServerTLS is the path on a container where the server certificate-key pair used to set up the peer server is mounted.
	VolumeMountPathEtcdPeerServerTLS = "/var/etcd/ssl/peer/server"
	// VolumeMountPathBackupRestoreCA is the path on a container where the CA certificate bundle and CA certificate key used to sign certificates for backup-restore communication are mounted.
	VolumeMountPathBackupRestoreCA = "/var/etcdbr/ssl/ca"
	// VolumeMountPathBackupRestoreServerTLS is the path on a container where the server certificate-key pair used to set up the backup-restore server is mounted.
	VolumeMountPathBackupRestoreServerTLS = "/var/etcdbr/ssl/server"
	// VolumeMountPathBackupRestoreClientTLS is the path on a container where the client certificate-key pair used by the client to communicate to the backup-restore server is mounted.
	VolumeMountPathBackupRestoreClientTLS = "/var/etcdbr/ssl/client"

	// VolumeMountPathGCSBackupSecret is the path on a container where the GCS backup secret is mounted.
	VolumeMountPathGCSBackupSecret = "/var/.gcp/" // #nosec G101 -- this is a path to the GCP backup credentials file, and not the credential itself.
	// VolumeMountPathNonGCSProviderBackupSecret is the path on a container where the non-GCS provider backup secret is mounted.
	VolumeMountPathNonGCSProviderBackupSecret = "/var/etcd-backup" // #nosec G101 -- this is a path to the backup credentials dir, and not the credential itself.

	// VolumeMountPathEtcdData is the path on a container where the etcd data directory is mounted.
	VolumeMountPathEtcdData = "/var/etcd/data"
)
