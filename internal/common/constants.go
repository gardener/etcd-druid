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
	// ChartPath is the directory containing the default image vector file.
	ChartPath = "charts"
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
	ClientServiceComponentName       = "etcd-client-service"
	ConfigMapComponentName           = "etcd-config"
	MemberLeaseComponentName         = "etcd-member-lease"
	SnapshotLeaseComponentName       = "etcd-snapshot-lease"
	PeerServiceComponentName         = "etcd-peer-service"
	PodDisruptionBudgetComponentName = "etcd-pdb"
	RoleComponentName                = "etcd-druid-role"
	RoleBindingComponentName         = "druid-role-binding"
	ServiceAccountComponentName      = "druid-service-account"
	StatefulSetComponentName         = "etcd-sts"
)
