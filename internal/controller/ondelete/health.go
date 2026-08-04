// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"github.com/gardener/etcd-druid/internal/common"

	corev1 "k8s.io/api/core/v1"
)

// containerState classifies the etcd container's runtime state into buckets for step 2 of the pod update procedure relies on the sub-priorities.
type containerState int

const (
	containerStateUnknown containerState = iota
	containerStateDead
	containerStateTransient
	containerStateAlive
)

var waitingReasonsDead = map[string]struct{}{
	"CrashLoopBackOff":           {},
	"RunContainerError":          {},
	"ImagePullBackOff":           {},
	"ErrImagePull":               {},
	"CreateContainerConfigError": {},
}

var waitingReasonsTransient = map[string]struct{}{
	"ContainerCreating": {},
	"PodInitializing":   {},
}

// classifyEtcdContainer classifies the etcd container's state as per the health assessment done in docs/proposals/07-quorum-aware-pod-updates.md.
func classifyEtcdContainer(pod *corev1.Pod) containerState {
	status := findEtcdContainerStatus(pod)
	if status == nil {
		return containerStateUnknown
	}
	if status.State.Terminated != nil {
		return containerStateDead
	}
	if status.State.Waiting != nil {
		reason := status.State.Waiting.Reason
		if _, ok := waitingReasonsDead[reason]; ok {
			return containerStateDead
		}
		if _, ok := waitingReasonsTransient[reason]; ok {
			return containerStateTransient
		}
		// Unrecognised Waiting reason: marking transiet to be on the side of "still coming up".
		return containerStateTransient
	}
	if status.State.Running != nil {
		return containerStateAlive
	}
	return containerStateUnknown
}

// isParticipating reports whether the etcd container is contributing to quorum or not using its Ready field which is a linearizable call.
func isParticipating(pod *corev1.Pod) bool {
	status := findEtcdContainerStatus(pod)
	if status == nil {
		return false
	}
	return status.Ready
}

func findEtcdContainerStatus(pod *corev1.Pod) *corev1.ContainerStatus {
	for i := range pod.Status.ContainerStatuses {
		if pod.Status.ContainerStatuses[i].Name == common.ContainerNameEtcd {
			return &pod.Status.ContainerStatuses[i]
		}
	}
	return nil
}
