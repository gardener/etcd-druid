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
		moveLeaderErr  error
		wantRemovedIDs []uint64
		// wantMovedTo lists the transferee IDs passed to MoveLeader.
		wantMovedTo []uint64
		wantRequeue bool
		wantErrCode druidapicommon.ErrorCode
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
			name:         "surplus leader: leadership moves to a healthy retained member, no removal yet",
			specReplicas: 2,
			scaleReason:  druidv1alpha1.ScaleOperationReasonScalingIn,
			liveMembers: []etcdmember.Member{
				healthyVoter(0x1, "etcd-main-0"),
				healthyVoter(0x2, "etcd-main-1"),
				healthyLeader(0x3, "etcd-main-2"),
			},
			wantMovedTo:    []uint64{0x1},
			wantRequeue:    true,
			wantCloseCalls: 1,
		},
		{
			name:         "surplus leader with no managed voter to take over: falls through to the quorum check",
			specReplicas: 1,
			scaleReason:  druidv1alpha1.ScaleOperationReasonScalingIn,
			// etcd-main-0 is the only retained member but it is a learner, so it
			// cannot take leadership. Quorum stays safe because learners do not vote.
			liveMembers: []etcdmember.Member{
				{ID: 0x1, Name: "etcd-main-0", Role: etcdmember.MemberRoleLearner, Health: etcdmember.MemberHealthHealthy},
				healthyLeader(0x3, "etcd-main-2"),
			},
			wantErrCode:    ErrQuorumUnsafeMemberRemoval,
			wantCloseCalls: 1,
		},
		{
			name:         "MoveLeader error surfaces as ErrRemoveEtcdMember",
			specReplicas: 2,
			scaleReason:  druidv1alpha1.ScaleOperationReasonScalingIn,
			liveMembers: []etcdmember.Member{
				healthyVoter(0x1, "etcd-main-0"),
				healthyVoter(0x2, "etcd-main-1"),
				healthyLeader(0x3, "etcd-main-2"),
			},
			moveLeaderErr:  fmt.Errorf("not leader"),
			wantMovedTo:    []uint64{0x1},
			wantErrCode:    ErrRemoveEtcdMember,
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
				Members:       tc.liveMembers,
				ListErr:       tc.listErr,
				RemoveErr:     tc.removeErr,
				MoveLeaderErr: tc.moveLeaderErr,
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
			g.Expect(fakeClient.MoveLeaderCalls).To(Equal(tc.wantMovedTo))
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

	t.Run("no member of the Etcd live -> requeue, no removal", func(t *testing.T) {
		g := NewWithT(t)
		etcd := newEtcd()
		sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 1)
		sts.Spec.Replicas = ptr.To(int32(1))
		sts.Status.ReadyReplicas = 1
		cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
			WithObjects(etcd.DeepCopy(), sts).WithStatusSubresource(&druidv1alpha1.Etcd{}).Build()

		// Only the source member is live; the Etcd's own member (etcd-main-0) has not joined.
		fakeClient := &etcdfake.Client{Members: []etcdmember.Member{healthyLeader(0x9, "etcd-source-0")}}
		r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: fakeClient}}
		opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

		err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)
		g.Expect(err).To(HaveOccurred())
		derr := druiderr.AsDruidError(err)
		g.Expect(derr.Code).To(Equal(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)))
		g.Expect(fakeClient.RemoveCalls).To(BeEmpty(), "source member must not be removed before all members of the Etcd are live and healthy")
	})

	t.Run("all members of the Etcd live and healthy -> source member removed", func(t *testing.T) {
		g := NewWithT(t)
		etcd := newEtcd()
		sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 1)
		sts.Spec.Replicas = ptr.To(int32(1))
		sts.Status.ReadyReplicas = 1
		cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
			WithObjects(etcd.DeepCopy(), sts).WithStatusSubresource(&druidv1alpha1.Etcd{}).Build()

		// The Etcd's own member etcd-main-0 is live alongside the source member.
		fakeClient := &etcdfake.Client{Members: []etcdmember.Member{
			healthyLeader(0x1, "etcd-main-0"),
			healthyVoter(0x9, "etcd-source-0"),
		}}
		r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: fakeClient}}
		opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

		err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)
		g.Expect(err).To(HaveOccurred()) // requeue after removal
		g.Expect(fakeClient.RemoveCalls).To(Equal([]uint64{0x9}), "source member removed once the Etcd's own members are live")
	})

	t.Run("source member is leader -> leadership moves to the Etcd's own member, bootstrap members last", func(t *testing.T) {
		g := NewWithT(t)
		etcd := newEtcd()
		// etcd-source-1 stays in spec and has a lower ID than etcd-main-0, but
		// members of the current Etcd are preferred.
		etcd.Spec.Etcd.BootstrapWithExistingCluster.Members = []druidv1alpha1.BootstrapExistingMember{{Name: "etcd-source-1"}}
		etcd.Status.BootstrapWithExistingCluster.Members = []druidv1alpha1.BootstrapJoinedMember{{Name: "etcd-source-0"}, {Name: "etcd-source-1"}}
		sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 1)
		sts.Spec.Replicas = ptr.To(int32(1))
		sts.Status.ReadyReplicas = 1
		cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
			WithObjects(etcd.DeepCopy(), sts).WithStatusSubresource(&druidv1alpha1.Etcd{}).Build()

		fakeClient := &etcdfake.Client{Members: []etcdmember.Member{
			healthyVoter(0x8, "etcd-source-1"),
			healthyLeader(0x9, "etcd-source-0"),
			healthyVoter(0xa, "etcd-main-0"),
		}}
		r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: fakeClient}}
		opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

		err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)
		g.Expect(druiderr.AsDruidError(err).Code).To(Equal(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)))
		g.Expect(fakeClient.MoveLeaderCalls).To(Equal([]uint64{0xa}), "leadership must go to the Etcd's own member etcd-main-0")
		g.Expect(fakeClient.RemoveCalls).To(BeEmpty(), "the former leader is removed on the next reconcile")
	})

	t.Run("unhealthy bootstrap member -> removed once quorum-safe", func(t *testing.T) {
		g := NewWithT(t)
		etcd := newEtcd()
		sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 1)
		sts.Spec.Replicas = ptr.To(int32(1))
		sts.Status.ReadyReplicas = 1
		cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
			WithObjects(etcd.DeepCopy(), sts).WithStatusSubresource(&druidv1alpha1.Etcd{}).Build()

		// The source member is unhealthy, but the remaining voter is healthy, so
		// removing it is quorum-safe.
		fakeClient := &etcdfake.Client{Members: []etcdmember.Member{
			healthyLeader(0x1, "etcd-main-0"),
			{ID: 0x9, Name: "etcd-source-0", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthUnhealthy},
		}}
		r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: fakeClient}}
		opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

		err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)
		g.Expect(err).To(HaveOccurred()) // requeue after removal
		derr := druiderr.AsDruidError(err)
		g.Expect(derr).NotTo(BeNil())
		g.Expect(derr.Code).To(Equal(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)))
		g.Expect(fakeClient.RemoveCalls).To(Equal([]uint64{0x9}), "the unhealthy source member is removed since doing so is quorum-safe")
	})

	t.Run("healthy STS members but several unhealthy bootstrap voters -> quorum-unsafe, held back", func(t *testing.T) {
		g := NewWithT(t)
		etcd := newEtcd()
		etcd.Spec.Etcd.BootstrapWithExistingCluster.Members = []druidv1alpha1.BootstrapExistingMember{
			{Name: "etcd-source-1"}, {Name: "etcd-source-2"}, {Name: "etcd-source-3"},
		}
		etcd.Status.BootstrapWithExistingCluster.Members = []druidv1alpha1.BootstrapJoinedMember{
			{Name: "etcd-source-0"}, {Name: "etcd-source-1"}, {Name: "etcd-source-2"}, {Name: "etcd-source-3"},
		}
		etcd.Spec.Replicas = 3
		sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 3)
		sts.Spec.Replicas = ptr.To(int32(3))
		sts.Status.ReadyReplicas = 3
		cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
			WithObjects(etcd.DeepCopy(), sts).WithStatusSubresource(&druidv1alpha1.Etcd{}).Build()

		// The 3 STS members are healthy, so the bootstrap hold passes. All 4 source
		// members are unhealthy. Removing the surplus etcd-source-0 would leave 6
		// voters of which 3 are healthy, below the quorum of 4.
		fakeClient := &etcdfake.Client{Members: []etcdmember.Member{
			healthyLeader(0x1, "etcd-main-0"),
			healthyVoter(0x2, "etcd-main-1"),
			healthyVoter(0x3, "etcd-main-2"),
			{ID: 0x6, Name: "etcd-source-3", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthUnhealthy},
			{ID: 0x7, Name: "etcd-source-2", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthUnhealthy},
			{ID: 0x8, Name: "etcd-source-0", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthUnhealthy},
			{ID: 0x9, Name: "etcd-source-1", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthUnhealthy},
		}}
		r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: fakeClient}}
		opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

		err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)
		g.Expect(err).To(HaveOccurred())
		derr := druiderr.AsDruidError(err)
		g.Expect(derr).NotTo(BeNil())
		g.Expect(derr.Code).To(Equal(ErrQuorumUnsafeMemberRemoval))
		g.Expect(fakeClient.RemoveCalls).To(BeEmpty(), "removal is held back since too few healthy voters would remain")
	})
}

// TestEnsureMemberRemovalExternallyManagedMembers verifies surplus member
// removal for externally managed members: a live member whose address is no
// longer in spec.externallyManagedMemberAddresses is removed without any scale
// condition or StatefulSet replicas, but only once the cluster has formed.
func TestEnsureMemberRemovalExternallyManagedMembers(t *testing.T) {
	const (
		ip0 = "10.0.0.1"
		ip1 = "10.0.0.2"
		ip2 = "10.0.0.3"
	)
	member := func(ip string) string { return fmt.Sprintf("%s-%s", removalEtcdName, ip) }

	tests := []struct {
		name string
		// noStatusMembers leaves status.members empty (cluster not formed yet).
		noStatusMembers bool
		liveMembers     []etcdmember.Member
		wantRequeue     bool
		wantRemoved     []uint64
	}{
		{
			name:            "cluster not formed yet -> no-op",
			noStatusMembers: true,
			liveMembers:     []etcdmember.Member{healthyLeader(0x1, member(ip0)), healthyVoter(0x2, member(ip1)), healthyVoter(0x3, member(ip2))},
		},
		{
			name:        "address removed from spec -> surplus member removed",
			liveMembers: []etcdmember.Member{healthyLeader(0x1, member(ip0)), healthyVoter(0x2, member(ip1)), healthyVoter(0x3, member(ip2))},
			wantRequeue: true,
			wantRemoved: []uint64{0x3},
		},
		{
			name: "removed member already stopped -> still removed",
			liveMembers: []etcdmember.Member{
				healthyLeader(0x1, member(ip0)),
				healthyVoter(0x2, member(ip1)),
				{ID: 0x3, Name: member(ip2), Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthUnknown},
			},
			wantRequeue: true,
			wantRemoved: []uint64{0x3},
		},
		{
			name:        "every live member in spec -> no-op",
			liveMembers: []etcdmember.Member{healthyLeader(0x1, member(ip0)), healthyVoter(0x2, member(ip1))},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			// spec keeps ip0 and ip1; ip2 has been removed.
			etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
				WithReplicas(2).
				WithExternallyManagedMembers([]string{ip0, ip1}).
				Build()
			etcd.UID = removalEtcdUID
			if !tc.noStatusMembers {
				etcd.Status.Members = []druidv1alpha1.EtcdMemberStatus{{Name: member(ip0)}, {Name: member(ip1)}, {Name: member(ip2)}}
			}
			cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).WithObjects(etcd.DeepCopy()).Build()
			fakeClient := &etcdfake.Client{Members: tc.liveMembers}
			r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: fakeClient}}
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

			err := r.ensureSurplusMembersAreRemoved(opCtx, etcd)
			if tc.wantRequeue {
				g.Expect(druiderr.AsDruidError(err).Code).To(Equal(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(fakeClient.RemoveCalls).To(Equal(tc.wantRemoved))
		})
	}
}

// TestShouldRemoveSurplusMembersExternallyManagedMembers verifies that for
// externally managed members, surplus member removal runs once the cluster has
// formed, without a scale-in condition or a StatefulSet.
func TestShouldRemoveSurplusMembersExternallyManagedMembers(t *testing.T) {
	tests := []struct {
		name          string
		statusMembers []druidv1alpha1.EtcdMemberStatus
		want          bool
	}{
		{
			name: "cluster not formed yet -> false",
			want: false,
		},
		{
			name:          "cluster formed -> true",
			statusMembers: []druidv1alpha1.EtcdMemberStatus{{Name: "etcd-main-10.0.0.1"}, {Name: "etcd-main-10.0.0.2"}},
			want:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
				WithReplicas(2).
				WithExternallyManagedMembers([]string{"10.0.0.1", "10.0.0.2"}).
				Build()
			etcd.Status.Members = tc.statusMembers
			// No StatefulSet and no scale condition: neither is needed for
			// externally managed members.
			cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).WithObjects(etcd.DeepCopy()).Build()
			r := _resource{client: cl, clientFactory: &etcdfake.Factory{Client: &etcdfake.Client{}}}
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

			got, err := r.shouldRemoveSurplusMembers(opCtx, etcd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tc.want))
		})
	}
}

// TestShouldRemoveSurplusMembersStatefulSetGate verifies that, during a
// scale-in, surplus member removal runs whenever the StatefulSet has replicas,
// even if none of its pods are ready yet, and is skipped without a StatefulSet
// or when it is scaled to zero.
func TestShouldRemoveSurplusMembersStatefulSetGate(t *testing.T) {
	tests := []struct {
		name          string
		noSts         bool
		stsReplicas   int32
		readyReplicas int32
		want          bool
	}{
		{name: "no StatefulSet -> false", noSts: true, want: false},
		{name: "StatefulSet scaled to zero -> false", stsReplicas: 0, want: false},
		{name: "StatefulSet with replicas but no ready pods -> true", stsReplicas: 3, readyReplicas: 0, want: true},
		{name: "StatefulSet with ready pods -> true", stsReplicas: 3, readyReplicas: 3, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
				WithReplicas(1).
				Build()
			etcd.UID = removalEtcdUID
			etcd.Status.Conditions = []druidv1alpha1.Condition{{
				Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
				Status: druidv1alpha1.ConditionFalse,
				Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
			}}

			clBuilder := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).WithObjects(etcd.DeepCopy())
			if !tc.noSts {
				sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, tc.stsReplicas)
				sts.Spec.Replicas = ptr.To(tc.stsReplicas)
				sts.Status.ReadyReplicas = tc.readyReplicas
				clBuilder = clBuilder.WithObjects(sts)
			}
			r := _resource{client: clBuilder.Build(), clientFactory: &etcdfake.Factory{Client: &etcdfake.Client{}}}
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

			got, err := r.shouldRemoveSurplusMembers(opCtx, etcd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tc.want))
		})
	}
}

// TestSplitExpectedMembersBootstrapRemoval verifies that a joined bootstrap
// member no longer present in spec is surplus even without a replica change,
// while the Etcd's own members and the bootstrap members still in spec are
// retained.
func TestSplitExpectedMembersBootstrapRemoval(t *testing.T) {
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

	// Live members: the Etcd's 3 own members plus both bootstrap source members.
	// etcd-source-0 is still in spec (should stay), etcd-source-1 is not (surplus).
	liveMembers := etcdmember.Members{
		healthyLeader(0x1, "etcd-main-0"),
		healthyVoter(0x2, "etcd-main-1"),
		healthyVoter(0x3, "etcd-main-2"),
		healthyVoter(0x9, "etcd-source-0"), // still in spec → expected
		healthyVoter(0xa, "etcd-source-1"), // not in spec → surplus
	}

	retained, surplus := liveMembers.Split(druidv1alpha1.ExpectedMemberNames(etcd))
	g.Expect(surplus).To(HaveLen(1))
	g.Expect(surplus[0].Name).To(Equal("etcd-source-1"))
	g.Expect(retained).To(HaveLen(4))
}
