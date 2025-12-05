// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	. "github.com/gardener/etcd-druid/api/core/v1alpha1"
	. "github.com/onsi/gomega"
)

// TestGetTimeToExpiryAndHasTTLExpired tests the GetTimeToExpiry and HasTTLExpired methods of the EtcdOpsTask struct.
func TestGetTimeToExpiryAndHasTTLExpired(t *testing.T) {
	g := NewWithT(t)
	now := time.Now().UTC()

	tests := []struct {
		name    string
		task    *EtcdOpsTask
		expired bool
	}{
		{
			name: "should not expire when task is in progress and within TTL window",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Second))},
				Spec:       EtcdOpsTaskSpec{TTLSecondsAfterFinished: ptr.To(int32(60))},
				Status: EtcdOpsTaskStatus{
					State:              ptr.To(TaskStateInProgress),
					LastTransitionTime: &metav1.Time{Time: now.Add(-30 * time.Second)},
				},
			},
			expired: false,
		},
		{
			name: "should not expire when task is finished but still within TTL window",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now)},
				Spec:       EtcdOpsTaskSpec{TTLSecondsAfterFinished: ptr.To(int32(60))},
				Status: EtcdOpsTaskStatus{
					State:              ptr.To(TaskStateSucceeded),
					LastTransitionTime: &metav1.Time{Time: now.Add(-30 * time.Second)},
				},
			},
			expired: false,
		},
		{
			name: "should expire when task is finished and TTL window has passed",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-120 * time.Second))},
				Spec:       EtcdOpsTaskSpec{TTLSecondsAfterFinished: ptr.To(int32(60))},
				Status: EtcdOpsTaskStatus{
					State:              ptr.To(TaskStateFailed),
					LastTransitionTime: &metav1.Time{Time: now.Add(-90 * time.Second)},
				},
			},
			expired: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			remaining := tc.task.GetTimeToExpiry()
			if tc.expired {
				g.Expect(remaining).To(Equal(time.Duration(0)))
				g.Expect(tc.task.HasTTLExpired()).To(BeTrue())
			} else {
				g.Expect(remaining).To(BeNumerically(">", 0))
				g.Expect(tc.task.HasTTLExpired()).To(BeFalse())
			}
		})
	}
}

// TestIsCompleted tests the IsCompleted method of the EtcdOpsTask struct.
func TestIsCompleted(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name      string
		task      *EtcdOpsTask
		completed bool
	}{
		{
			name: "should return false when state is nil",
			task: &EtcdOpsTask{
				Status: EtcdOpsTaskStatus{
					State: nil,
				},
			},
			completed: false,
		},
		{
			name: "should return false when state is Pending",
			task: &EtcdOpsTask{
				Status: EtcdOpsTaskStatus{
					State: ptr.To(TaskStatePending),
				},
			},
			completed: false,
		},
		{
			name: "should return false when state is InProgress",
			task: &EtcdOpsTask{
				Status: EtcdOpsTaskStatus{
					State: ptr.To(TaskStateInProgress),
				},
			},
			completed: false,
		},
		{
			name: "should return true when state is Succeeded",
			task: &EtcdOpsTask{
				Status: EtcdOpsTaskStatus{
					State: ptr.To(TaskStateSucceeded),
				},
			},
			completed: true,
		},
		{
			name: "should return true when state is Failed",
			task: &EtcdOpsTask{
				Status: EtcdOpsTaskStatus{
					State: ptr.To(TaskStateFailed),
				},
			},
			completed: true,
		},
		{
			name: "should return true when state is Rejected",
			task: &EtcdOpsTask{
				Status: EtcdOpsTaskStatus{
					State: ptr.To(TaskStateRejected),
				},
			},
			completed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g.Expect(tc.task.IsCompleted()).To(Equal(tc.completed))
		})
	}
}

// TestGetEtcdReference tests the GetEtcdReference method of the EtcdOpsTask struct.
func TestGetEtcdReference(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name        string
		task        *EtcdOpsTask
		expectedRef string
		expectedNS  string
		expectEmpty bool
	}{
		{
			name: "should return valid reference when etcdName is set",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "test-namespace",
				},
				Spec: EtcdOpsTaskSpec{
					EtcdName: ptr.To("my-etcd"),
				},
			},
			expectedRef: "my-etcd",
			expectedNS:  "test-namespace",
			expectEmpty: false,
		},
		{
			name: "should return empty reference when etcdName is nil",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "test-namespace",
				},
				Spec: EtcdOpsTaskSpec{
					EtcdName: nil,
				},
			},
			expectEmpty: true,
		},
		{
			name: "should return empty reference when etcdName is empty string",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "test-namespace",
				},
				Spec: EtcdOpsTaskSpec{
					EtcdName: ptr.To(""),
				},
			},
			expectEmpty: true,
		},
		{
			name: "should use task namespace for etcd reference namespace",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "different-namespace",
				},
				Spec: EtcdOpsTaskSpec{
					EtcdName: ptr.To("etcd-cluster"),
				},
			},
			expectedRef: "etcd-cluster",
			expectedNS:  "different-namespace",
			expectEmpty: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ref := tc.task.GetEtcdReference()

			if tc.expectEmpty {
				g.Expect(ref.Name).To(BeEmpty())
				g.Expect(ref.Namespace).To(BeEmpty())
			} else {
				g.Expect(ref.Name).To(Equal(tc.expectedRef))
				g.Expect(ref.Namespace).To(Equal(tc.expectedNS))
			}
		})
	}
}
