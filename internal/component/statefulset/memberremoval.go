// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"
	"strconv"
	"strings"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	etcdutils "github.com/gardener/etcd-druid/internal/utils/etcd"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrRemoveEtcdMember indicates an error removing an etcd member during scale-in.
	ErrRemoveEtcdMember druidapicommon.ErrorCode = "ERR_REMOVE_ETCD_MEMBER"
	// ErrDeletePVC indicates an error deleting a surplus PersistentVolumeClaim during scale-in.
	ErrDeletePVC druidapicommon.ErrorCode = "ERR_DELETE_PVC"
)

// ensureMemberRemoval removes at most one surplus etcd member per reconcile
// before the StatefulSet is scaled in, so that the cluster never drops below
// quorum. It is a no-op unless a scale-in (or bootstrap members
// removal) is in progress, and it deliberately removes only a single member per
// invocation, requeuing until no surplus members remain.
//
// The flow is:
//  1. Determine the set of surplus members from spec vs the observed StatefulSet
//     and the recorded bootstrap-join status. If none, return (no-op).
//  2. List the live members from etcd via the MemberClient.
//  3. Pre-check quorum safety for the next candidate; if unsafe, record the
//     condition and requeue rather than issue a removal etcd would reject.
//  4. Remove exactly one member (learners first, leader last), then requeue so
//     the StatefulSet shrink only proceeds once membership has been trimmed.
func (r _resource) ensureMemberRemoval(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	// A nil factory means member removal is not wired (e.g. in unit tests that do
	// not exercise scale-in); treat it as a no-op.
	if r.memberClientFactory == nil {
		return nil
	}

	surplus, err := r.surplusMemberNames(ctx, etcd)
	if err != nil {
		return err
	}
	if len(surplus) == 0 {
		return nil
	}

	memberClient, err := r.memberClientFactory.NewMemberClient(ctx, r.client, etcd)
	if err != nil {
		return druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to create etcd member client for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	defer func() {
		if cerr := memberClient.Close(); cerr != nil {
			ctx.Logger.Error(cerr, "failed to close etcd member client")
		}
	}()

	members, err := memberClient.ListMembers(ctx)
	if err != nil {
		return druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to list etcd members for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}

	// Restrict removal candidates to the members etcd actually reports and which
	// the desired state no longer wants.
	candidates := filterMembersByName(members, surplus)
	if len(candidates) == 0 {
		// etcd no longer knows about the surplus members; the shrink can proceed.
		return nil
	}

	healthyIDs, knownIDs, leaderID := healthyAndLeaderMemberIDs(etcd)
	ordered := etcdutils.OrderRemovalCandidates(candidates, leaderID)
	candidate := ordered[0]

	if !etcdutils.QuorumSafeToRemove(members, candidate, healthyIDs, knownIDs) {
		ctx.Logger.Info("holding scale-in: removing the selected member would break quorum",
			"member", candidate.Name, "memberID", fmt.Sprintf("%x", candidate.ID))
		return druiderr.New(druiderr.ErrRequeueAfter, component.OperationPreSync,
			fmt.Sprintf("removing member %s would break quorum for etcd %v; waiting for the cluster to become healthy",
				candidate.Name, client.ObjectKeyFromObject(etcd)))
	}

	ctx.Logger.Info("removing surplus etcd member for scale-in",
		"member", candidate.Name, "memberID", fmt.Sprintf("%x", candidate.ID))
	if err := memberClient.RemoveMember(ctx, candidate.ID); err != nil {
		return druiderr.WrapError(err, ErrRemoveEtcdMember, component.OperationPreSync,
			fmt.Sprintf("failed to remove etcd member %s for etcd: %v", candidate.Name, client.ObjectKeyFromObject(etcd)))
	}

	// Requeue so the StatefulSet shrink only proceeds once the removed member's
	// pod has been terminated on a subsequent reconcile and no surplus remains.
	return druiderr.New(druiderr.ErrRequeueAfter, component.OperationPreSync,
		fmt.Sprintf("removed etcd member %s; requeuing to trim remaining surplus members for etcd %v",
			candidate.Name, client.ObjectKeyFromObject(etcd)))
}

// surplusMemberNames returns the set of etcd member names that the desired state
// no longer wants and which must be removed from the cluster before the
// StatefulSet is scaled in. It combines two sources:
//
//   - scale-in: members backing pod ordinals >= spec.replicas (only when the
//     observed StatefulSet is larger than the desired replica count), and
//   - bootstrap members removal: members joined via bootstrapWithExistingCluster
//     that are no longer present in the spec.
//
// It returns an empty set when no scale-in or bootstrap-removal is in progress.
func (r _resource) surplusMemberNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (map[string]bool, error) {
	surplus := map[string]bool{}

	// Scale-in surplus: pod ordinals at or above the desired replica count, but
	// only if the StatefulSet is currently larger than desired. Transitions
	// to/from zero (hibernation) are handled elsewhere and are not scale-ins.
	if etcd.Spec.Replicas > 0 {
		sts, err := kubernetes.GetStatefulSet(ctx, r.client, etcd)
		if err != nil {
			return nil, druiderr.WrapError(err, ErrGetStatefulSet, component.OperationPreSync,
				fmt.Sprintf("failed to get StatefulSet while selecting members to remove for etcd: %v", client.ObjectKeyFromObject(etcd)))
		}
		if sts != nil {
			stsReplicas := ptr.Deref(sts.Spec.Replicas, 0)
			if stsReplicas > etcd.Spec.Replicas {
				for ordinal := etcd.Spec.Replicas; ordinal < stsReplicas; ordinal++ {
					podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, int(ordinal))
					surplus[druidv1alpha1.GetMemberName(etcd.Spec.MemberNamePrefix, podName)] = true
				}
			}
		}
	}

	// Bootstrap members removal: joined source members no longer present in spec.
	for _, name := range druidv1alpha1.GetBootstrapMemberNamesToDecommission(etcd) {
		surplus[name] = true
	}

	return surplus, nil
}

// filterMembersByName returns the members whose Name is present in names.
func filterMembersByName(members []etcdutils.Member, names map[string]bool) []etcdutils.Member {
	filtered := make([]etcdutils.Member, 0, len(names))
	for _, m := range members {
		if names[m.Name] {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// healthyAndLeaderMemberIDs derives, from the Etcd status members, three
// values used by QuorumSafeToRemove:
//   - healthy: IDs of members with status Ready (known healthy)
//   - known:   IDs of all members tracked in Etcd status (healthy or not)
//   - leaderID: ID of the current leader (0 if unknown)
//
// Member IDs in status originate from the member lease holder identity written
// by etcd-backup-restore and are rendered as hex. Passing both the healthy and
// known sets lets QuorumSafeToRemove treat members absent from the status (e.g.
// source cluster members during bootstrap removal) as healthy rather than
// mistaking them for unhealthy members.
func healthyAndLeaderMemberIDs(etcd *druidv1alpha1.Etcd) (healthy map[uint64]bool, known map[uint64]bool, leaderID uint64) {
	healthy = map[uint64]bool{}
	known = map[uint64]bool{}
	for _, m := range etcd.Status.Members {
		if m.ID == nil {
			continue
		}
		id, ok := parseMemberID(*m.ID)
		if !ok {
			continue
		}
		known[id] = true
		if m.Status == druidv1alpha1.EtcdMemberStatusReady {
			healthy[id] = true
		}
		if m.Role != nil && *m.Role == druidv1alpha1.EtcdRoleLeader {
			leaderID = id
		}
	}
	return healthy, known, leaderID
}

// parseMemberID parses an etcd member ID string as recorded in the Etcd status.
// The ID originates from the member lease holder identity written by
// etcd-backup-restore, which encodes it as the hex form of the member ID
// (types.ID.String()); see the health check's extractMemberIdAndRole. It is
// therefore parsed as base-16 only — a value that does not parse as hex is
// treated as unknown rather than silently reinterpreted in another base.
func parseMemberID(s string) (uint64, bool) {
	id, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func isScaleInInProgress(etcd *druidv1alpha1.Etcd) bool {
	for _, cond := range etcd.Status.Conditions {
		if cond.Type == druidv1alpha1.ConditionTypeScaleOperationComplete &&
			cond.Status == druidv1alpha1.ConditionFalse &&
			cond.Reason == druidv1alpha1.ScaleOperationReasonScalingIn {
			return true
		}
	}
	return false
}

// deleteSurplusPVCs deletes the PersistentVolumeClaims backing pod ordinals at
// or above the desired replica count during a scale-in. A StatefulSet
// does not reclaim its per-pod PVCs when scaled down, so without this the
// storage of removed members would leak. It never acts on a transition to zero
// replicas (hibernation), which is not a scale-in.
//
// The surplus set is derived from the PVCs that actually exist rather than from
// the StatefulSet's replica count: Sync patches the StatefulSet down to the
// desired size before this runs, so the observed StatefulSet no longer reflects
// the pre-scale-in width. Every managed PVC follows the StatefulSet naming
// convention `<vctName>-<stsName>-<ordinal>`, and any PVC whose ordinal is at or
// above spec.replicas is surplus. Deletion is best-effort and idempotent
// (missing PVCs are ignored), so a re-run after a partial failure converges.
func (r _resource) deleteSurplusPVCs(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	// A transition to zero replicas is hibernation, not a scale-in; leave the
	// PVCs intact so the cluster can be woken up with its data.
	if etcd.Spec.Replicas <= 0 {
		return nil
	}
	// Deleting ordinal PVCs is only safe for an explicitly recorded scale-in. Do
	// not infer scale-in from "PVC ordinal >= spec.replicas" alone: during wake-up
	// from hibernation, the StatefulSet can legitimately be at 0 while higher
	// ordinal PVCs from the pre-hibernation cluster still exist and must be kept.
	if !isScaleInInProgress(etcd) {
		return nil
	}

	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.client.List(ctx, pvcList,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)),
	); err != nil {
		return druiderr.WrapError(err, ErrDeletePVC, component.OperationSync,
			fmt.Sprintf("failed to list PVCs while deleting surplus PVCs for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}

	vctName := ptr.Deref(etcd.Spec.VolumeClaimTemplate, etcd.Name)
	prefix := fmt.Sprintf("%s-%s-", vctName, druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta))
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		ordinalStr, ok := strings.CutPrefix(pvc.Name, prefix)
		if !ok {
			continue
		}
		ordinal, err := strconv.Atoi(ordinalStr)
		if err != nil {
			// Not an ordinal-suffixed member PVC; leave it untouched.
			continue
		}
		if int32(ordinal) < etcd.Spec.Replicas {
			continue
		}
		ctx.Logger.Info("deleting surplus PVC for scale-in", "pvc", pvc.Name)
		if err := r.client.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
			return druiderr.WrapError(err, ErrDeletePVC, component.OperationSync,
				fmt.Sprintf("failed to delete surplus PVC %s for etcd: %v", pvc.Name, client.ObjectKeyFromObject(etcd)))
		}
	}
	return nil
}
