// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
)

func TestIsPodOutdated(t *testing.T) {
	tests := []struct {
		name        string
		podLabels   map[string]string
		updateRev   string
		expectedOut bool
	}{
		{
			name:        "pod is at target revision -> not outdated",
			podLabels:   map[string]string{appsv1.StatefulSetRevisionLabel: "rev-2"},
			updateRev:   "rev-2",
			expectedOut: false,
		},
		{
			name:        "pod is at older revision -> outdated",
			podLabels:   map[string]string{appsv1.StatefulSetRevisionLabel: "rev-1"},
			updateRev:   "rev-2",
			expectedOut: true,
		},
		{
			name:        "pod has no controller-revision-hash label -> outdated",
			podLabels:   nil,
			updateRev:   "rev-2",
			expectedOut: true,
		},
		{
			name:        "empty target revision -> not outdated (STS controller has not populated it yet)",
			podLabels:   map[string]string{appsv1.StatefulSetRevisionLabel: "rev-1"},
			updateRev:   "",
			expectedOut: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: tc.podLabels}}
			sts := &appsv1.StatefulSet{Status: appsv1.StatefulSetStatus{UpdateRevision: tc.updateRev}}
			g.Expect(isPodOutdated(pod, sts)).To(Equal(tc.expectedOut))
		})
	}
}
