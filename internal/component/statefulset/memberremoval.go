// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdclient "github.com/gardener/etcd-druid/internal/client/etcd"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrRemoveEtcdMember indicates an error removing an etcd member during scale-in.
	ErrRemoveEtcdMember druidapicommon.ErrorCode = "ERR_REMOVE_ETCD_MEMBER"
	// ErrQuorumUnsafeMemberRemoval indicates that a surplus member was not removed
	// because doing so would break quorum. Returned from PreSync, it is mapped by
	// the reconcile flow to a requeue that records it in status.lastErrors while the
	// scale-in is held, and clears once removal becomes safe.
	ErrQuorumUnsafeMemberRemoval druidapicommon.ErrorCode = "ERR_QUORUM_UNSAFE_MEMBER_REMOVAL"
)

// ensureSurplusMembersAreRemoved removes at most one surplus etcd member per
// reconcile, requeuing until no surplus remains, so the cluster never drops
// below quorum. A member is surplus when the live cluster contains it but the
// desired spec does not, which happens in two cases: a scale-in (spec.replicas
// lowered) and a bootstrap members removal (a joined bootstrapWithExistingCluster
// source member dropped from spec). It is a no-op when neither applies.
func (r _resource) ensureSurplusMembersAreRemoved(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	applicable, err := r.shouldRemoveSurplusMembers(ctx, etcd)
	if err != nil || !applicable {
		return err
	}

	memberClient, err := r.newMemberClient(ctx, etcd)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := memberClient.Close(); cerr != nil {
			ctx.Logger.Error(cerr, "failed to close etcd member client")
		}
	}()

	members, err := getLiveMembersFromCluster(ctx, etcd, memberClient)
	if err != nil {
		return err
	}

	// Bootstrap decommission: hold until all druid-managed members are live and
	// healthy, so the target cluster stands on its own before any source member leaves.
	if isBootstrapMembersRemoval(etcd) && !etcdmember.AllMembersHealthy(members, managedMemberNames(etcd)) {
		ctx.Logger.Info("holding bootstrap member decommission: not all managed members are live and healthy yet")
		return druiderr.New(druiderr.ErrRequeueAfter, component.OperationPreSync,
			fmt.Sprintf("cannot decommission bootstrap members: waiting for all managed members to become live and healthy for etcd %v",
				client.ObjectKeyFromObject(etcd)))
	}

	surplus := etcdmember.SurplusMemberNames(members, druidv1alpha1.ExpectedMemberNames(etcd))
	candidate := etcdmember.SelectNextRemovalCandidate(members, surplus)
	if candidate == nil {
		return nil
	}

	if err := checkQuorumSafeMemberRemoval(ctx, etcd, *candidate, members); err != nil {
		return err
	}

	ctx.Logger.Info("removing surplus etcd member",
		"member", candidate.Name, "memberID", fmt.Sprintf("%x", candidate.ID))
	if err := memberClient.MemberRemove(ctx, candidate.ID); err != nil {
		return druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to remove etcd member %s for etcd: %v", candidate.Name, client.ObjectKeyFromObject(etcd)))
	}

	// Requeue so removal proceeds one member per reconcile: a scale-in waits for
	// the removed member's pod to terminate, and any further surplus is trimmed on
	// subsequent reconciles until none remains.
	return druiderr.New(druiderr.ErrRequeueAfter, component.OperationPreSync,
		fmt.Sprintf("removed etcd member %s; requeuing to trim remaining surplus members for etcd %v",
			candidate.Name, client.ObjectKeyFromObject(etcd)))
}

// shouldRemoveSurplusMembers reports whether surplus member removal should run in
// this reconcile at all, and short-circuits the cases where dialing the etcd
// cluster is pointless or wrong.
func (r _resource) shouldRemoveSurplusMembers(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (bool, error) {
	// A nil factory means member removal is not wired (e.g. in unit tests that do
	// not exercise scale-in); treat it as a no-op.
	if r.clientFactory == nil {
		return false, nil
	}

	// Surplus members only exist during a scale-in or bootstrap members removal.
	// Skip the MemberList dial otherwise: during a config-only change (e.g. a
	// peer/client TLS transition) it would time out and abort the very Sync that
	// rolls the change out.
	if !druidv1alpha1.IsScaleOperationInProgressWithReason(etcd,
		druidv1alpha1.ScaleOperationReasonScalingIn,
		druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval) {
		return false, nil
	}

	// Zero replicas is not a scale-in: do not treat live members as surplus.
	// When the cluster scales back up, the pods and members are recreated.
	if druidv1alpha1.HasZeroReplicas(etcd) {
		return false, nil
	}

	// If no StatefulSet exists yet (initial cluster creation), or if no pods are
	// ready (cluster is still bootstrapping or fully down), there are no etcd
	// members to remove. Skip the MemberList dial entirely to avoid a connection
	// timeout against a client Service that has no endpoints yet.
	existingSts, err := r.getExistingStatefulSet(ctx, etcd.ObjectMeta)
	if err != nil {
		return false, druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to get existing StatefulSet while checking for surplus members for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	return existingSts != nil && existingSts.Status.ReadyReplicas > 0, nil
}

// newMemberClient dials the etcd cluster and returns a member client. The caller
// owns the returned client and must close it.
func (r _resource) newMemberClient(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (etcdclient.Client, error) {
	memberClient, err := r.clientFactory.NewClient(ctx, r.client, etcd)
	if err != nil {
		return nil, druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to create etcd member client for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	return memberClient, nil
}

// getLiveMembersFromCluster returns the current live member list from the etcd cluster.
func getLiveMembersFromCluster(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, memberClient etcdclient.Client) ([]etcdmember.Member, error) {
	members, err := memberClient.MemberList(ctx)
	if err != nil {
		return nil, druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to list etcd members for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	return members, nil
}

// checkQuorumSafeMemberRemoval refuses to remove candidate when doing so would
// break quorum. The returned error requeues and records
// ERR_QUORUM_UNSAFE_MEMBER_REMOVAL in status.lastErrors (see
// preSyncEtcdResources); it clears once removal becomes safe.
func checkQuorumSafeMemberRemoval(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, candidate etcdmember.Member, members []etcdmember.Member) error {
	if etcdmember.QuorumSafeToRemove(members, candidate.ID) {
		return nil
	}
	ctx.Logger.Info("holding member removal: removing the selected member would break quorum",
		"member", candidate.Name, "memberID", fmt.Sprintf("%x", candidate.ID))
	return druiderr.New(ErrQuorumUnsafeMemberRemoval, component.OperationPreSync,
		fmt.Sprintf("member %s not removed: removal would break quorum for etcd %v; waiting for the cluster to become healthy",
			candidate.Name, client.ObjectKeyFromObject(etcd)))
}

// isBootstrapMembersRemoval reports whether this reconcile is decommissioning
// source members joined via bootstrapWithExistingCluster.
func isBootstrapMembersRemoval(etcd *druidv1alpha1.Etcd) bool {
	return len(druidv1alpha1.GetBootstrapMemberNamesToDecommission(etcd)) > 0
}

// managedMemberNames returns the set of member names backing pod ordinals in
// [0, spec.replicas): the members the desired state wants to keep.
func managedMemberNames(etcd *druidv1alpha1.Etcd) map[string]bool {
	names := make(map[string]bool, etcd.Spec.Replicas)
	for ordinal := int32(0); ordinal < etcd.Spec.Replicas; ordinal++ {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, int(ordinal))
		names[druidv1alpha1.GetMemberName(etcd.Spec.MemberNamePrefix, podName)] = true
	}
	return names
}
