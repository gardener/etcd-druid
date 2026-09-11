// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreatePVC creates a PVC for the given StatefulSet pod with the default druid
// labels so that label-selector queries in tests match real cluster behaviour.
func CreatePVC(sts *appsv1.StatefulSet, podName string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	if sts == nil {
		return nil
	}
	if len(sts.Spec.VolumeClaimTemplates) == 0 {
		return nil
	}

	// Mirror the two labels that druid stamps on all managed resources so that
	// MatchingLabels(GetDefaultLabels(etcd.ObjectMeta)) finds these PVCs.
	labels := map[string]string{
		druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
	}
	if partOf, ok := sts.Labels[druidv1alpha1.LabelPartOfKey]; ok {
		labels[druidv1alpha1.LabelPartOfKey] = partOf
	}

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", sts.Spec.VolumeClaimTemplates[0].Name, podName),
			Namespace: sts.Namespace,
			Labels:    labels,
		},
		Spec: sts.Spec.VolumeClaimTemplates[0].Spec,
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}
}
