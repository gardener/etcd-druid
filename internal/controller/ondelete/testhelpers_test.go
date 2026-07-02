// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"github.com/gardener/etcd-druid/internal/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// Shared test-fixture helpers for the ondelete package. Kept minimal so each
// test can override only the fields it cares about.

// stsFixture builds a StatefulSet ready to be indexed by the label selector
// getStsLabels() emits. updateRevision drives what pods are compared against.
func stsFixture(name, namespace, updateRevision string, replicas int32) *appsv1.StatefulSet {
	sel := &metav1.LabelSelector{MatchLabels: getStsLabels(name)}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(replicas),
			Selector: sel,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.OnDeleteStatefulSetStrategyType,
			},
		},
		Status: appsv1.StatefulSetStatus{UpdateRevision: updateRevision},
	}
}

// makeStsPod builds a pod that carries the labels of a StatefulSet with the
// given base name plus the controller-revision-hash label at revision.
// participating controls the etcd container's Ready flag.
func makeStsPod(name, namespace, revision string, participating bool) *corev1.Pod {
	labels := getStsLabels(baseNameFromPod(name))
	labels[appsv1.StatefulSetRevisionLabel] = revision
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  common.ContainerNameEtcd,
					Ready: participating,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
		},
	}
}

// withContainerState replaces the etcd container's state on the pod, useful for
// classifier tests where the Running default of makeStsPod would give the wrong
// bucket.
func withContainerState(pod *corev1.Pod, state corev1.ContainerState) *corev1.Pod {
	for i := range pod.Status.ContainerStatuses {
		if pod.Status.ContainerStatuses[i].Name == common.ContainerNameEtcd {
			pod.Status.ContainerStatuses[i].State = state
			return pod
		}
	}
	return pod
}

// withDeletionTimestamp marks the pod as terminating.
func withDeletionTimestamp(pod *corev1.Pod) *corev1.Pod {
	now := metav1.Now()
	pod.DeletionTimestamp = &now
	pod.Finalizers = []string{"kubernetes"}
	return pod
}

// stsRef builds a NamespacedName pointing at an sts of the given name.
func stsRef(name, namespace string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: namespace}
}

// getStsLabels returns the label set the ondelete tests use for both STS
// selectors and pod labels. The set is intentionally minimal — it just needs
// a non-empty selector that groups the fixture pods together.
func getStsLabels(baseName string) map[string]string {
	return map[string]string{"ondelete-test/sts": baseName}
}

// baseNameFromPod strips the "-<ordinal>" suffix from a pod name so its labels
// match the parent STS. If no suffix is present, the pod name is returned as-is.
func baseNameFromPod(podName string) string {
	for i := len(podName) - 1; i >= 0; i-- {
		if podName[i] == '-' {
			return podName[:i]
		}
	}
	return podName
}
