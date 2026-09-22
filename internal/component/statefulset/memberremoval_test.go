// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"context"
	"fmt"
	"testing"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdclient "github.com/gardener/etcd-druid/internal/client/etcd"
	etcdfake "github.com/gardener/etcd-druid/internal/client/etcd/fake"
	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/gomega"
)

const (
	removalEtcdName  = "etcd-main"
	removalNamespace = "test-ns"
	removalEtcdUID   = types.UID("etcd-main-uid")
)

// healthyVoter builds a healthy voting member.
func healthyVoter(id uint64, name string) etcdmember.Member {
	return etcdmember.Member{ID: id, Name: name, Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy}
}

// healthyLeader builds a healthy leader member.
func healthyLeader(id uint64, name string) etcdmember.Member {
	return etcdmember.Member{ID: id, Name: name, Role: etcdmember.MemberRoleLeader, Health: etcdmember.MemberHealthHealthy}
}

// fiveHealthyMembers returns a 5-member all-healthy cluster with etcd-main-0 as leader.
func fiveHealthyMembers() []etcdmember.Member {
	return []etcdmember.Member{
		healthyLeader(0x1, "etcd-main-0"),
		healthyVoter(0x2, "etcd-main-1"),
		healthyVoter(0x3, "etcd-main-2"),
		healthyVoter(0x4, "etcd-main-3"),
		healthyVoter(0x5, "etcd-main-4"),
	}
}

// TestEnsureMemberRemoval exercises the pre-sync member removal: it must
// remove exactly one surplus member per reconcile (learners first, leader last),
// requeue after a successful removal, hold back when the removal would break
// quorum, and be a no-op when every live member is in the expected set.
// Removal only runs while a scale operation is recorded as in progress
// (ScaleOperationComplete=False with reason ScalingIn or BootstrapMembersRemoval);
// otherwise the cluster is never dialed. Within a scale-in, surplus is derived
// from (live members) minus (expected per spec): the StatefulSet replica count is
// not consulted; the live etcd cluster is the source of truth.
func TestEnsureMemberRemoval(t *testing.T) {
	tests := []struct {
		name string
		// specReplicas is the desired replica count.
		specReplicas int32
		// nilFactory wires a nil Factory (removal not enabled).
		nilFactory bool
		// scaleReason, when set, records ScaleOperationComplete=False with this
		// reason so the surplus-removal gate proceeds. Empty means no scale
		// operation is in progress and the gate short-circuits before dialing.
		scaleReason    string
		liveMembers    []etcdmember.Member
		listErr        error
		removeErr      error
		wantRemovedIDs []uint64
		wantRequeue    bool
		wantErrCode    druidapicommon.ErrorCode
		// wantCloseCalls is the expected number of times Client.Close
		// should be called; 0 when the gate short-circuits before creating a client.
		wantCloseCalls int
	}{
		{
			name:           "no scale operation in progress -> no-op, cluster not dialed",
			specReplicas:   5,
			liveMembers:    fiveHealthyMembers(),
			wantCloseCalls: 0,
		},
		{
			name:           "scale-in recorded but live members match spec exactly -> no-op",
			specReplicas:   5,
			scaleReason:    druidv1alpha1.ScaleOperationReasonScalingIn,
			liveMembers:    fiveHealthyMembers(),
			wantCloseCalls: 1,
		},
		{
			name:           "nil factory -> no-op even with surplus live members",
			specReplicas:   3,
			nilFactory:     true,
			scaleReason:    druidv1alpha1.ScaleOperationReasonScalingIn,
			liveMembers:    fiveHealthyMembers(),
			wantCloseCalls: 0,
		},
		{
			name:         "fresh cluster: live members match spec (no surplus) -> no-op",
			specReplicas: 3,
			scaleReason:  druidv1alpha1.ScaleOperationReasonScalingIn,
			liveMembers: []etcdmember.Member{
				healthyLeader(0x1, "etcd-main-0"),
				healthyVoter(0x2, "etcd-main-1"),
				healthyVoter(0x3, "etcd-main-2"),
			},
			wantCloseCalls: 1,
		},
		{
			name:           "scale-in 5->3: removes the lowest-ID surplus voter first and requeues",
			specReplicas:   3,
			scaleReason:    druidv1alpha1.ScaleOperationReasonScalingIn,
			liveMembers:    fiveHealthyMembers(),
			wantRemovedIDs: []uint64{0x4}, // surplus voters {0x4,0x5}; lowest ID removed first
			wantRequeue:    true,
			wantCloseCalls: 1,
		},
		{
			name:         "scale-in but surplus member already gone from etcd -> no-op",
			specReplicas: 4,
			scaleReason:  druidv1alpha1.ScaleOperationReasonScalingIn,
			liveMembers: []etcdmember.Member{
				healthyLeader(0x1, "etcd-main-0"),
				healthyVoter(0x2, "etcd-main-1"),
				healthyVoter(0x3, "etcd-main-2"),
				healthyVoter(0x4, "etcd-main-3"),
			},
			wantCloseCalls: 1,
		},
		{
			name:         "quorum-unsafe removal is held back",
			specReplicas: 1,
			scaleReason:  druidv1alpha1.ScaleOperationReasonScalingIn,
			// Removing etcd-main-1 would leave etcd-main-0 as the only surviving
			// voter but it is Unknown -> surviving voter not healthy -> blocked.
			liveMembers: []etcdmember.Member{
				{ID: 0x1, Name: "etcd-main-0", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthUnknown},
				healthyVoter(0x2, "etcd-main-1"),
			},
			wantErrCode:    ErrQuorumUnsafeMemberRemoval,
			wantCloseCalls: 1,
		},
		{
			name:           "MemberList error surfaces as ErrRemoveEtcdMember",
			specReplicas:   3,
			scaleReason:    druidv1alpha1.ScaleOperationReasonScalingIn,
			liveMembers:    fiveHealthyMembers(),
			listErr:        fmt.Errorf("boom"),
			wantErrCode:    ErrRemoveEtcdMember,
			wantCloseCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
				WithReplicas(tc.specReplicas).
				Build()
			etcd.UID = removalEtcdUID
			if tc.scaleReason != "" {
				etcd.Status.Conditions = []druidv1alpha1.Condition{{
					Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
					Status: druidv1alpha1.ConditionFalse,
					Reason: tc.scaleReason,
				}}
			}

			clBuilder := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
				WithObjects(etcd.DeepCopy()).
				WithStatusSubresource(&druidv1alpha1.Etcd{})
			// When the factory is wired (real-cluster scenarios), add an existing STS
			// with replicas equal to the number of live members so the STS-existence
			// guard in ensureSurplusMembersAreRemoved does not short-circuit.
			if !tc.nilFactory && len(tc.liveMembers) > 0 {
				sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, int32(len(tc.liveMembers)))
				sts.Status.ReadyReplicas = int32(len(tc.liveMembers))
				clBuilder = clBuilder.WithObjects(sts)
			}
			cl := clBuilder.Build()

			fakeClient := &etcdfake.Client{
				Members:   tc.liveMembers,
				ListErr:   tc.listErr,
				RemoveErr: tc.removeErr,
			}
			var factory etcdclient.Factory
			if !tc.nilFactory {
				factory = &etcdfake.Factory{Client: fakeClient}
			}

			r := _resource{client: cl, clientFactory: factory}
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

			err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)

			if tc.wantErrCode != "" {
				g.Expect(err).To(HaveOccurred())
				derr := druiderr.AsDruidError(err)
				g.Expect(derr).NotTo(BeNil())
				g.Expect(derr.Code).To(Equal(tc.wantErrCode))
			} else if tc.wantRequeue {
				g.Expect(err).To(HaveOccurred())
				derr := druiderr.AsDruidError(err)
				g.Expect(derr).NotTo(BeNil())
				g.Expect(derr.Code).To(Equal(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(fakeClient.RemoveCalls).To(Equal(tc.wantRemovedIDs))
			g.Expect(fakeClient.CloseCalls).To(Equal(tc.wantCloseCalls), "Client.Close must be called exactly once per factory creation")
		})
	}
}

// TestEnsureMemberRemovalReturnsQuorumUnsafeErrorWhenBlocked verifies that a
// quorum-blocked reconcile returns a DruidError carrying
// ERR_QUORUM_UNSAFE_MEMBER_REMOVAL (which the reconcile flow records in
// status.lastErrors) without patching status itself, and that a subsequent
// unblocked reconcile removes the member and returns a plain requeue.
func TestEnsureMemberRemovalReturnsQuorumUnsafeErrorWhenBlocked(t *testing.T) {
	g := NewWithT(t)

	// 3->2 scale-in: surplus is etcd-main-2. First reconcile: etcd-main-1 is
	// Unknown, so removing etcd-main-2 leaves a non-healthy survivor -> blocked.
	blockedMembers := []etcdmember.Member{
		healthyLeader(0x1, "etcd-main-0"),
		{ID: 0x2, Name: "etcd-main-1", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthUnknown},
		healthyVoter(0x3, "etcd-main-2"),
	}

	etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
		WithReplicas(2).
		Build()
	etcd.UID = removalEtcdUID
	etcd.Status.LastOperation = &druidapicommon.LastOperation{State: druidapicommon.LastOperationState("Processing")}
	etcd.Status.Conditions = []druidv1alpha1.Condition{{
		Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
		Status: druidv1alpha1.ConditionFalse,
		Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
	}}

	sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 3)
	sts.Spec.Replicas = ptr.To(int32(3))
	sts.Status.ReadyReplicas = 3

	// Count status patches so we can assert the branch does not record out-of-band.
	statusPatchCount := 0
	cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
		WithObjects(etcd.DeepCopy(), sts).
		WithStatusSubresource(&druidv1alpha1.Etcd{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				statusPatchCount++
				return c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	fakeClient := &etcdfake.Client{Members: blockedMembers}
	r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: fakeClient}}
	opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

	// Blocked reconcile: returns the quorum-unsafe DruidError, no removal, no
	// out-of-band status patch.
	err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)
	g.Expect(err).To(HaveOccurred())
	derr := druiderr.AsDruidError(err)
	g.Expect(derr).NotTo(BeNil())
	g.Expect(derr.Code).To(Equal(ErrQuorumUnsafeMemberRemoval))
	g.Expect(fakeClient.RemoveCalls).To(BeEmpty())
	g.Expect(statusPatchCount).To(Equal(0), "the quorum-unsafe branch must not patch status directly; the reconcile flow records the error")

	// Cluster recovers: etcd-main-1 becomes healthy. Unblocked reconcile removes
	// etcd-main-2 and returns a plain requeue (no quorum-unsafe error).
	fakeClient.Members = []etcdmember.Member{
		healthyLeader(0x1, "etcd-main-0"),
		healthyVoter(0x2, "etcd-main-1"),
		healthyVoter(0x3, "etcd-main-2"),
	}

	err = r.ensureSurplusMembersAreRemoved(opCtx, etcd)
	g.Expect(err).To(HaveOccurred()) // requeue after successful removal
	derr = druiderr.AsDruidError(err)
	g.Expect(derr).NotTo(BeNil())
	g.Expect(derr.Code).To(Equal(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)))
	g.Expect(fakeClient.RemoveCalls).To(Equal([]uint64{0x3}))
}

// TestEnsureMemberRemovalBootstrapManagedMemberGate verifies that a
// bootstrap/source surplus member is not removed until every druid-managed
// member (an ordinal in [0, spec.replicas)) is live and healthy.
func TestEnsureMemberRemovalBootstrapManagedMemberGate(t *testing.T) {
	newEtcd := func() *druidv1alpha1.Etcd {
		etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
			WithReplicas(1).
			Build()
		etcd.UID = removalEtcdUID
		etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
			Members: []druidv1alpha1.BootstrapExistingMember{},
		}
		etcd.Status.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingClusterStatus{
			Members: []druidv1alpha1.BootstrapJoinedMember{{Name: "etcd-source-0"}},
		}
		etcd.Status.Conditions = []druidv1alpha1.Condition{{
			Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
			Status: druidv1alpha1.ConditionFalse,
			Reason: druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
		}}
		return etcd
	}

	t.Run("no managed member live -> requeue, no removal", func(t *testing.T) {
		g := NewWithT(t)
		etcd := newEtcd()
		sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 1)
		sts.Spec.Replicas = ptr.To(int32(1))
		sts.Status.ReadyReplicas = 1
		cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
			WithObjects(etcd.DeepCopy(), sts).WithStatusSubresource(&druidv1alpha1.Etcd{}).Build()

		// Only the source member is live; the managed member (etcd-main-0) has not joined.
		fakeClient := &etcdfake.Client{Members: []etcdmember.Member{healthyLeader(0x9, "etcd-source-0")}}
		r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: fakeClient}}
		opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

		err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)
		g.Expect(err).To(HaveOccurred())
		derr := druiderr.AsDruidError(err)
		g.Expect(derr.Code).To(Equal(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)))
		g.Expect(fakeClient.RemoveCalls).To(BeEmpty(), "source member must not be removed before all managed members are live and healthy")
	})

	t.Run("all managed members live and healthy -> source member removed", func(t *testing.T) {
		g := NewWithT(t)
		etcd := newEtcd()
		sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 1)
		sts.Spec.Replicas = ptr.To(int32(1))
		sts.Status.ReadyReplicas = 1
		cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
			WithObjects(etcd.DeepCopy(), sts).WithStatusSubresource(&druidv1alpha1.Etcd{}).Build()

		// Managed member etcd-main-0 is live alongside the source member.
		fakeClient := &etcdfake.Client{Members: []etcdmember.Member{
			healthyLeader(0x1, "etcd-main-0"),
			healthyVoter(0x9, "etcd-source-0"),
		}}
		r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: fakeClient}}
		opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

		err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)
		g.Expect(err).To(HaveOccurred()) // requeue after removal
		g.Expect(fakeClient.RemoveCalls).To(Equal([]uint64{0x9}), "source member removed once a managed member is live")
	})
}

// TestSurplusMemberNamesBootstrapRemoval verifies that joined bootstrap members
// no longer present in spec are selected for removal even without a replica
// change, and that the live-diff computation correctly identifies surplus
// vs expected members.
func TestSurplusMemberNamesBootstrapRemoval(t *testing.T) {
	g := NewWithT(t)

	etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
		WithReplicas(3).
		Build()
	etcd.UID = removalEtcdUID
	etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
		Members: []druidv1alpha1.BootstrapExistingMember{{Name: "etcd-source-0"}},
	}
	etcd.Status.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingClusterStatus{
		Members: []druidv1alpha1.BootstrapJoinedMember{
			{Name: "etcd-source-0"},
			{Name: "etcd-source-1"},
		},
	}

	// Live members: the 3 managed members plus both bootstrap source members.
	// etcd-source-0 is still in spec (should stay), etcd-source-1 is not (surplus).
	liveMembers := []etcdmember.Member{
		healthyLeader(0x1, "etcd-main-0"),
		healthyVoter(0x2, "etcd-main-1"),
		healthyVoter(0x3, "etcd-main-2"),
		healthyVoter(0x9, "etcd-source-0"), // still in spec → expected
		healthyVoter(0xa, "etcd-source-1"), // not in spec → surplus
	}

	surplus := etcdmember.SurplusMemberNames(liveMembers, druidv1alpha1.ExpectedMemberNames(etcd))
	g.Expect(surplus).To(HaveKey("etcd-source-1"))
	g.Expect(surplus).NotTo(HaveKey("etcd-source-0"))
	g.Expect(surplus).NotTo(HaveKey("etcd-main-0"))
	g.Expect(surplus).NotTo(HaveKey("etcd-main-1"))
	g.Expect(surplus).NotTo(HaveKey("etcd-main-2"))
}
