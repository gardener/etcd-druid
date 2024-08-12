// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LocalProviderDefaultMountPath is the default path where the buckets directory is mounted.
	LocalProviderDefaultMountPath = "/etc/gardener/local-backupbuckets"
	// EtcdBackupSecretHostPath is the hostPath field in the etcd-backup secret.
	EtcdBackupSecretHostPath = "hostPath"
)

const (
	aws       = "aws"
	azure     = "azure"
	gcp       = "gcp"
	alicloud  = "alicloud"
	openstack = "openstack"
	dell      = "dell"
	openshift = "openshift"
	stackit   = "stackit"
)

const (
	// S3 is a constant for the AWS and S3 compliant storage provider.
	S3 = "S3"
	// ABS is a constant for the Azure storage provider.
	ABS = "ABS"
	// GCS is a constant for the Google storage provider.
	GCS = "GCS"
	// OSS is a constant for the Alicloud storage provider.
	OSS = "OSS"
	// Swift is a constant for the OpenStack storage provider.
	Swift = "Swift"
	// Local is a constant for the Local storage provider.
	Local = "Local"
	// ECS is a constant for the EMC storage provider.
	ECS = "ECS"
	// OCS is a constant for the OpenShift storage provider.
	OCS = "OCS"
)

// StorageProviderFromInfraProvider converts infra to object store provider.
func StorageProviderFromInfraProvider(infra *druidv1alpha1.StorageProvider) (string, error) {
	if infra == nil {
		return "", nil
	}

	switch *infra {
	case aws, S3:
		return S3, nil
	// S3-compatible providers
	case stackit:
		return S3, nil
	case azure, ABS:
		return ABS, nil
	case alicloud, OSS:
		return OSS, nil
	case openstack, Swift:
		return Swift, nil
	case gcp, GCS:
		return GCS, nil
	case dell, ECS:
		return ECS, nil
	case openshift, OCS:
		return OCS, nil
	case Local, druidv1alpha1.StorageProvider(strings.ToLower(Local)):
		return Local, nil
	default:
		return "", fmt.Errorf("unsupported storage provider: '%v'", *infra)
	}
}

// GetHostMountPathFromSecretRef returns the hostPath configured for the given store.
func GetHostMountPathFromSecretRef(ctx context.Context, client client.Client, logger logr.Logger, store *druidv1alpha1.StoreSpec, namespace string) (string, error) {
	if store.SecretRef == nil {
		logger.Info("secretRef is not defined for store, using default hostPath", "namespace", namespace)
		return LocalProviderDefaultMountPath, nil
	}

	secret := &corev1.Secret{}
	if err := client.Get(ctx, utils.Key(namespace, store.SecretRef.Name), secret); err != nil {
		return "", err
	}

	hostPath, ok := secret.Data[EtcdBackupSecretHostPath]
	if !ok {
		return LocalProviderDefaultMountPath, nil
	}

	return string(hostPath), nil
}

// GetProviderEnvVars returns provider-specific environment variables for the given store.
func GetProviderEnvVars(store *druidv1alpha1.StoreSpec) ([]corev1.EnvVar, error) {
	if store == nil {
		return nil, nil
	}

	var envVars []corev1.EnvVar

	provider, err := StorageProviderFromInfraProvider(store.Provider)
	if err != nil {
		return nil, fmt.Errorf("storage provider is not recognized while fetching secrets from environment variable")
	}

	switch provider {
	case S3:
		envVars = append(envVars, utils.GetEnvVarFromValue(common.EnvAWSApplicationCredentials, common.VolumeMountPathNonGCSProviderBackupSecret))

	case ABS:
		envVars = append(envVars, utils.GetEnvVarFromValue(common.EnvAzureApplicationCredentials, common.VolumeMountPathNonGCSProviderBackupSecret))
		envVars = append(envVars, utils.GetEnvVarFromSecret(common.EnvAzureEmulatorEnabled, store.SecretRef.Name, "emulatorEnabled", true))

	case GCS:
		envVars = append(envVars, utils.GetEnvVarFromValue(common.EnvGoogleApplicationCredentials, fmt.Sprintf("%sserviceaccount.json", common.VolumeMountPathGCSBackupSecret)))

	case Swift:
		envVars = append(envVars, utils.GetEnvVarFromValue(common.EnvOpenstackApplicationCredentials, common.VolumeMountPathNonGCSProviderBackupSecret))

	case OSS:
		envVars = append(envVars, utils.GetEnvVarFromValue(common.EnvAlicloudApplicationCredentials, common.VolumeMountPathNonGCSProviderBackupSecret))

	case ECS:
		if store.SecretRef == nil {
			return nil, fmt.Errorf("no secretRef could be configured for backup store of ECS")
		}
		envVars = append(envVars, utils.GetEnvVarFromSecret(common.EnvECSEndpoint, store.SecretRef.Name, "endpoint", false))
		envVars = append(envVars, utils.GetEnvVarFromSecret(common.EnvECSAccessKeyID, store.SecretRef.Name, "accessKeyID", false))
		envVars = append(envVars, utils.GetEnvVarFromSecret(common.EnvECSSecretAccessKey, store.SecretRef.Name, "secretAccessKey", false))

	case OCS:
		envVars = append(envVars, utils.GetEnvVarFromValue(common.EnvOpenshiftApplicationCredentials, common.VolumeMountPathNonGCSProviderBackupSecret))
	}

	return envVars, nil
}
