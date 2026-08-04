// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"testing"

	"github.com/gardener/etcd-druid/internal/common"

	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/gomega"
)

func TestClassifyEtcdContainer(t *testing.T) {
	tests := []struct {
		name     string
		status   *corev1.ContainerStatus
		expected containerState
	}{
		{
			name:     "no etcd container status -> Unknown",
			status:   nil,
			expected: containerStateUnknown,
		},
		{
			name: "terminated -> Dead",
			status: &corev1.ContainerStatus{
				Name:  common.ContainerNameEtcd,
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}},
			},
			expected: containerStateDead,
		},
		{
			name:     "waiting CrashLoopBackOff -> Dead",
			status:   waitingStatus("CrashLoopBackOff"),
			expected: containerStateDead,
		},
		{
			name:     "waiting RunContainerError -> Dead",
			status:   waitingStatus("RunContainerError"),
			expected: containerStateDead,
		},
		{
			name:     "waiting ImagePullBackOff -> Dead",
			status:   waitingStatus("ImagePullBackOff"),
			expected: containerStateDead,
		},
		{
			name:     "waiting ErrImagePull -> Dead",
			status:   waitingStatus("ErrImagePull"),
			expected: containerStateDead,
		},
		{
			name:     "waiting CreateContainerConfigError -> Dead",
			status:   waitingStatus("CreateContainerConfigError"),
			expected: containerStateDead,
		},
		{
			name:     "waiting ContainerCreating -> Transient",
			status:   waitingStatus("ContainerCreating"),
			expected: containerStateTransient,
		},
		{
			name:     "waiting PodInitializing -> Transient",
			status:   waitingStatus("PodInitializing"),
			expected: containerStateTransient,
		},
		{
			name:     "waiting with unknown reason -> Transient (defensive)",
			status:   waitingStatus("SomeFutureReason"),
			expected: containerStateTransient,
		},
		{
			name: "running -> Alive",
			status: &corev1.ContainerStatus{
				Name:  common.ContainerNameEtcd,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			},
			expected: containerStateAlive,
		},
		{
			name: "no state populated (all-nil sub-states) -> Unknown",
			status: &corev1.ContainerStatus{
				Name:  common.ContainerNameEtcd,
				State: corev1.ContainerState{},
			},
			expected: containerStateUnknown,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pod := &corev1.Pod{}
			if tc.status != nil {
				pod.Status.ContainerStatuses = []corev1.ContainerStatus{*tc.status}
			}
			g.Expect(classifyEtcdContainer(pod)).To(Equal(tc.expected))
		})
	}
}

func TestClassifyEtcdContainer_IgnoresBackupRestoreSidecar(t *testing.T) {
	g := NewWithT(t)
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  common.ContainerNameEtcdBackupRestore,
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 137}},
				},
				{
					Name:  common.ContainerNameEtcd,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
		},
	}
	g.Expect(classifyEtcdContainer(pod)).To(Equal(containerStateAlive))
}

func TestIsParticipating(t *testing.T) {
	tests := []struct {
		name     string
		status   *corev1.ContainerStatus
		expected bool
	}{
		{
			name:     "no etcd container status -> not participating",
			status:   nil,
			expected: false,
		},
		{
			name: "etcd container Ready=true -> participating",
			status: &corev1.ContainerStatus{
				Name:  common.ContainerNameEtcd,
				Ready: true,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			},
			expected: true,
		},
		{
			name: "etcd container Ready=false -> not participating",
			status: &corev1.ContainerStatus{
				Name:  common.ContainerNameEtcd,
				Ready: false,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			},
			expected: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pod := &corev1.Pod{}
			if tc.status != nil {
				pod.Status.ContainerStatuses = []corev1.ContainerStatus{*tc.status}
			}
			g.Expect(isParticipating(pod)).To(Equal(tc.expected))
		})
	}
}

func TestFindEtcdContainerStatus(t *testing.T) {
	g := NewWithT(t)

	t.Run("returns the etcd container's status when both containers are present", func(t *testing.T) {
		pod := &corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
			{Name: common.ContainerNameEtcdBackupRestore, Ready: true},
			{Name: common.ContainerNameEtcd, Ready: true},
		}}}
		status := findEtcdContainerStatus(pod)
		g.Expect(status).ToNot(BeNil())
		g.Expect(status.Name).To(Equal(common.ContainerNameEtcd))
	})

	t.Run("returns nil when the etcd container is absent", func(t *testing.T) {
		pod := &corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
			{Name: common.ContainerNameEtcdBackupRestore, Ready: true},
		}}}
		g.Expect(findEtcdContainerStatus(pod)).To(BeNil())
	})

	t.Run("returns nil for an empty container-statuses slice", func(t *testing.T) {
		g.Expect(findEtcdContainerStatus(&corev1.Pod{})).To(BeNil())
	})
}

func waitingStatus(reason string) *corev1.ContainerStatus {
	return &corev1.ContainerStatus{
		Name:  common.ContainerNameEtcd,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}},
	}
}
