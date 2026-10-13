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
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"

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

// executePodUpdateProcedure runs the pod update procedure
func (r *Reconciler) executePodUpdateProcedure(ctx context.Context, logger logr.Logger, sts *appsv1.StatefulSet, etcd *druidv1alpha1.Etcd, pods []corev1.Pod) (ctrl.Result, error) {
	outdatedPodSet, currentPodSet := partitionByRevision(pods, sts)
	logger.V(1).Info("classified pods", "outdated", len(outdatedPodSet), "current", len(currentPodSet), "updateRevision", sts.Status.UpdateRevision)

	// Step 1: no outdated pods -> rollout complete.
	if len(outdatedPodSet) == 0 {
		logger.V(1).Info("rollout complete: all pods are upto date")
		return ctrl.Result{}, nil
	}

	// If the pod set is mid-flight, hold the update process. Reasons being, a prior Delete has not settled, or the
	// StatefulSet controller has not yet created every desired pod (scale-up).
	// Blocks all subsequent steps because they reason about pod identity/count.
	if reason := podSetInFlightReason(sts, pods); reason != "" {
		logger.Info("holding: pod set in flight", "reason", reason)
		return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
	}

	if ptr.Deref(sts.Spec.Replicas, 0) == 1 {
		logger.Info("single-node cluster: deleting the sole outdated pod", "pod", outdatedPodSet[0].Name)
		return r.deleteSelectedPodAndWait(ctx, logger, &outdatedPodSet[0])
	}

	members, result, err := r.listLiveMembers(ctx, logger, etcd)
	if err != nil || members == nil {
		return result, err
	}

	// If the quorum is lost, it is better to delete all outdated members together to recover faster as it does not make the cluster go into a bad state.
	if !etcdmember.HasQuorum(members) {
		logger.Info("cluster has lost quorum: deleting all outdated pods to recover", "outdated", podNamesFromList(outdatedPodSet))
		return r.deleteAllOutdatedPods(ctx, logger, outdatedPodSet)
	}

	// Step 2: prefer outdated non-participating pods first.
	// The operation is safe regardless of quorum state because a non-participating pod contributes nothing to quorum.
	if pod, bucket := selectOutdatedNonParticipatingPods(logger, outdatedPodSet); pod != nil {
		logger.Info("step 2: deleting non-participating outdated pod", "pod", pod.Name, "bucket", bucket)
		return r.deleteIfQuorumSafe(ctx, logger, etcd, members, pod)
	}

	// Step 3: before deleting a participating pod, all updated pods must be participating.
	if unready := updatedNotParticipatingPodNames(currentPodSet); len(unready) > 0 {
		logger.Info("step 3: holding for updated pods to rejoin quorum", "waitingOn", unready)
		return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
	}

	// Step 4: pick one participating outdated pod in the order of follower before leader to minimise the number of leader elections.
	pod, role, err := r.selectOutdatedParticipatingPods(ctx, logger, etcd, outdatedPodSet)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pod != nil {
		logger.Info("step 4: deleting participating outdated pod", "pod", pod.Name, "role", role)
		return r.deleteIfQuorumSafe(ctx, logger, etcd, members, pod)
	}

	// this should be unreachable due to the above gates but is kept only as a defensive mechanism.
	logger.V(1).Info("no pod selected despite outdated pods existing, requeuing", "outdatedCount", len(outdatedPodSet))
	return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
}

// partitionByRevision splits the given set of pods into outdated and current sets by comparing
// each pod's controller-revision-hash against sts.Status.UpdateRevision.
func partitionByRevision(pods []corev1.Pod, sts *appsv1.StatefulSet) (outdatedPodSet, currentPodSet []corev1.Pod) {
	for i := range pods {
		if isPodOutdated(&pods[i], sts) {
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

// updatedNotParticipatingPodNames returns the names of current revision pods that are
// not participating in quorum.
func updatedNotParticipatingPodNames(current []corev1.Pod) []string {
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

// listLiveMembers dials the etcd cluster and returns its live membership
func (r *Reconciler) listLiveMembers(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (etcdmember.Members, ctrl.Result, error) {
	etcdClient, err := r.clientFactory.NewClient(ctx, r.client, etcd)
	if err != nil {
		logger.Info("holding: failed to create etcd client, requeuing", "err", err.Error())
		return nil, ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
	}
	defer func() {
		if cerr := etcdClient.Close(); cerr != nil {
			logger.Error(cerr, "failed to close etcd client")
		}
	}()

	members, err := etcdClient.MemberList(ctx)
	if err != nil {
		logger.Info("holding: failed to list etcd members, requeuing", "err", err.Error())
		return nil, ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
	}
	return members, ctrl.Result{}, nil
}

// deleteIfQuorumSafe deletes the selected pod only when removing its etcd member
// keeps a healthy voting quorum.
func (r *Reconciler) deleteIfQuorumSafe(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, members etcdmember.Members, pod *corev1.Pod) (ctrl.Result, error) {
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
		return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
	}
	if !etcdmember.QuorumSafeToRemove(members, member.ID) {
		logger.Info("holding: deleting selected pod would break quorum, requeuing", "pod", pod.Name, "member", memberName)
		return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
	}
	return r.deleteSelectedPodAndWait(ctx, logger, pod)
}

// deleteAllOutdatedPods deletes every outdated pod in one pass.
func (r *Reconciler) deleteAllOutdatedPods(ctx context.Context, logger logr.Logger, outdatedPodSet []corev1.Pod) (ctrl.Result, error) {
	for i := range outdatedPodSet {
		if _, err := r.deleteSelectedPod(ctx, logger, &outdatedPodSet[i]); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
}

// deleteSelectedPodAndWait issues a Delete for the pod and waits until the local cache observes the
// resulting terminating state, to make sure successive reconciles do not see stale pre-delete state.
func (r *Reconciler) deleteSelectedPodAndWait(ctx context.Context, logger logr.Logger, pod *corev1.Pod) (ctrl.Result, error) {
	deleted, err := r.deleteSelectedPod(ctx, logger, pod)
	if err != nil {
		return ctrl.Result{}, err
	}
	if deleted {
		if err := r.waitForPodDeletionObserved(ctx, logger, pod); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
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
