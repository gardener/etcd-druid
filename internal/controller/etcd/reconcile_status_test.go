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
//   - write the full source-member inventory once the condition flips True,
//     stamping every entry with the same JoinedAt,
//   - back-fill PeerURLs on entries written by an earlier druid release that
//     omitted them (the field was added later as +optional),
//   - never overwrite an existing JoinedAt or otherwise re-write the snapshot.
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

	makeEtcd := func(spec *druidv1alpha1.BootstrapWithExistingCluster, conditions []druidv1alpha1.Condition, status []druidv1alpha1.BootstrapJoinedMember) *druidv1alpha1.Etcd {
		etcd := &druidv1alpha1.Etcd{}
		etcd.Spec.Etcd.BootstrapWithExistingCluster = spec
		etcd.Status.Conditions = conditions
		etcd.Status.BootstrapWithExistingClusterMembers = status
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
				g.Expect(etcd.Status.BootstrapWithExistingClusterMembers).To(BeEmpty())
			},
		},
		{
			name: "spec.members is empty — no-op",
			etcd: makeEtcd(&druidv1alpha1.BootstrapWithExistingCluster{}, []druidv1alpha1.Condition{conditionTrue}, nil),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				g.Expect(etcd.Status.BootstrapWithExistingClusterMembers).To(BeEmpty())
			},
		},
		{
			name: "condition False (BootstrapInProgress) — snapshot is NOT written",
			etcd: makeEtcd(&druidv1alpha1.BootstrapWithExistingCluster{Members: srcMembers}, []druidv1alpha1.Condition{conditionFalse}, nil),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				g.Expect(etcd.Status.BootstrapWithExistingClusterMembers).To(BeEmpty(),
					"snapshot must not be written until the BootstrappedWithExistingCluster condition is True — guards against persisting partial state mid-join")
			},
		},
		{
			name: "condition True, status empty — snapshot is written once with shared JoinedAt",
			etcd: makeEtcd(&druidv1alpha1.BootstrapWithExistingCluster{Members: srcMembers}, []druidv1alpha1.Condition{conditionTrue}, nil),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				snap := etcd.Status.BootstrapWithExistingClusterMembers
				g.Expect(snap).To(HaveLen(2))
				g.Expect(snap[0].Name).To(Equal("etcd-source-0"))
				g.Expect(snap[0].PeerURLs).To(ConsistOf("https://etcd-source-0:2380"))
				g.Expect(snap[1].Name).To(Equal("etcd-source-1"))
				g.Expect(snap[1].PeerURLs).To(ConsistOf("https://etcd-source-1:2380"))
				// JoinedAt must be identical across all entries (bootstrap is atomic).
				g.Expect(snap[0].JoinedAt).To(Equal(snap[1].JoinedAt))
				g.Expect(snap[0].JoinedAt.IsZero()).To(BeFalse())
			},
		},
		{
			name: "snapshot already populated — JoinedAt is preserved on idempotent re-run",
			etcd: func() *druidv1alpha1.Etcd {
				preExisting := metav1.NewTime(metav1.Now().Add(-time.Hour))
				return makeEtcd(
					&druidv1alpha1.BootstrapWithExistingCluster{Members: srcMembers},
					[]druidv1alpha1.Condition{conditionTrue},
					[]druidv1alpha1.BootstrapJoinedMember{
						{Name: "etcd-source-0", PeerURLs: []string{"https://etcd-source-0:2380"}, JoinedAt: preExisting},
						{Name: "etcd-source-1", PeerURLs: []string{"https://etcd-source-1:2380"}, JoinedAt: preExisting},
					},
				)
			}(),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				snap := etcd.Status.BootstrapWithExistingClusterMembers
				g.Expect(snap).To(HaveLen(2))
				// The existing JoinedAt must NOT be overwritten on later reconciles.
				g.Expect(snap[0].JoinedAt).To(Equal(snap[1].JoinedAt))
			},
		},
		{
			name: "snapshot has empty PeerURLs (older druid release) — they are back-filled from spec",
			etcd: makeEtcd(
				&druidv1alpha1.BootstrapWithExistingCluster{Members: srcMembers},
				[]druidv1alpha1.Condition{conditionTrue},
				[]druidv1alpha1.BootstrapJoinedMember{
					{Name: "etcd-source-0", JoinedAt: metav1.Now()},
					{Name: "etcd-source-1", JoinedAt: metav1.Now()},
				},
			),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				snap := etcd.Status.BootstrapWithExistingClusterMembers
				g.Expect(snap).To(HaveLen(2))
				g.Expect(snap[0].PeerURLs).To(ConsistOf("https://etcd-source-0:2380"))
				g.Expect(snap[1].PeerURLs).To(ConsistOf("https://etcd-source-1:2380"))
			},
		},
		{
			name: "snapshot has empty PeerURLs but spec no longer references that name — entry is left untouched",
			etcd: makeEtcd(
				&druidv1alpha1.BootstrapWithExistingCluster{Members: srcMembers},
				[]druidv1alpha1.Condition{conditionTrue},
				[]druidv1alpha1.BootstrapJoinedMember{
					{Name: "etcd-source-orphan", JoinedAt: metav1.Now()},
				},
			),
			validate: func(g *WithT, etcd *druidv1alpha1.Etcd) {
				snap := etcd.Status.BootstrapWithExistingClusterMembers
				g.Expect(snap).To(HaveLen(1))
				g.Expect(snap[0].Name).To(Equal("etcd-source-orphan"))
				g.Expect(snap[0].PeerURLs).To(BeEmpty())
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
