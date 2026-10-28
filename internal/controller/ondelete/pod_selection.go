// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"

	corev1 "k8s.io/api/core/v1"
)

// etcdContainerReadiness is the readiness of a pod's etcd container, derived
// from the kubelet-observed (and therefore possibly lagging) Ready field. Only
// three categories matter for choosing the next pod to delete.
type etcdContainerReadiness int

const (
	// etcdContainerNotReady means the etcd container has a published status and
	// its readiness probe is not succeeding. Such a pod is not participating in
	// quorum, so it is the safest to delete first.
	etcdContainerNotReady etcdContainerReadiness = iota
	// etcdContainerReadinessUnknown means Kubernetes has not yet published a
	// status for the etcd container (pod initializing, or kubelet status has not
	// reached the controller cache yet). Treated conservatively to not cause artificial harm to the cluster.
	etcdContainerReadinessUnknown
	// etcdContainerReady means the etcd container's readiness probe is succeeding.
	etcdContainerReady
)

// memberRolePriority orders roles for deletion among participating (ready or
// unknown-readiness) pods: a leader is deleted last so its removal, which forces
// a leader election, happens only after all followers have been updated. The
// role is best-effort, derived from the live member list; when it cannot be
// determined a pod is treated as a follower (memberRolePriorityUnknown).
type memberRolePriority int

const (
	memberRolePriorityLearner memberRolePriority = iota
	memberRolePriorityFollower
	memberRolePriorityUnknown
	memberRolePriorityLeader
)

// podUpdateCandidate is an outdated pod annotated with the two signals used to
// order deletions: the etcd container's readiness and the member's role.
type podUpdateCandidate struct {
	pod          *corev1.Pod
	readiness    etcdContainerReadiness
	rolePriority memberRolePriority
}

// selectNextPodForUpdate returns the outdated pod that should be deleted next,
// or nil when outdatedPods is empty.
func selectNextPodForUpdate(etcd *druidv1alpha1.Etcd, outdatedPods []corev1.Pod, members etcdmember.Members) *corev1.Pod {
	candidates := buildPodUpdateCandidates(etcd, outdatedPods, members)
	if len(candidates) == 0 {
		return nil
	}
	return slices.MinFunc(candidates, comparePodUpdateCandidates).pod
}

// buildPodUpdateCandidates builds one candidate per outdated pod, resolving each
// pod's role from the member list by etcd member name.
func buildPodUpdateCandidates(etcd *druidv1alpha1.Etcd, outdatedPods []corev1.Pod, members etcdmember.Members) []podUpdateCandidate {
	rolesByMemberName := make(map[string]memberRolePriority, len(members))
	for _, member := range members {
		rolesByMemberName[member.Name] = rolePriorityOf(member.Role)
	}
	candidates := make([]podUpdateCandidate, 0, len(outdatedPods))
	for i := range outdatedPods {
		pod := &outdatedPods[i]
		memberName := druidv1alpha1.GetMemberName(etcd.Spec.MemberNamePrefix, pod.Name)
		role, found := rolesByMemberName[memberName]
		if !found {
			// No live member for this pod (e.g. not yet joined): treat as a
			// follower; the quorum check still gates the eventual deletion.
			role = memberRolePriorityUnknown
		}
		candidates = append(candidates, podUpdateCandidate{
			pod:          pod,
			readiness:    readinessOfEtcdContainer(pod),
			rolePriority: role,
		})
	}
	return candidates
}

// comparePodUpdateCandidates is the total ordering used to pick the next pod to
// delete. Lower (more negative) sorts first, so the chosen pod is the MinFunc:
//
//  1. Not-ready pods come first: they are not participating in quorum, so
//     deleting them is safe regardless of role.
//  2. Among not-ready pods role is irrelevant, so order by descending ordinal for
//     determinism.
//  3. Among ready/unknown-readiness pods, role decides: learner, then follower,
//     then unknown, then leader, so the leader is updated last.
//  4. Ties break on readiness (update the less-certain pod earlier) and then on
//     descending ordinal.
func comparePodUpdateCandidates(pod1, pod2 podUpdateCandidate) int {
	pod1NotReady := pod1.readiness == etcdContainerNotReady
	pod2NotReady := pod2.readiness == etcdContainerNotReady
	if pod1NotReady != pod2NotReady {
		if pod1NotReady {
			return -1
		}
		return 1
	}
	if pod1NotReady {
		// Both not ready: role does not matter, prefer the highest ordinal.
		return compareStatefulSetPodOrdinalsDescending(pod1.pod, pod2.pod)
	}
	// Both ready or unknown-readiness: leader last.
	if result := cmp.Compare(pod1.rolePriority, pod2.rolePriority); result != 0 {
		return result
	}
	// Same role: prefer the less-certain (unknown) readiness, then highest ordinal.
	if result := cmp.Compare(pod1.readiness, pod2.readiness); result != 0 {
		return result
	}
	return compareStatefulSetPodOrdinalsDescending(pod1.pod, pod2.pod)
}

// compareStatefulSetPodOrdinalsDescending orders two StatefulSet pods so the
// higher ordinal sorts first (returns negative). Pods whose ordinal cannot be
// parsed sort last among their group, by name, so the ordering stays total and
// deterministic.
func compareStatefulSetPodOrdinalsDescending(pod1, pod2 *corev1.Pod) int {
	pod1Ordinal, pod1Parsed := statefulSetPodOrdinal(pod1.Name)
	pod2Ordinal, pod2Parsed := statefulSetPodOrdinal(pod2.Name)
	switch {
	case pod1Parsed && pod2Parsed:
		return cmp.Compare(pod2Ordinal, pod1Ordinal) // descending
	case pod1Parsed:
		return -1
	case pod2Parsed:
		return 1
	default:
		return cmp.Compare(pod1.Name, pod2.Name)
	}
}

// statefulSetPodOrdinal extracts the trailing "-<n>" ordinal from a StatefulSet
// pod name, returning the ordinal and whether it parsed.
func statefulSetPodOrdinal(podName string) (int, bool) {
	idx := strings.LastIndex(podName, "-")
	if idx < 0 || idx == len(podName)-1 {
		return 0, false
	}
	n, err := strconv.Atoi(podName[idx+1:])
	if err != nil {
		return 0, false
	}
	return n, true
}

// readinessOfEtcdContainer categorizes the etcd container's readiness into not-ready/unknown/ready.
func readinessOfEtcdContainer(pod *corev1.Pod) etcdContainerReadiness {
	status := findEtcdContainerStatus(pod)
	if status == nil {
		return etcdContainerReadinessUnknown
	}
	if status.Ready {
		return etcdContainerReady
	}
	return etcdContainerNotReady
}

// rolePriorityOf maps an etcd member role to its deletion priority.
func rolePriorityOf(role etcdmember.MemberRole) memberRolePriority {
	switch role {
	case etcdmember.MemberRoleLearner:
		return memberRolePriorityLearner
	case etcdmember.MemberRoleLeader:
		return memberRolePriorityLeader
	case etcdmember.MemberRoleMember:
		return memberRolePriorityFollower
	default:
		return memberRolePriorityUnknown
	}
}

// findEtcdContainerStatus returns the etcd container's status, or nil if absent.
func findEtcdContainerStatus(pod *corev1.Pod) *corev1.ContainerStatus {
	for i := range pod.Status.ContainerStatuses {
		if pod.Status.ContainerStatuses[i].Name == common.ContainerNameEtcd {
			return &pod.Status.ContainerStatuses[i]
		}
	}
	return nil
}
