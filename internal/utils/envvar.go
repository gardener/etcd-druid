// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

// GetEnvVarFromValue returns environment variable object with the provided name and value
func GetEnvVarFromValue(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}

// GetEnvVarFromFieldPath returns environment variable object with provided name and value from field path
func GetEnvVarFromFieldPath(name, fieldPath string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: fieldPath,
			},
		},
	}
}

// GetEnvVarFromSecret returns environment variable object with provided name and optional value from secret
func GetEnvVarFromSecret(name, secretName, secretKey string, optional bool) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      secretKey,
				Optional: pointer.Bool(optional),
			},
		},
	}
}

// GetProviderEnvVars returns provider-specific environment variables for the given store
func GetProviderEnvVars(store *druidv1alpha1.StoreSpec) ([]corev1.EnvVar, error) {
	var envVars []corev1.EnvVar

	provider, err := StorageProviderFromInfraProvider(store.Provider)
	if err != nil {
		return nil, fmt.Errorf("storage provider is not recognized while fetching secrets from environment variable")
	}

	const credentialsMountPath = "/var/etcd-backup"
	switch provider {
	case S3:
		envVars = append(envVars, GetEnvVarFromValue(common.EnvAWSApplicationCredentials, credentialsMountPath))

	case ABS:
		envVars = append(envVars, GetEnvVarFromValue(common.EnvAzureApplicationCredentials, credentialsMountPath))

	case GCS:
		envVars = append(envVars, GetEnvVarFromValue(common.EnvGoogleApplicationCredentials, "/var/.gcp/serviceaccount.json"))
		envVars = append(envVars, GetEnvVarFromSecret(common.EnvGoogleStorageAPIEndpoint, store.SecretRef.Name, "storageAPIEndpoint", true))

	case Swift:
		envVars = append(envVars, GetEnvVarFromValue(common.EnvOpenstackApplicationCredentials, credentialsMountPath))

	case OSS:
		envVars = append(envVars, GetEnvVarFromValue(common.EnvAlicloudApplicationCredentials, credentialsMountPath))

	case ECS:
		if store.SecretRef == nil {
			return nil, fmt.Errorf("no secretRef could be configured for backup store of ECS")
		}
		envVars = append(envVars, GetEnvVarFromSecret(common.EnvECSEndpoint, store.SecretRef.Name, "endpoint", false))
		envVars = append(envVars, GetEnvVarFromSecret(common.EnvECSAccessKeyID, store.SecretRef.Name, "accessKeyID", false))
		envVars = append(envVars, GetEnvVarFromSecret(common.EnvECSSecretAccessKey, store.SecretRef.Name, "secretAccessKey", false))

	case OCS:
		envVars = append(envVars, GetEnvVarFromValue(common.EnvOpenshiftApplicationCredentials, credentialsMountPath))
	}

	return envVars, nil
}

// GetBackupRestoreContainerEnvVars returns backup-restore container environment variables for the given store
func GetBackupRestoreContainerEnvVars(store *druidv1alpha1.StoreSpec) ([]corev1.EnvVar, error) {
	var envVars []corev1.EnvVar

	envVars = append(envVars, GetEnvVarFromFieldPath(common.EnvPodName, "metadata.name"))
	envVars = append(envVars, GetEnvVarFromFieldPath(common.EnvPodNamespace, "metadata.namespace"))

	if store == nil {
		return envVars, nil
	}

	storageContainer := pointer.StringDeref(store.Container, "")
	envVars = append(envVars, GetEnvVarFromValue(common.EnvStorageContainer, storageContainer))

	providerEnvVars, err := GetProviderEnvVars(store)
	if err != nil {
		return nil, err
	}
	envVars = append(envVars, providerEnvVars...)

	return envVars, nil
}
