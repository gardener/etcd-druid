// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import corev1 "k8s.io/api/core/v1"

// MatchPodConditions checks if the specified condition type and status are present in the given pod conditions.
func MatchPodConditions(conditions []corev1.PodCondition, condType corev1.PodConditionType, condStatus corev1.ConditionStatus) bool {
	for _, condition := range conditions {
		if condition.Type == condType && condition.Status == condStatus {
			return true
		}
	}
	return false
}

// HasPodReadyConditionTrue checks if the pod has a Ready condition with status True.
func HasPodReadyConditionTrue(pod *corev1.Pod) bool {
	return MatchPodConditions(pod.Status.Conditions, corev1.PodReady, corev1.ConditionTrue)
}
