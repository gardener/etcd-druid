// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
)

// TestMutateBootstrapWithExistingClusterStatus exercises the status snapshot
// writer for the bootstrap-with-existing-cluster feature. The function must:
//   - no-op when spec is unset or empty,
//   - no-op while the BootstrappedWithExistingCluster condition is not True
//     (i.e., during the join), so that the snapshot is only written once the
//     join has succeeded — preventing premature/partial state from being
//     persisted,
//   - write the source-member snapshot once the condition flips True, with a
//     single JoinedAt at the wrapper level and the members copied from spec,
//   - never overwrite the snapshot once it has been recorded.
func TestMutateBootstrapWithExistingClusterStatus(t *testing.T) {
	conditionTrue := druidv1alpha1.Condition{
		Type:   druidv1alpha1.ConditionTypeBootstrappedWithExistingCluster,
		Status: druidv1alpha1.ConditionTrue,
	}
	conditionFalse := druidv1alpha1.Condition{
		Type:   druidv1alpha1.ConditionTypeBootstrappedWithExistingCluster,
		Status: druidv1alpha1.ConditionFalse,
	}
	srcMembers := []druidv1alpha1.BootstrapExistingMember{
		{Name: "etcd-source-0", PeerURLs: []string{"https://etcd-source-0:2380"}},
		{Name: "etcd-source-1", PeerURLs: []string{"https://etcd-source-1:2380"}},
	}

	makeEtcd := func(spec *druidv1alpha1.BootstrapWithExistingCluster, conditions []druidv1alpha1.Condition, status *druidv1alpha1.BootstrapWithExistingClusterStatus) *druidv1alpha1.Etcd {
		etcd := &druidv1alpha1.Etcd{}
		etcd.Spec.Etcd.BootstrapWithExistingCluster = spec
		etcd.Status.Conditions = conditions
		etcd.Status.BootstrapWithExistingCluster = status
		return etcd
	}

	tests := []struct {
		name string
		etcd *druidv1alpha1.Etcd
		// validate inspects the etcd after the mutate function has run.
		validate func(g *WithT, etcd *druidv1alpha1.Etcd)
	}{
		{
			name: "spec is nil — no-op",
			etcd: makeEtcd(nil, []druidv1alpha1.Condition{conditionTrue}, nil),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				g.Expect(etcd.Status.BootstrapWithExistingCluster).To(BeNil())
			},
		},
		{
			name: "spec.members is empty — no-op",
			etcd: makeEtcd(&druidv1alpha1.BootstrapWithExistingCluster{}, []druidv1alpha1.Condition{conditionTrue}, nil),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				g.Expect(etcd.Status.BootstrapWithExistingCluster).To(BeNil())
			},
		},
		{
			name: "condition False (BootstrapInProgress) — snapshot is NOT written",
			etcd: makeEtcd(&druidv1alpha1.BootstrapWithExistingCluster{Members: srcMembers}, []druidv1alpha1.Condition{conditionFalse}, nil),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				g.Expect(etcd.Status.BootstrapWithExistingCluster).To(BeNil(),
					"snapshot must not be written until the BootstrappedWithExistingCluster condition is True — guards against persisting partial state mid-join")
			},
		},
		{
			name: "condition True, status not populated — snapshot is written once with a non-zero JoinedAt",
			etcd: makeEtcd(&druidv1alpha1.BootstrapWithExistingCluster{Members: srcMembers}, []druidv1alpha1.Condition{conditionTrue}, nil),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				snap := etcd.Status.BootstrapWithExistingCluster
				g.Expect(snap).NotTo(BeNil())
				g.Expect(snap.JoinedAt.IsZero()).To(BeFalse(), "JoinedAt must be set when the snapshot is recorded")
				g.Expect(snap.Members).To(HaveLen(2))
				g.Expect(snap.Members[0].Name).To(Equal("etcd-source-0"))
				g.Expect(snap.Members[0].PeerURLs).To(ConsistOf("https://etcd-source-0:2380"))
				g.Expect(snap.Members[1].Name).To(Equal("etcd-source-1"))
				g.Expect(snap.Members[1].PeerURLs).To(ConsistOf("https://etcd-source-1:2380"))
			},
		},
		{
			name: "snapshot already populated — JoinedAt is preserved on idempotent re-run",
			etcd: func() *druidv1alpha1.Etcd {
				preExisting := metav1.NewTime(metav1.Now().Add(-time.Hour))
				return makeEtcd(
					&druidv1alpha1.BootstrapWithExistingCluster{Members: srcMembers},
					[]druidv1alpha1.Condition{conditionTrue},
					&druidv1alpha1.BootstrapWithExistingClusterStatus{
						JoinedAt: preExisting,
						Members: []druidv1alpha1.BootstrapJoinedMember{
							{Name: "etcd-source-0", PeerURLs: []string{"https://etcd-source-0:2380"}},
							{Name: "etcd-source-1", PeerURLs: []string{"https://etcd-source-1:2380"}},
						},
					},
				)
			}(),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				snap := etcd.Status.BootstrapWithExistingCluster
				g.Expect(snap).NotTo(BeNil())
				// The existing JoinedAt must NOT be overwritten on later reconciles.
				g.Expect(time.Since(snap.JoinedAt.Time)).To(BeNumerically(">=", time.Hour-time.Minute),
					"JoinedAt must be preserved from the initial write, not refreshed")
				g.Expect(snap.Members).To(HaveLen(2))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r := &Reconciler{}
			res := r.mutateBootstrapWithExistingClusterStatus(component.OperatorContext{}, tt.etcd, logr.Discard())
			g.Expect(res.HasErrors()).To(BeFalse())
			tt.validate(g, tt.etcd)
		})
	}
}
