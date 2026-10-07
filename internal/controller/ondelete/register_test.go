// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/onsi/gomega"
)

func TestOnDeleteStrategyPredicate(t *testing.T) {
	tests := []struct {
		name         string
		oldStrategy  appsv1.StatefulSetUpdateStrategyType
		newStrategy  appsv1.StatefulSetUpdateStrategyType
		object       client.Object
		expectCreate bool
		expectUpdate bool
		expectDelete bool
	}{
		{
			name:         "Create on OnDelete STS is accepted",
			newStrategy:  appsv1.OnDeleteStatefulSetStrategyType,
			expectCreate: true,
			expectUpdate: true,
		},
		{
			name:         "Create on RollingUpdate STS is filtered out",
			newStrategy:  appsv1.RollingUpdateStatefulSetStrategyType,
			expectCreate: false,
			expectUpdate: false,
		},
		{
			name:         "Update transitioning INTO OnDelete is accepted",
			oldStrategy:  appsv1.RollingUpdateStatefulSetStrategyType,
			newStrategy:  appsv1.OnDeleteStatefulSetStrategyType,
			expectCreate: true,
			expectUpdate: true,
		},
		{
			name:         "Update transitioning OUT of OnDelete is accepted (to let the controller notice the disengagement)",
			oldStrategy:  appsv1.OnDeleteStatefulSetStrategyType,
			newStrategy:  appsv1.RollingUpdateStatefulSetStrategyType,
			expectCreate: false, // Create uses newStrategy only
			expectUpdate: true,
		},
		{
			name:         "Update on non-StatefulSet object is filtered out",
			object:       &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "not-an-sts"}},
			expectCreate: false,
			expectUpdate: false,
			expectDelete: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	p := onDeleteStrategy()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var oldObj, newObj client.Object
			if tc.object != nil {
				oldObj = tc.object
				newObj = tc.object
			} else {
				oldObj = stsWithStrategy("old", tc.oldStrategy, "")
				newObj = stsWithStrategy("new", tc.newStrategy, "")
			}
			g.Expect(p.Create(event.CreateEvent{Object: newObj})).To(Equal(tc.expectCreate))
			g.Expect(p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})).To(Equal(tc.expectUpdate))
			g.Expect(p.Delete(event.DeleteEvent{Object: newObj})).To(Equal(tc.expectDelete))
			g.Expect(p.Generic(event.GenericEvent{Object: newObj})).To(BeFalse())
		})
	}
}

func TestUpdateRevisionChangedPredicate(t *testing.T) {
	tests := []struct {
		name         string
		oldRev       string
		newRev       string
		oldStrategy  appsv1.StatefulSetUpdateStrategyType
		newStrategy  appsv1.StatefulSetUpdateStrategyType
		expectUpdate bool
	}{
		{
			name:         "no revision change and no strategy change -> filtered out",
			oldRev:       "rev-1",
			newRev:       "rev-1",
			oldStrategy:  appsv1.OnDeleteStatefulSetStrategyType,
			newStrategy:  appsv1.OnDeleteStatefulSetStrategyType,
			expectUpdate: false,
		},
		{
			name:         "revision change accepted",
			oldRev:       "rev-1",
			newRev:       "rev-2",
			oldStrategy:  appsv1.OnDeleteStatefulSetStrategyType,
			newStrategy:  appsv1.OnDeleteStatefulSetStrategyType,
			expectUpdate: true,
		},
		{
			name:         "strategy transition accepted even without revision change",
			oldRev:       "rev-1",
			newRev:       "rev-1",
			oldStrategy:  appsv1.RollingUpdateStatefulSetStrategyType,
			newStrategy:  appsv1.OnDeleteStatefulSetStrategyType,
			expectUpdate: true,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	p := updateRevisionChanged()

	// Every Create is accepted so the controller can pick up mid-rollouts on restart.
	t.Run("Create is always accepted", func(t *testing.T) {
		g.Expect(p.Create(event.CreateEvent{Object: stsWithStrategy("x", appsv1.OnDeleteStatefulSetStrategyType, "rev-1")})).To(BeTrue())
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			oldSts := stsWithStrategy("x", tc.oldStrategy, tc.oldRev)
			newSts := stsWithStrategy("x", tc.newStrategy, tc.newRev)
			g.Expect(p.Update(event.UpdateEvent{ObjectOld: oldSts, ObjectNew: newSts})).To(Equal(tc.expectUpdate))
		})
	}

	t.Run("Update with non-StatefulSet objects is filtered out", func(t *testing.T) {
		notAnSts := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "not-an-sts"}}
		g.Expect(p.Update(event.UpdateEvent{ObjectOld: notAnSts, ObjectNew: notAnSts})).To(BeFalse())
	})
}

// stsWithStrategy builds a minimal StatefulSet for predicate testing.
func stsWithStrategy(name string, strategy appsv1.StatefulSetUpdateStrategyType, revision string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testStsNamespace},
		Spec:       appsv1.StatefulSetSpec{UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: strategy}},
		Status:     appsv1.StatefulSetStatus{UpdateRevision: revision},
	}
}
