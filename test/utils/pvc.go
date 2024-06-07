// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreatePVC creates a PVC for the given StatefulSet pod.
func CreatePVC(sts *appsv1.StatefulSet, podName string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	if sts == nil {
		return nil
	}
	if sts.Spec.VolumeClaimTemplates == nil || len(sts.Spec.VolumeClaimTemplates) == 0 {
		return nil
	}

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", sts.Spec.VolumeClaimTemplates[0].Name, podName),
			Namespace: sts.Namespace,
		},
		Spec: sts.Spec.VolumeClaimTemplates[0].Spec,
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}
}
