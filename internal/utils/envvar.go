// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// GetEnvVarFromValue returns environment variable object with the provided name and value
func GetEnvVarFromValue(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}

// getEnvVarFromFieldPath returns environment variable object with provided name and value from field path
func getEnvVarFromFieldPath(name, fieldPath string) corev1.EnvVar {
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
				Optional: ptr.To(optional),
			},
		},
	}
}

// GetBackupRestoreContainerEnvVars returns non-provider-specific environment variables for the backup-restore container.
func GetBackupRestoreContainerEnvVars(etcd *druidv1alpha1.Etcd, store *druidv1alpha1.StoreSpec) ([]corev1.EnvVar, error) {
	var envVars []corev1.EnvVar

	if druidv1alpha1.IsPodManagementEnabled(etcd) {
		envVars = append(envVars, getEnvVarFromFieldPath(common.EnvPodName, "metadata.name"))
	} else {
		// Populate the POD_NAME as <etcd-name>-<pod-ip> in case of externally managed members.
		envVars = append(envVars, getEnvVarFromFieldPath(common.EnvPodIP, "status.podIP"))
		envVars = append(envVars, GetEnvVarFromValue(common.EnvPodName, fmt.Sprintf("%s-$(%s)", etcd.Name, common.EnvPodIP)))
	}
	envVars = append(envVars, getEnvVarFromFieldPath(common.EnvPodNamespace, "metadata.namespace"))

	if store == nil {
		return envVars, nil
	}

	storageContainer := ptr.Deref(store.Container, "")
	envVars = append(envVars, GetEnvVarFromValue(common.EnvStorageContainer, storageContainer))

	return envVars, nil
}
