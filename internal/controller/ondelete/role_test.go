// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

func TestIsLeader(t *testing.T) {
	tests := []struct {
		name     string
		role     *druidv1alpha1.EtcdRole
		expected bool
	}{
		{name: "nil role -> not leader", role: nil, expected: false},
		{name: "Leader -> leader", role: ptr.To(druidv1alpha1.EtcdRoleLeader), expected: true},
		{name: "Member -> not leader", role: ptr.To(druidv1alpha1.EtcdRoleMember), expected: false},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g.Expect(isLeader(tc.role)).To(Equal(tc.expected))
		})
	}
}

func TestMemberRole(t *testing.T) {
	const (
		ns       = "test-ns"
		etcdName = "etcd-test"
	)
	etcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{Name: etcdName, Namespace: ns},
		Spec:       druidv1alpha1.EtcdSpec{Replicas: 3},
	}
	// Pod name and lease name are equal when Spec.MemberNamePrefix is nil.
	podName := etcdName + "-0"

	tests := []struct {
		name           string
		holderIdentity *string
		expectedRole   *druidv1alpha1.EtcdRole
		expectedErr    bool
		omitLease      bool
	}{
		{
			name:           "leader role",
			holderIdentity: ptr.To("some-member-id:Leader"),
			expectedRole:   ptr.To(druidv1alpha1.EtcdRoleLeader),
		},
		{
			name:           "follower role via 2-part format",
			holderIdentity: ptr.To("some-member-id:Member"),
			expectedRole:   ptr.To(druidv1alpha1.EtcdRoleMember),
		},
		{
			name:           "follower role via 3-part format",
			holderIdentity: ptr.To("some-member-id:some-cluster-id:Member"),
			expectedRole:   ptr.To(druidv1alpha1.EtcdRoleMember),
		},
		{
			name:           "unknown role string yields nil role, not error",
			holderIdentity: ptr.To("some-member-id:BogusRole"),
			expectedRole:   nil,
		},
		{
			name:           "nil holderIdentity yields error",
			holderIdentity: nil,
			expectedErr:    true,
		},
		{
			name:           "malformed holderIdentity yields error",
			holderIdentity: ptr.To("garbage-without-colons"),
			expectedErr:    true,
		},
		{
			name:         "lease not found -> (nil role, no error)",
			omitLease:    true,
			expectedRole: nil,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			if !tc.omitLease {
				existingObjects = append(existingObjects, &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: ns},
					Spec:       coordinationv1.LeaseSpec{HolderIdentity: tc.holderIdentity},
				})
			}
			cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(existingObjects...).Build()
			r := &Reconciler{client: cl, logger: logr.Discard()}
			pod := makeStsPod(podName, ns, "rev-1", false)
			role, err := r.memberRole(context.Background(), etcd, pod)
			if tc.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(role).To(Equal(tc.expectedRole))
			}
		})
	}
}
