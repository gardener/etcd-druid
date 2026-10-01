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
// source member dropped from spec). For externally managed members it also
// covers an address removed from spec.externallyManagedMemberAddresses. It is a
// no-op when none applies.
func (r _resource) ensureSurplusMembersAreRemoved(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	shouldRemove, err := r.shouldRemoveSurplusMembers(ctx, etcd)
	if err != nil || !shouldRemove {
		return err
	}

	etcdClient, err := r.newEtcdClient(ctx, etcd)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := etcdClient.Close(); cerr != nil {
			ctx.Logger.Error(cerr, "failed to close etcd client")
		}
	}()

	liveMembers, err := getLiveMembersFromCluster(ctx, etcd, etcdClient)
	if err != nil {
		return err
	}

	// For a bootstrap decommission, wait until the target cluster stands on its
	// own before any source member leaves. For externally managed members, wait
	// so that a member that is still joining (added but not started, so it has
	// no name yet) is not taken for surplus.
	if druidv1alpha1.HasBootstrapMembersToDecommission(etcd) || !druidv1alpha1.ArePodsManagedByEtcdDruid(etcd) {
		if err := checkAllMembersHealthy(ctx, etcd, liveMembers); err != nil {
			return err
		}
	}

	retained, surplus := liveMembers.Split(druidv1alpha1.ExpectedMemberNames(etcd))
	candidate := etcdmember.SelectNextRemovalCandidate(surplus)
	if candidate == nil {
		return nil
	}

	if err := checkQuorumSafeMemberRemoval(ctx, etcd, *candidate, liveMembers); err != nil {
		return err
	}

	// Removing the leader forces a leader election and a short loss of
	// availability. Hand leadership to another member first and remove the
	// former leader on the next reconcile.
	if candidate.Role == etcdmember.MemberRoleLeader {
		moved, err := moveLeadershipAway(ctx, etcd, etcdClient, *candidate, retained)
		if err != nil || moved {
			return err
		}
	}

	ctx.Logger.Info("removing surplus etcd member",
		"member", candidate.Name, "memberID", fmt.Sprintf("%x", candidate.ID))
	if err := etcdClient.MemberRemove(ctx, candidate.ID); err != nil {
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

	// etcd-druid does not manage the pods of externally managed members, so it
	// cannot detect a scale-in from the StatefulSet. Once the cluster has formed,
	// any live member that is not in spec.externallyManagedMemberAddresses is
	// surplus.
	if !druidv1alpha1.ArePodsManagedByEtcdDruid(etcd) {
		return len(etcd.Status.Members) > 0, nil
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
	// When the cluster scales back out, the pods and members are recreated.
	if etcd.Spec.Replicas == 0 {
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

// checkAllMembersHealthy returns a requeue error unless every member of the
// Etcd (its own members, not the bootstrap source members) has joined the etcd
// cluster and is healthy. The error and the log name the members that have not
// joined, the members that are not healthy, and the surplus members whose
// removal is held back.
func checkAllMembersHealthy(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, liveMembers etcdmember.Members) error {
	names := druidv1alpha1.GetMemberNames(etcd)
	members, _ := liveMembers.Split(names)
	joined := members.Names()

	var notJoined, unhealthy []string
	for _, name := range names {
		if !joined.Has(name) {
			notJoined = append(notJoined, name)
		}
	}
	for _, m := range members {
		if !m.IsHealthy() {
			unhealthy = append(unhealthy, m.Name)
		}
	}
	if len(notJoined) == 0 && len(unhealthy) == 0 {
		return nil
	}

	// A member that has been added but not started has no name yet; it is
	// joining, so it is not reported as held back.
	var heldBack []string
	_, surplus := liveMembers.Split(druidv1alpha1.ExpectedMemberNames(etcd))
	for _, m := range surplus {
		if m.Name != "" {
			heldBack = append(heldBack, m.Name)
		}
	}

	ctx.Logger.Info("holding surplus member removal until all members of the Etcd have joined the etcd cluster and are healthy",
		"notJoined", notJoined, "unhealthy", unhealthy, "heldBack", heldBack)
	return druiderr.New(druiderr.ErrRequeueAfter, component.OperationPreSync,
		fmt.Sprintf("cannot remove surplus members %v for etcd %v: waiting for members to join the etcd cluster %v and to become healthy %v",
			heldBack, client.ObjectKeyFromObject(etcd), notJoined, unhealthy))
}

// newEtcdClient dials the etcd cluster and returns an etcd client. The caller
// owns the returned client and must close it.
func (r _resource) newEtcdClient(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (etcdclient.Client, error) {
	etcdClient, err := r.clientFactory.NewClient(ctx, r.client, etcd)
	if err != nil {
		return nil, druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to create etcd client for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	return etcdClient, nil
}

// getLiveMembersFromCluster returns the current live member list from the etcd cluster.
func getLiveMembersFromCluster(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, etcdClient etcdclient.Client) (etcdmember.Members, error) {
	members, err := etcdClient.MemberList(ctx)
	if err != nil {
		return nil, druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to list etcd members for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	return members, nil
}

// moveLeadershipAway transfers leadership from leader, which is about to be
// removed, to a healthy voting member in retained. Bootstrap members listed in
// spec.etcd.bootstrapWithExistingCluster are passed as the last preference, so
// leadership goes to any other retained member first. On a successful transfer
// it returns true together with a requeue error, so the former leader is
// removed on the next reconcile. When no member can take over, it returns false
// and the caller removes the leader directly.
func moveLeadershipAway(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, etcdClient etcdclient.Client, leader etcdmember.Member, retained etcdmember.Members) (bool, error) {
	transferee := etcdmember.SelectLeaderTransferee(retained, druidv1alpha1.GetBootstrapMemberNames(etcd))
	if transferee == nil {
		ctx.Logger.Info("no healthy member can take over leadership; removing the leader directly", "leader", leader.Name)
		return false, nil
	}

	ctx.Logger.Info("moving etcd leadership before removing the leader",
		"leader", leader.Name, "transferee", transferee.Name)
	if err := etcdClient.MoveLeader(ctx, leader.ClientURLs, transferee.ID); err != nil {
		return true, druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to move leadership from %s to %s for etcd: %v", leader.Name, transferee.Name, client.ObjectKeyFromObject(etcd)))
	}
	return true, druiderr.New(druiderr.ErrRequeueAfter, component.OperationPreSync,
		fmt.Sprintf("moved leadership from %s to %s; requeuing to remove the former leader for etcd %v",
			leader.Name, transferee.Name, client.ObjectKeyFromObject(etcd)))
}

// checkQuorumSafeMemberRemoval refuses to remove candidate when doing so would
// break quorum. The returned error requeues and records
// ERR_QUORUM_UNSAFE_MEMBER_REMOVAL in status.lastErrors (see
// preSyncEtcdResources); it clears once removal becomes safe.
func checkQuorumSafeMemberRemoval(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, candidate etcdmember.Member, members etcdmember.Members) error {
	if etcdmember.QuorumSafeToRemove(members, candidate.ID) {
		return nil
	}
	ctx.Logger.Info("holding member removal: removing the selected member would break quorum",
		"member", candidate.Name, "memberID", fmt.Sprintf("%x", candidate.ID))
	return druiderr.New(ErrQuorumUnsafeMemberRemoval, component.OperationPreSync,
		fmt.Sprintf("member %s not removed: removal would break quorum for etcd %v; waiting for the cluster to become healthy",
			candidate.Name, client.ObjectKeyFromObject(etcd)))
}
