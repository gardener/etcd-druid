// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

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
	// DefaultImageVectorFilePath is the path to the default image vector file.
	DefaultImageVectorFilePath = "charts/images.yaml"
	// GardenerOwnedBy is a constant for an annotation on a resource that describes the owner resource.
	GardenerOwnedBy = "gardener.cloud/owned-by"
	// GardenerOwnerType is a constant for an annotation on a resource that describes the type of owner resource.
	GardenerOwnerType = "gardener.cloud/owner-type"
	// FinalizerName is the name of the etcd finalizer.
	FinalizerName = "druid.gardener.cloud/etcd-druid"

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
