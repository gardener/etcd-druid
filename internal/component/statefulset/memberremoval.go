// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// memberremoval.go — quorum-safe surplus member removal for the StatefulSet component.
//
// PreSync path: remove at most one surplus etcd member per reconcile, requeue
// after each removal. Hibernation (replicas <= 0) is never treated as scale-in.
//
// Surplus PVC cleanup lives in pvccleanup.go.
package statefulset

import (
	"fmt"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdclient "github.com/gardener/etcd-druid/internal/client/etcd"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrRemoveEtcdMember is returned when removing an etcd member fails.
	ErrRemoveEtcdMember druidapicommon.ErrorCode = "ERR_REMOVE_ETCD_MEMBER"
	// ErrQuorumUnsafeMemberRemoval is returned from PreSync when removing the next
	// candidate would break quorum. The reconcile flow requeues and records it in
	// status.lastErrors; it clears once removal becomes safe.
	ErrQuorumUnsafeMemberRemoval druidapicommon.ErrorCode = "ERR_QUORUM_UNSAFE_MEMBER_REMOVAL"
)

// removeOneSurplusMember removes at most one surplus etcd member per reconcile
// and always requeues after a successful removal so the next reconcile can
// remove the next member or proceed to STS shrink. No-op when there is no surplus.
//
// Contract: at most one RemoveMember RPC per call.
func (r _resource) removeOneSurplusMember(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	names, err := r.surplusMemberNames(ctx, etcd)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}

	mc, err := r.openMemberClient(ctx, etcd)
	if err != nil {
		return err
	}
	defer r.closeMemberClient(ctx, mc)

	members, err := mc.ListMembers(ctx)
	if err != nil {
		return wrapMemberErr(err, etcd, "list members")
	}

	ordered := etcdmember.OrderRemovalCandidates(filterMembersByName(members, names))
	if len(ordered) == 0 {
		// etcd no longer knows about any surplus member.
		return nil
	}
	candidate := ordered[0]

	if !etcdmember.QuorumSafeToRemove(members, candidate.ID) {
		ctx.Logger.Info("holding scale-in: removing the selected member would break quorum",
			"member", candidate.Name, "memberID", hexID(candidate.ID))
		return druiderr.New(ErrQuorumUnsafeMemberRemoval, component.OperationPreSync,
			fmt.Sprintf("member %s not removed: removal would break quorum for etcd %v; waiting for the cluster to become healthy",
				candidate.Name, client.ObjectKeyFromObject(etcd)))
	}

	return r.removeAndRequeue(ctx, etcd, &candidate, mc)
}

// surplusMemberNames returns the union of scale-in surplus (ordinals >= spec.replicas
// when STS > desired) and bootstrap source members to decommission.
func (r _resource) surplusMemberNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (map[string]bool, error) {
	delta, err := kubernetes.ComputeScaleInReplicaDelta(ctx, r.client, etcd)
	if err != nil {
		return nil, druiderr.WrapError(err, ErrGetStatefulSet, component.OperationPreSync,
			fmt.Sprintf("failed to get StatefulSet while selecting members to remove for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}

	names := make(map[string]bool)
	for ordinal := etcd.Spec.Replicas; ordinal < etcd.Spec.Replicas+delta; ordinal++ {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, int(ordinal))
		names[druidv1alpha1.GetMemberName(etcd.Spec.MemberNamePrefix, podName)] = true
	}
	for _, n := range druidv1alpha1.GetBootstrapMemberNamesToDecommission(etcd) {
		names[n] = true
	}
	return names, nil
}

// filterMembersByName returns the members whose Name is present in names.
func filterMembersByName(members []etcdmember.Member, names map[string]bool) []etcdmember.Member {
	filtered := make([]etcdmember.Member, 0, len(names))
	for _, m := range members {
		if names[m.Name] {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// removeAndRequeue issues RemoveMember and always returns ErrRequeueAfter so
// the next reconcile removes the next surplus member or proceeds to STS shrink.
func (r _resource) removeAndRequeue(
	ctx component.OperatorContext,
	etcd *druidv1alpha1.Etcd,
	candidate *etcdmember.Member,
	mc etcdclient.MemberClient,
) error {
	ctx.Logger.Info("removing surplus etcd member for scale-in",
		"member", candidate.Name, "memberID", hexID(candidate.ID))
	if err := mc.RemoveMember(ctx, candidate.ID); err != nil {
		return wrapMemberErr(err, etcd, fmt.Sprintf("remove member %s", candidate.Name))
	}
	return druiderr.New(druiderr.ErrRequeueAfter, component.OperationPreSync,
		fmt.Sprintf("removed etcd member %s; requeuing to trim remaining surplus members for etcd %v",
			candidate.Name, client.ObjectKeyFromObject(etcd)))
}

func (r _resource) openMemberClient(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (etcdclient.MemberClient, error) {
	mc, err := r.memberClientFactory.NewMemberClient(ctx, r.client, etcd)
	if err != nil {
		return nil, druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to create etcd member client for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	return mc, nil
}

func (r _resource) closeMemberClient(ctx component.OperatorContext, mc etcdclient.MemberClient) {
	if err := mc.Close(); err != nil {
		ctx.Logger.Error(err, "failed to close etcd member client")
	}
}

func hexID(id uint64) string {
	return fmt.Sprintf("%x", id)
}

func wrapMemberErr(err error, etcd *druidv1alpha1.Etcd, what string) error {
	return druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
		fmt.Sprintf("%s for etcd: %v", what, client.ObjectKeyFromObject(etcd)))
}
