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
	sts      *appsv1.StatefulSet
	etcd     *druidv1alpha1.Etcd
	pods     []corev1.Pod
	outdated []corev1.Pod
	current  []corev1.Pod
	members  etcdmember.Members
}

// podUpdateStepFn is one step of the pod-update procedure.
type podUpdateStepFn func(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult

// executePodUpdateProcedure runs the pod update process as an ordered set of steps
func (r *Reconciler) executePodUpdateProcedure(ctx context.Context, logger logr.Logger, sts *appsv1.StatefulSet, etcd *druidv1alpha1.Etcd, pods []corev1.Pod) (ctrl.Result, error) {
	s := &podUpdateState{sts: sts, etcd: etcd, pods: pods}
	steps := []podUpdateStepFn{
		r.classifyPods,            // partition pods by revision
		r.completeIfAllUpToDate,   // Step 1 of DEP-07
		r.holdIfPodSetInFlight,    // in-flight gate
		r.handleSingleNodeCluster, // single-node short-circuit (no quorum to protect)
		r.listLiveMembers,         // dial etcd once, before any multi-node delete
		r.recoverIfQuorumLost,     // quorum already lost -> batch-delete all outdated
		r.deleteNonParticipating,  // Step 2 of DEP-07
		r.holdForUpdatedToRejoin,  // Step 3 of DEP-07
		r.deleteParticipating,     // Step 4 of DEP-07
	}
	for _, step := range steps {
		if result := step(ctx, logger, s); ctrlutils.ShortCircuitReconcileFlow(result) {
			return result.ReconcileResult()
		}
	}
	// Unreachable in practice: Step 4 always short-circuits when outdated pods remain.
	logger.V(1).Info("no pod selected despite outdated pods existing, requeuing", "outdatedCount", len(s.outdated))
	return ctrlutils.ReconcileAfter(requeueAfterInFlight, "no pod selected; requeuing defensively").ReconcileResult()
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

// handleSingleNodeCluster short-circuits a 1-replica cluster: there is no quorum
// to protect, so the sole outdated pod is deleted without dialing etcd.
func (r *Reconciler) handleSingleNodeCluster(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	if ptr.Deref(s.sts.Spec.Replicas, 0) != 1 {
		return ctrlutils.ContinueReconcile()
	}
	logger.Info("single-node cluster: deleting the sole outdated pod", "pod", s.outdated[0].Name)
	return r.deleteSelectedPodAndWait(ctx, logger, &s.outdated[0])
}

// listLiveMembers dials the etcd cluster once and records its live membership on
// the state for the quorum checks below. It fails closed: on any dial or list
// error it holds without deleting.
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

	members, err := etcdClient.MemberList(ctx)
	if err != nil {
		logger.Info("holding: failed to list etcd members, requeuing", "err", err.Error())
		return ctrlutils.ReconcileAfter(requeueAfterInFlight, "failed to list etcd members")
	}
	s.members = members
	return ctrlutils.ContinueReconcile()
}

// recoverIfQuorumLost batch-deletes all outdated pods when the live cluster has
// already lost quorum: there is nothing left to protect, so recovering faster
// beats the one-at-a-time cadence.
func (r *Reconciler) recoverIfQuorumLost(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	if etcdmember.HasQuorum(s.members) {
		return ctrlutils.ContinueReconcile()
	}
	logger.Info("cluster has lost quorum: deleting all outdated pods to recover", "outdated", podNamesFromList(s.outdated))
	return r.deleteAllOutdatedPods(ctx, logger, s.outdated)
}

// deleteNonParticipating deletes one non-participating outdated pod (Step 2),
// subject to the quorum check. These contribute nothing to quorum, so they are
// preferred before any participating pod.
func (r *Reconciler) deleteNonParticipating(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	pod, bucket := selectOutdatedNonParticipatingPods(logger, s.outdated)
	if pod == nil {
		return ctrlutils.ContinueReconcile()
	}
	logger.Info("step 2: deleting non-participating outdated pod", "pod", pod.Name, "bucket", bucket)
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

// deleteParticipating deletes one participating outdated pod (Step 4), follower
// before leader to minimise leader elections, subject to the quorum check.
func (r *Reconciler) deleteParticipating(ctx context.Context, logger logr.Logger, s *podUpdateState) ctrlutils.ReconcileStepResult {
	pod, role, err := r.selectOutdatedParticipatingPods(ctx, logger, s.etcd, s.outdated)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if pod == nil {
		return ctrlutils.ContinueReconcile()
	}
	logger.Info("step 4: deleting participating outdated pod", "pod", pod.Name, "role", role)
	return r.deleteIfQuorumSafe(ctx, logger, s.etcd, s.members, pod)
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

// selectOutdatedNonParticipatingPods returns the highest-priority non-participating
// outdated pod for Step 2 (priority: Dead > Transient > Alive).
// Pods with Unknown-state are skipped due to insufficient evidence to say they are non-participating.
func selectOutdatedNonParticipatingPods(logger logr.Logger, outdatedPodSet []corev1.Pod) (*corev1.Pod, string) {
	var deadPodSet, transientPodSet, alivePodSet []*corev1.Pod
	for i := range outdatedPodSet {
		pod := &outdatedPodSet[i]
		if isPodParticipating(pod) {
			continue
		}
		switch classifyEtcdContainer(pod) {
		case containerStateDead:
			deadPodSet = append(deadPodSet, pod)
		case containerStateTransient:
			transientPodSet = append(transientPodSet, pod)
		case containerStateAlive:
			alivePodSet = append(alivePodSet, pod)
		}
	}
	logger.V(1).Info("non-participating outdated pod classification",
		"dead", podNames(deadPodSet), "transient", podNames(transientPodSet), "alive", podNames(alivePodSet))

	if len(deadPodSet) > 0 {
		return deadPodSet[0], "Dead"
	}
	if len(transientPodSet) > 0 {
		return transientPodSet[0], "Transient"
	}
	if len(alivePodSet) > 0 {
		return alivePodSet[0], "Alive"
	}
	return nil, ""
}

// updatedNonParticipatingPodNames returns the names of current revision pods that are
// not participating in quorum.
func updatedNonParticipatingPodNames(current []corev1.Pod) []string {
	var names []string
	for i := range current {
		if !isPodParticipating(&current[i]) {
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

// selectOutdatedParticipatingPods picks a follower before leader.
// A lease-read failure is also treated as an unknown role and the pod is classified
// as a follower.
func (r *Reconciler) selectOutdatedParticipatingPods(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, outdatedPodSet []corev1.Pod) (*corev1.Pod, string, error) {
	var followers, leaders []*corev1.Pod
	for i := range outdatedPodSet {
		pod := &outdatedPodSet[i]
		role, err := r.memberRole(ctx, etcd, pod)
		if err != nil {
			logger.V(1).Info("treating pod as a follower after lease-read failure", "pod", pod.Name, "err", err.Error())
		}
		if isLeader(role) {
			leaders = append(leaders, pod)
		} else {
			followers = append(followers, pod)
		}
	}
	logger.V(1).Info("participating outdated pod classification",
		"followers", podNames(followers), "leaders", podNames(leaders))

	if len(followers) > 0 {
		return followers[0], "Follower", nil
	}
	if len(leaders) > 0 {
		return leaders[0], "Leader", nil
	}
	return nil, "", nil
}

// deleteIfQuorumSafe deletes the selected pod only when removing its etcd member
// keeps a healthy voting quorum.
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
	if !etcdmember.QuorumSafeToRemove(members, member.ID) {
		logger.Info("holding: deleting selected pod would break quorum, requeuing", "pod", pod.Name, "member", memberName)
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

// podNames returns the names of the given pods, or nil for empty input.
func podNames(pods []*corev1.Pod) []string {
	if len(pods) == 0 {
		return nil
	}
	names := make([]string, 0, len(pods))
	for _, p := range pods {
		names = append(names, p.Name)
	}
	return names
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
