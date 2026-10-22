// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"context"
	"errors"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// requeueAfterInFlight is the ceiling for requeue delay used whenever the reconciler is
	// waiting on a physical event (pod deletion, recreation, or readiness).
	requeueAfterInFlight = 10 * time.Second
	// podDeletionPollInterval is how often deleteSelected polls the cache for a
	// pod's terminating state to become visible.
	podDeletionPollInterval = 200 * time.Millisecond
	// podDeletionPollTimeout bounds the wait for the cache to observe a Delete.
	// A timeout is non-fatal: the top gate on the next reconcile still holds.
	podDeletionPollTimeout = 10 * time.Second
)

// podUpdateState is the shared structure to hold the information about the pod-update steps in a reconciliation.
type podUpdateState struct {
	sts         *appsv1.StatefulSet
	etcd        *druidv1alpha1.Etcd
	pods        []corev1.Pod
	outdated    []corev1.Pod
	current     []corev1.Pod
	members     etcdmember.Members
	quorumState etcdmember.QuorumState
}

// podUpdateStepFn is one step of the pod-update procedure.
type podUpdateStepFn func(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult

// executePodUpdateProcedure runs the pod update procedure
func (r *Reconciler) executePodUpdateProcedure(ctx context.Context, logger logr.Logger, sts *appsv1.StatefulSet, etcd *druidv1alpha1.Etcd, pods []corev1.Pod) (ctrl.Result, error) {
	s := &podUpdateState{sts: sts, etcd: etcd, pods: pods}

	if result := runRolloutSteps(ctx, logger, s,
		r.classifyPods,          // partition pods by revision
		r.completeIfAllUpToDate, // nothing outdated -> rollout complete
		r.holdIfPodSetInFlight,  // a prior delete / scale-up has not settled -> hold
	); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result.ReconcileResult()
	}

	if isSingleMemberCluster(sts) {
		return r.reconcileSingleMemberRollout(ctx, logger, s).ReconcileResult()
	}
	return r.reconcileMultiMemberRollout(ctx, logger, s).ReconcileResult()
}

// runRolloutSteps runs steps in order, short-circuiting on the first step that
// does not continue, otherwise returning ContinueReconcile.
func runRolloutSteps(ctx context.Context, logger logr.Logger, s *podUpdateState, steps ...podUpdateStepFn) ctrlutils.ReconcileStepResult {
	for _, step := range steps {
		if result := step(ctx, logger, s); ctrlutils.ShortCircuitReconcileFlow(result) {
			return result
		}
	}
	return ctrlutils.ContinueReconcile()
}

// reconcileSingleMemberRollout handles a 1-replica cluster: there is no quorum to
// protect, so the sole outdated pod is deleted without dialing etcd.
func (r *Reconciler) reconcileSingleMemberRollout(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	logger.Info("single-member cluster: deleting the sole outdated pod", "pod", s.outdated[0].Name)
	return r.deleteSelectedPodAndWait(ctx, logger, &s.outdated[0])
}

// reconcileMultiMemberRollout runs a series of quorum-aware steps to progress the pod update process.
func (r *Reconciler) reconcileMultiMemberRollout(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	return runRolloutSteps(ctx, logger, s,
		r.listLiveMembers,            // dial etcd once: observe quorum + list members
		r.recoverIfQuorumLost,        // conclusive quorum loss -> batch-delete all outdated
		r.deleteNextNonParticipating, // delete a non-participating outdated pod
		r.holdForUpdatedToRejoin,     // gate: hold until updated pods rejoin, before touching a participant
		r.deleteNextParticipating,    // delete one participating outdated pod (follower before leader)
	)
}

// classifyPods partitions the pod set into outdated and current by revision.
func (r *Reconciler) classifyPods(_ context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	s.outdated, s.current = partitionByRevision(s.pods, s.sts)
	logger.V(1).Info("classified pods", "outdated", len(s.outdated), "current", len(s.current), "updateRevision", s.sts.Status.UpdateRevision)
	return ctrlutils.ContinueReconcile()
}

// completeIfAllUpToDate ends the rollout when no pod is outdated (Step 1).
func (r *Reconciler) completeIfAllUpToDate(_ context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	if len(s.outdated) == 0 {
		logger.V(1).Info("rollout complete: all pods are upto date")
		return ctrlutils.DoNotRequeue()
	}
	return ctrlutils.ContinueReconcile()
}

// holdIfPodSetInFlight holds while a prior Delete has not settled or the
// StatefulSet controller has not yet created every desired pod (scale-up).
// Subsequent steps reason about pod identity/count, so they must not run yet.
func (r *Reconciler) holdIfPodSetInFlight(_ context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	if reason := podSetInFlightReason(s.sts, s.pods); reason != "" {
		logger.Info("holding: pod set in flight", "reason", reason)
		return ctrlutils.ReconcileAfter(requeueAfterInFlight, reason)
	}
	return ctrlutils.ContinueReconcile()
}

// listLiveMembers dials the etcd cluster once and records its live membership
// and the conclusive quorum state of the cluster.
func (r *Reconciler) listLiveMembers(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	etcdClient, err := r.clientFactory.NewClient(ctx, r.client, s.etcd)
	if err != nil {
		logger.Info("holding: failed to create etcd client, requeuing", "err", err.Error())
		return ctrlutils.ReconcileAfter(requeueAfterInFlight, "failed to create etcd client")
	}
	defer func() {
		if cerr := etcdClient.Close(); cerr != nil {
			logger.Error(cerr, "failed to close etcd client")
		}
	}()

	// A conclusive quorum observation via a linearizable operation. A transient
	// failure (timeout, dial error) is returned as an error and makes us hold,
	// so we never treat an inconclusive signal as quorum loss.
	quorumState, err := etcdClient.ObserveQuorum(ctx)
	if err != nil {
		logger.Info("holding: could not conclusively observe quorum, requeuing", "err", err.Error())
		return ctrlutils.ReconcileAfter(requeueAfterInFlight, "could not conclusively observe quorum")
	}
	s.quorumState = quorumState

	members, err := etcdClient.MemberList(ctx)
	if err != nil {
		logger.Info("holding: failed to list etcd members, requeuing", "err", err.Error())
		return ctrlutils.ReconcileAfter(requeueAfterInFlight, "failed to list etcd members")
	}
	s.members = members
	return ctrlutils.ContinueReconcile()
}

// recoverIfQuorumLost batch-deletes all outdated pods only when etcd itself
// reports a conclusive quorum loss (no leader). There is then nothing left to
// protect, so recovering faster beats the one-at-a-time cadence.
func (r *Reconciler) recoverIfQuorumLost(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	if s.quorumState != etcdmember.QuorumStateLost {
		return ctrlutils.ContinueReconcile()
	}
	logger.Info("cluster has conclusively lost quorum: deleting all outdated pods to recover", "outdated", podNamesFromList(s.outdated))
	return r.deleteAllOutdatedPods(ctx, logger, s.outdated)
}

// deleteNextNonParticipating deletes one non-participating outdated pod. These
// contribute nothing to quorum, so they are updated first.
// The quorum check guards the deletion.
func (r *Reconciler) deleteNextNonParticipating(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	nonParticipating, _ := partitionOutdatedByParticipation(s.outdated)
	pod := selectNextPodForUpdate(s.etcd, nonParticipating, s.members)
	if pod == nil {
		return ctrlutils.ContinueReconcile()
	}
	logger.Info("deleting non-participating outdated pod", "pod", pod.Name)
	return r.deleteIfQuorumSafe(ctx, logger, s.etcd, s.members, pod)
}

// deleteNextParticipating deletes one participating outdated pod (follower before leader)
// Called only after the rejoin gate confirms all updated pods are participating again.
// The quorum check guards the deletion.
func (r *Reconciler) deleteNextParticipating(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	_, participating := partitionOutdatedByParticipation(s.outdated)
	pod := selectNextPodForUpdate(s.etcd, participating, s.members)
	if pod == nil {
		return ctrlutils.ContinueReconcile()
	}
	logger.Info("deleting participating outdated pod", "pod", pod.Name)
	return r.deleteIfQuorumSafe(ctx, logger, s.etcd, s.members, pod)
}

// holdForUpdatedToRejoin holds until every already-updated pod is participating
// again (Step 3), so a second participating member is never removed while a
// previously-updated one is still recovering.
func (r *Reconciler) holdForUpdatedToRejoin(_ context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	if unready := updatedNonParticipatingPodNames(s.current); len(unready) > 0 {
		logger.Info("step 3: holding for updated pods to rejoin quorum", "waitingOn", unready)
		return ctrlutils.ReconcileAfter(requeueAfterInFlight, fmt.Sprintf("waiting for updated pods to rejoin quorum: %v", unready))
	}
	return ctrlutils.ContinueReconcile()
}

// isSingleMemberCluster reports whether the StatefulSet manages a single replica.
func isSingleMemberCluster(sts *appsv1.StatefulSet) bool {
	return ptr.Deref(sts.Spec.Replicas, 0) == 1
}

// partitionOutdatedByParticipation splits outdated pods into non-participating and participating in quorum.
func partitionOutdatedByParticipation(outdated []corev1.Pod) (nonParticipating, participating []corev1.Pod) {
	for i := range outdated {
		if readinessOfEtcdContainer(&outdated[i]) == etcdContainerNotReady {
			nonParticipating = append(nonParticipating, outdated[i])
		} else {
			participating = append(participating, outdated[i])
		}
	}
	return nonParticipating, participating
}

// partitionByRevision splits the given set of pods into outdated and current sets by comparing
// each pod's controller-revision-hash against sts.Status.UpdateRevision.
func partitionByRevision(pods []corev1.Pod, sts *appsv1.StatefulSet) (outdatedPodSet, currentPodSet []corev1.Pod) {
	for i := range pods {
		if kubernetes.IsStatefulSetPodOutdated(&pods[i], sts) {
			outdatedPodSet = append(outdatedPodSet, pods[i])
		} else {
			currentPodSet = append(currentPodSet, pods[i])
		}
	}
	return outdatedPodSet, currentPodSet
}

// podSetInFlightReason returns a short reason string if the pod set is not yet
// at its target shape, or the empty string if and wwhen it is settled.
func podSetInFlightReason(sts *appsv1.StatefulSet, pods []corev1.Pod) string {
	if terminatingPodSet := terminatingPodNames(pods); len(terminatingPodSet) > 0 {
		return fmt.Sprintf("pods terminating: %v", terminatingPodSet)
	}
	desiredReplicaCount := int(ptr.Deref(sts.Spec.Replicas, 0))
	if len(pods) < desiredReplicaCount {
		return fmt.Sprintf("pod count %d below desired %d", len(pods), desiredReplicaCount)
	}
	return ""
}

// updatedNonParticipatingPodNames returns the names of current revision pods that are
// not participating in quorum.
func updatedNonParticipatingPodNames(current []corev1.Pod) []string {
	var names []string
	for i := range current {
		// A pod participates in quorum only when its etcd container is Ready.
		if readinessOfEtcdContainer(&current[i]) != etcdContainerReady {
			names = append(names, current[i].Name)
		}
	}
	return names
}

// terminatingPodNames returns the names of pods with DeletionTimestamp set.
func terminatingPodNames(pods []corev1.Pod) []string {
	var terminatingPodNames []string
	for i := range pods {
		if pods[i].DeletionTimestamp != nil {
			terminatingPodNames = append(terminatingPodNames, pods[i].Name)
		}
	}
	return terminatingPodNames
}

// deleteIfQuorumSafe deletes the selected pod only when removing its etcd member
// keeps a healthy voting quorum. The pod is mapped to its member by the etcd
// member name (which carries spec.memberNamePrefix). It fails closed: if the pod
// has no live member, or removal would break quorum, it holds without deleting.
func (r *Reconciler) deleteIfQuorumSafe(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, members etcdmember.Members, pod *corev1.Pod) ctrlutils.ReconcileStepResult {
	memberName := druidv1alpha1.GetMemberName(etcd.Spec.MemberNamePrefix, pod.Name)
	var member *etcdmember.Member
	for i := range members {
		if members[i].Name == memberName {
			member = &members[i]
			break
		}
	}
	if member == nil {
		logger.Info("holding: selected pod has no live etcd member, requeuing", "pod", pod.Name, "member", memberName)
		return ctrlutils.ReconcileAfter(requeueAfterInFlight, fmt.Sprintf("pod %s has no live etcd member", pod.Name))
	}
	canRemove, err := etcdmember.CanRemoveMemberWithoutLosingQuorum(members, member.ID)
	if err != nil || !canRemove {
		logger.Info("holding: deleting selected pod would break quorum, requeuing", "pod", pod.Name, "member", memberName, "err", err)
		return ctrlutils.ReconcileAfter(requeueAfterInFlight, fmt.Sprintf("deleting pod %s would break quorum", pod.Name))
	}
	return r.deleteSelectedPodAndWait(ctx, logger, pod)
}

// deleteAllOutdatedPods deletes every outdated pod in one pass.
func (r *Reconciler) deleteAllOutdatedPods(ctx context.Context, logger logr.Logger, outdatedPodSet []corev1.Pod) ctrlutils.ReconcileStepResult {
	for i := range outdatedPodSet {
		if _, err := r.deleteSelectedPod(ctx, logger, &outdatedPodSet[i]); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ReconcileAfter(requeueAfterInFlight, "deleted all outdated pods to recover from quorum loss")
}

// deleteSelectedPodAndWait issues a Delete for the pod and waits until the local cache observes the
// resulting terminating state, to make sure successive reconciles do not see stale pre-delete state.
func (r *Reconciler) deleteSelectedPodAndWait(ctx context.Context, logger logr.Logger, pod *corev1.Pod) ctrlutils.ReconcileStepResult {
	deleted, err := r.deleteSelectedPod(ctx, logger, pod)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if deleted {
		if err := r.waitForPodDeletionObserved(ctx, logger, pod); err != nil {
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ReconcileAfter(requeueAfterInFlight, fmt.Sprintf("deleted pod %s; waiting for it to be recreated at the update revision", pod.Name))
}

// deleteSelectedPod deletes a single pod, guarding against a pod that is already
// terminating or already gone.
// It reports whether this call was able to issue the Delete.
func (r *Reconciler) deleteSelectedPod(ctx context.Context, logger logr.Logger, pod *corev1.Pod) (bool, error) {
	if pod.DeletionTimestamp != nil {
		logger.V(1).Info("selected pod is already terminating, skipping delete", "pod", pod.Name)
		return false, nil
	}
	if err := r.client.Delete(ctx, pod); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.V(1).Info("pod already deleted by another actor", "pod", pod.Name)
			return false, nil
		}
		return false, fmt.Errorf("failed to delete pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	return true, nil
}

// waitForPodDeletionObserved blocks until the local cache reflects the pod's
// deletion or the poll timeout expires.
func (r *Reconciler) waitForPodDeletionObserved(ctx context.Context, logger logr.Logger, pod *corev1.Pod) error {
	err := wait.PollUntilContextTimeout(ctx, podDeletionPollInterval, podDeletionPollTimeout, true,
		func(ctx context.Context) (bool, error) {
			observed := &corev1.Pod{}
			errGetPod := r.client.Get(ctx, client.ObjectKeyFromObject(pod), observed)
			if apierrors.IsNotFound(errGetPod) {
				return true, nil
			}
			if errGetPod != nil {
				return false, errGetPod
			}
			return observed.DeletionTimestamp != nil, nil
		})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.V(1).Info("timeout waiting for pod deletion to be observed, proceeding", "pod", pod.Name, "timeout", podDeletionPollTimeout)
			return nil
		}
		return fmt.Errorf("waiting for pod %s/%s deletion to be observed: %w", pod.Namespace, pod.Name, err)
	}
	return nil
}

// podNamesFromList returns the names of the given pods, or nil for empty input.
func podNamesFromList(pods []corev1.Pod) []string {
	if len(pods) == 0 {
		return nil
	}
	names := make([]string, 0, len(pods))
	for i := range pods {
		names = append(names, pods[i].Name)
	}
	return names
}
