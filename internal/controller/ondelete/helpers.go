// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// isPodOutdated reports whether the pod's controller-revision-hash differs
// from sts.Status.UpdateRevision. Empty target yields false so callers do not
// act before the STS controller populates it. The upstream label contract is
// not formally guaranteed; revisit if the STS controller changes its
// update-tracking (mirror change also needed in AreAllStsPodsAtUpdateRevision).
func isPodOutdated(pod *corev1.Pod, sts *appsv1.StatefulSet) bool {
	if sts.Status.UpdateRevision == "" {
		return false
	}
	return pod.Labels[appsv1.StatefulSetRevisionLabel] != sts.Status.UpdateRevision
}

// isSingleNode reports whether the StatefulSet models a single-node etcd
// cluster (spec.replicas == 1). See DEP-07 Single-Node Clusters.
func isSingleNode(sts *appsv1.StatefulSet) bool {
	if sts.Spec.Replicas == nil {
		return false
	}
	return *sts.Spec.Replicas == 1
}
