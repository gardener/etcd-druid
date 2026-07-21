//go:build local

// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func init() {
	localProviderInitContainer = func(etcd *druidv1alpha1.Etcd, etcdBackupVolumeMount *corev1.VolumeMount, initContainerImage string, provider *string) []corev1.Container {
		return []corev1.Container{{
			Name:            common.InitContainerNameChangeBackupBucketPermissions,
			Image:           initContainerImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"sh", "-c", "--"},
			Args:            []string{fmt.Sprintf("chown -R %d:%d %s", nonRootUser, nonRootUser, kubernetes.MountPathLocalStore(etcd, provider))},
			VolumeMounts:    []corev1.VolumeMount{*etcdBackupVolumeMount},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				RunAsGroup:               ptr.To[int64](0),
				RunAsNonRoot:             ptr.To(false),
				RunAsUser:                ptr.To[int64](0),
			},
		}}
	}
}
