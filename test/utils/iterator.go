// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func OwnerRefIterator(element interface{}) string {
	return (element.(metav1.OwnerReference)).Name
}

func ServicePortIterator(element interface{}) string {
	return (element.(corev1.ServicePort)).Name
}

func VolumeMountIterator(element interface{}) string {
	return (element.(corev1.VolumeMount)).Name
}

func VolumeIterator(element interface{}) string {
	return (element.(corev1.Volume)).Name
}

func KeyIterator(element interface{}) string {
	return (element.(corev1.KeyToPath)).Key
}

func EnvIterator(element interface{}) string {
	return (element.(corev1.EnvVar)).Name
}

func ContainerIterator(element interface{}) string {
	return (element.(corev1.Container)).Name
}

func HostAliasIterator(element interface{}) string {
	return (element.(corev1.HostAlias)).IP
}

func PVCIterator(element interface{}) string {
	return (element.(corev1.PersistentVolumeClaim)).Name
}

func AccessModeIterator(element interface{}) string {
	return string(element.(corev1.PersistentVolumeAccessMode))
}

func CmdIterator(element interface{}) string {
	return element.(string)
}

func RuleIterator(element interface{}) string {
	return element.(rbacv1.PolicyRule).APIGroups[0]
}

func StringArrayIterator(element interface{}) string {
	return element.(string)
}
