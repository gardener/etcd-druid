// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition_test

import (
	"context"
	"maps"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/health/condition"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// The existing Ginkgo suite in check_all_members_updated_test.go covers the
// RollingUpdate branch. These Go-native tests focus on the OnDelete branch
// added in M7 and cover the pod-level revision comparison.

func TestAllMembersUpdatedCheck_OnDelete(t *testing.T) {
	const (
		ns       = "default"
		etcdName = "test"
	)
	etcd := druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{Name: etcdName, Namespace: ns},
	}

	tests := []struct {
		name            string
		buildObjects    func() []client.Object
		expectedStatus  druidv1alpha1.ConditionStatus
		expectedReason  string
		reasonSubstring string
	}{
		{
			name: "OnDelete: all pods at target revision -> True",
			buildObjects: func() []client.Object {
				sts := onDeleteControlledSTS(etcdName, ns, "rev-2", 3, 3)
				sel := sts.Spec.Selector.MatchLabels
				return []client.Object{
					sts,
					allMembersUpdatedPod("p-0", ns, "rev-2", sel),
					allMembersUpdatedPod("p-1", ns, "rev-2", sel),
					allMembersUpdatedPod("p-2", ns, "rev-2", sel),
				}
			},
			expectedStatus: druidv1alpha1.ConditionTrue,
			expectedReason: "AllMembersUpdated",
		},
		{
			name: "OnDelete: one pod at older revision -> False and message includes pod name",
			buildObjects: func() []client.Object {
				sts := onDeleteControlledSTS(etcdName, ns, "rev-2", 3, 3)
				sel := sts.Spec.Selector.MatchLabels
				return []client.Object{
					sts,
					allMembersUpdatedPod("p-0", ns, "rev-2", sel),
					allMembersUpdatedPod("p-behind", ns, "rev-1", sel),
					allMembersUpdatedPod("p-2", ns, "rev-2", sel),
				}
			},
			expectedStatus:  druidv1alpha1.ConditionFalse,
			expectedReason:  "NotAllMembersUpdated",
			reasonSubstring: "p-behind",
		},
		{
			name: "OnDelete: observedGeneration lags spec generation -> False",
			buildObjects: func() []client.Object {
				sts := onDeleteControlledSTS(etcdName, ns, "rev-2", 3, 3)
				sts.Generation = 2
				sts.Status.ObservedGeneration = 1
				return []client.Object{sts}
			},
			expectedStatus: druidv1alpha1.ConditionFalse,
			expectedReason: "NotAllMembersUpdated",
		},
		{
			name: "OnDelete: works even when currentRevision != updateRevision (K8s < 1.37 bug)",
			buildObjects: func() []client.Object {
				sts := onDeleteControlledSTS(etcdName, ns, "rev-2", 3, 3)
				sts.Status.CurrentRevision = "rev-1"
				sts.Status.UpdatedReplicas = 0 // upstream STS controller does not bump these under OnDelete
				sel := sts.Spec.Selector.MatchLabels
				return []client.Object{
					sts,
					allMembersUpdatedPod("p-0", ns, "rev-2", sel),
					allMembersUpdatedPod("p-1", ns, "rev-2", sel),
					allMembersUpdatedPod("p-2", ns, "rev-2", sel),
				}
			},
			expectedStatus: druidv1alpha1.ConditionTrue,
			expectedReason: "AllMembersUpdated",
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(tc.buildObjects()...).Build()
			result := condition.AllMembersUpdatedCheck(cl).Check(context.Background(), etcd)
			g.Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersUpdated))
			g.Expect(result.Status()).To(Equal(tc.expectedStatus))
			g.Expect(result.Reason()).To(Equal(tc.expectedReason))
			if tc.reasonSubstring != "" {
				g.Expect(result.Message()).To(ContainSubstring(tc.reasonSubstring))
			}
		})
	}
}

func onDeleteControlledSTS(name, namespace, updateRevision string, replicas, readyReplicas int32) *appsv1.StatefulSet {
	labels := map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: name}
	etcdMeta := metav1.ObjectMeta{Name: name, Namespace: namespace}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Generation:      1,
			OwnerReferences: []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcdMeta)},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:       ptr.To(replicas),
			Selector:       &metav1.LabelSelector{MatchLabels: labels},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      readyReplicas,
			UpdateRevision:     updateRevision,
		},
	}
}

func allMembersUpdatedPod(name, namespace, revision string, selector map[string]string) *corev1.Pod {
	labels := map[string]string{}
	maps.Copy(labels, selector)
	labels[appsv1.StatefulSetRevisionLabel] = revision
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels}}
}
