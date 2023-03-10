// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
