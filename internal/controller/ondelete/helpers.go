// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// isPodOutdated reports whether the pod's controller-revision-hash differs from sts.Status.UpdateRevision.
func isPodOutdated(pod *corev1.Pod, sts *appsv1.StatefulSet) bool {
	if sts.Status.UpdateRevision == "" {
		return false
	}
	return pod.Labels[appsv1.StatefulSetRevisionLabel] != sts.Status.UpdateRevision
}
