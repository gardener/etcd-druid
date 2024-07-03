// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func OwnerRefIterator(element interface{}) string {
	return (element.(metav1.OwnerReference)).Name
}

func VolumeMountIterator(element interface{}) string {
	return (element.(corev1.VolumeMount)).Name
}

func VolumeIterator(element interface{}) string {
	return (element.(corev1.Volume)).Name
}

func EnvIterator(element interface{}) string {
	return (element.(corev1.EnvVar)).Name
}

func ContainerIterator(element interface{}) string {
	return (element.(corev1.Container)).Name
}

func CmdIterator(element interface{}) string {
	return element.(string)
}
