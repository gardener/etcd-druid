// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/gomega"
)

func TestMatchPodConditions(t *testing.T) {
	testCases := []struct {
		name       string
		conditions []corev1.PodCondition
		condType   corev1.PodConditionType
		condStatus corev1.ConditionStatus
		expected   bool
	}{
		{
			name: "condition type and status are present",
			conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			condType:   corev1.PodReady,
			condStatus: corev1.ConditionTrue,
			expected:   true,
		},
		{
			name: "condition type and status are not present",
			conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				},
			},
			condType:   corev1.PodReady,
			condStatus: corev1.ConditionTrue,
			expected:   false,
		},
		{
			name: "condition type is present but status is not",
			conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionUnknown,
				},
			},
			condType:   corev1.PodReady,
			condStatus: corev1.ConditionTrue,
			expected:   false,
		},
		{
			name: "condition type is not present but status is",
			conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodInitialized,
					Status: corev1.ConditionTrue,
				},
			},
			condType:   corev1.PodReady,
			condStatus: corev1.ConditionTrue,
			expected:   false,
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g.Expect(MatchPodConditions(tc.conditions, tc.condType, tc.condStatus)).To(Equal(tc.expected))
		})
	}
}

func TestHasPodReadyConditionTrue(t *testing.T) {
	testCases := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "pod has Ready condition with status True",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "pod has Ready condition with status not True",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "pod has no Ready condition",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodInitialized,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g.Expect(HasPodReadyConditionTrue(tc.pod)).To(Equal(tc.expected))
		})
	}
}
