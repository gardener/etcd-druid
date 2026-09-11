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
	removalEtcdName   = "etcd-main"
	removalNamespace  = "test-ns"
	removalEtcdUID    = types.UID("etcd-main-uid")
	removalStsReplica = int32(5)
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
// quorum, and be a no-op when there is no scale-in in progress. All role/health
// data is taken from the live member list, never from CR status (rule 3).
func TestEnsureMemberRemoval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// specReplicas is the desired replica count (StatefulSet observed is 5).
		specReplicas int32
		// noStatefulSet omits the StatefulSet entirely (fresh cluster).
		noStatefulSet  bool
		liveMembers    []etcdmember.Member
		listErr        error
		removeErr      error
		wantRemovedIDs []uint64
		wantRequeue    bool
		wantErrCode    druidapicommon.ErrorCode
		// wantCloseCalls is the expected number of times MemberClient.Close
		// should be called; 0 for early-exit paths that never reach NewMemberClient.
		wantCloseCalls int
	}{
		{
			name:           "no scale-in in progress -> no-op",
			specReplicas:   5,
			liveMembers:    fiveHealthyMembers(),
			wantCloseCalls: 0,
		},
		{
			name:           "no StatefulSet yet -> no-op",
			specReplicas:   3,
			noStatefulSet:  true,
			liveMembers:    fiveHealthyMembers(),
			wantCloseCalls: 0,
		},
		{
			name:           "scale-in 5->3 removes lowest-ID surplus member first and requeues",
			specReplicas:   3,
			liveMembers:    fiveHealthyMembers(),
			wantRemovedIDs: []uint64{0x4}, // etcd-main-3 (lowest member ID among surplus)
			wantRequeue:    true,
			wantCloseCalls: 1,
		},
		{
			name:         "scale-in but surplus member already gone from etcd -> no-op",
			specReplicas: 4,
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
			name:           "ListMembers error surfaces as ErrRemoveEtcdMember",
			specReplicas:   3,
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

			builder := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
				WithObjects(etcd.DeepCopy()).
				WithStatusSubresource(&druidv1alpha1.Etcd{})
			if !tc.noStatefulSet {
				sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, removalStsReplica)
				sts.Spec.Replicas = ptr.To(removalStsReplica)
				builder = builder.WithObjects(sts)
			}
			cl := builder.Build()

			fakeClient := &etcdfake.MemberClient{
				Members:   tc.liveMembers,
				ListErr:   tc.listErr,
				RemoveErr: tc.removeErr,
			}
			factory := &etcdfake.MemberClientFactory{Client: fakeClient}

			r := _resource{client: cl, memberClientFactory: factory}
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

			err := r.removeOneSurplusMember(opCtx, etcd)

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
			g.Expect(fakeClient.CloseCalls).To(Equal(tc.wantCloseCalls), "MemberClient.Close must be called exactly once per factory creation")
		})
	}
}

// TestEnsureMemberRemovalReturnsQuorumUnsafeErrorWhenBlocked verifies that a
// quorum-blocked reconcile returns a DruidError carrying
// ERR_QUORUM_UNSAFE_MEMBER_REMOVAL (which the reconcile flow records in
// status.lastErrors) without patching status itself, and that a subsequent
// unblocked reconcile removes the member and returns a plain requeue.
func TestEnsureMemberRemovalReturnsQuorumUnsafeErrorWhenBlocked(t *testing.T) {
	t.Parallel()
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

	sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 3)
	sts.Spec.Replicas = ptr.To(int32(3))

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

	fakeClient := &etcdfake.MemberClient{Members: blockedMembers}
	r := _resource{client: cl, memberClientFactory: &etcdfake.MemberClientFactory{Client: fakeClient}}
	opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

	// Blocked reconcile: returns the quorum-unsafe DruidError, no removal, no
	// out-of-band status patch.
	err := r.removeOneSurplusMember(opCtx, etcd)
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

	err = r.removeOneSurplusMember(opCtx, etcd)
	g.Expect(err).To(HaveOccurred()) // requeue after successful removal
	derr = druiderr.AsDruidError(err)
	g.Expect(derr).NotTo(BeNil())
	g.Expect(derr.Code).To(Equal(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)))
	g.Expect(fakeClient.RemoveCalls).To(Equal([]uint64{0x3}))
}

// TestEnsureMemberRemovalBootstrapSurplusRemoved verifies that a bootstrap
// source member is removed once a managed member is live alongside it.
func TestEnsureMemberRemovalBootstrapSurplusRemoved(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

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

	sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 1)
	sts.Spec.Replicas = ptr.To(int32(1))
	cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).
		WithObjects(etcd.DeepCopy(), sts).WithStatusSubresource(&druidv1alpha1.Etcd{}).Build()

	// Managed member etcd-main-0 is live alongside the source member.
	fakeClient := &etcdfake.MemberClient{Members: []etcdmember.Member{
		healthyLeader(0x1, "etcd-main-0"),
		healthyVoter(0x9, "etcd-source-0"),
	}}
	r := _resource{client: cl, memberClientFactory: &etcdfake.MemberClientFactory{Client: fakeClient}}
	opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

	err := r.removeOneSurplusMember(opCtx, etcd)
	g.Expect(err).To(HaveOccurred()) // requeue after removal
	g.Expect(fakeClient.RemoveCalls).To(Equal([]uint64{0x9}), "source member removed once a managed member is live")
}

// TestSurplusMemberNamesBootstrapRemoval verifies that joined bootstrap members
// no longer present in spec are selected for removal even without a replica
// change.
func TestSurplusMemberNamesBootstrapRemoval(t *testing.T) {
	t.Parallel()
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

	// StatefulSet at the desired size, so no replica-driven surplus.
	sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 3)
	sts.Spec.Replicas = ptr.To(int32(3))
	cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).WithObjects(sts).Build()

	r := _resource{client: cl}
	opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

	names, err := r.surplusMemberNames(opCtx, etcd)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(names).To(HaveKey("etcd-source-1"))
	g.Expect(names).NotTo(HaveKey("etcd-source-0"))
}
