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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// requeueAfterInFlight is the requeue delay used whenever the reconciler is
	// waiting on a physical event (pod deletion, recreation, or readiness).
	// The Pod watch usually re-enqueues sooner via a state change; this is the ceiling.
	requeueAfterInFlight = 10 * time.Second

	// podDeletionPollInterval is how often deleteSelected polls the cache for a
	// pod's terminating state to become visible.
	podDeletionPollInterval = 200 * time.Millisecond
	// podDeletionPollTimeout bounds the wait for the cache to observe a Delete.
	// A timeout is non-fatal: the top gate on the next reconcile still holds.
	podDeletionPollTimeout = 10 * time.Second
)

// executePodUpdateProcedure runs the pod update procedure per DEP-07 and issues
// at most one Delete per invocation.
func (r *Reconciler) executePodUpdateProcedure(
	ctx context.Context,
	logger logr.Logger,
	sts *appsv1.StatefulSet,
	etcd *druidv1alpha1.Etcd,
	pods []corev1.Pod,
) (ctrl.Result, error) {
	outdated, current := partitionByRevision(pods, sts)
	logger.V(1).Info("classified pods", "outdated", len(outdated), "current", len(current), "updateRevision", sts.Status.UpdateRevision)

	// Step 1: no outdated pods -> rollout complete.
	if len(outdated) == 0 {
		logger.V(1).Info("step 1: rollout complete")
		return ctrl.Result{}, nil
	}

	// Hold if the pod set is mid-flight: a prior Delete has not settled, or the
	// StatefulSet controller has not yet created every desired pod (scale-up).
	// Blocks all subsequent steps because they reason about pod identity/count.
	if reason := podSetInFlightReason(sts, pods); reason != "" {
		logger.Info("holding: pod set in flight", "reason", reason)
		return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
	}

	// Step 2: prefer non-participating outdated pods. Safe regardless of quorum
	// state — a non-participating pod contributes nothing to quorum.
	if pod, bucket := selectNonParticipatingOutdated(logger, outdated); pod != nil {
		logger.Info("step 2: deleting non-participating outdated pod", "pod", pod.Name, "bucket", bucket)
		return r.deleteSelected(ctx, logger, pod)
	}

	// Step 3: before touching a participating pod, every already-updated pod
	// must be participating.
	if unready := unreadyUpdatedPodNames(current); len(unready) > 0 {
		logger.Info("step 3: holding for updated pods to rejoin quorum", "waitingOn", unready)
		return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
	}

	// Step 4: pick one participating outdated pod, follower before leader.
	pod, role, err := r.selectParticipatingOutdated(ctx, logger, etcd, outdated)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pod != nil {
		logger.Info("step 4: deleting participating outdated pod", "pod", pod.Name, "role", role)
		return r.deleteSelected(ctx, logger, pod)
	}

	// generally unreachable due to the gates above. Kept as a defensive mechanism.
	logger.V(1).Info("no pod selected despite outdated pods existing, requeuing", "outdatedCount", len(outdated))
	return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
}

// partitionByRevision splits pods into outdated and current sets by comparing
// each pod's controller-revision-hash against sts.Status.UpdateRevision.
func partitionByRevision(pods []corev1.Pod, sts *appsv1.StatefulSet) (outdated, current []corev1.Pod) {
	for i := range pods {
		if isPodOutdated(&pods[i], sts) {
			outdated = append(outdated, pods[i])
		} else {
			current = append(current, pods[i])
		}
	}
	return outdated, current
}

// podSetInFlightReason returns a short reason string if the pod set is not yet
// at its target shape, or the empty string when it is settled. The pod set is
// in flight if any pod is terminating (a prior Delete has not completed) or if
// the observed pod count is below sts.Spec.Replicas (STS controller has not
// finished creating pods; e.g. mid-scale-up or mid-recreate).
func podSetInFlightReason(sts *appsv1.StatefulSet, pods []corev1.Pod) string {
	if terminating := terminatingPodNames(pods); len(terminating) > 0 {
		return fmt.Sprintf("pods terminating: %v", terminating)
	}
	desired := int32(0)
	if sts.Spec.Replicas != nil {
		desired = *sts.Spec.Replicas
	}
	if int32(len(pods)) < desired {
		return fmt.Sprintf("pod count %d below desired %d", len(pods), desired)
	}
	return ""
}

// selectNonParticipatingOutdated returns the highest-priority non-participating
// outdated pod for Step 2 (priority: Dead > Transient > Alive). Unknown-state
// pods are skipped: insufficient evidence to say they are non-participating.
func selectNonParticipatingOutdated(logger logr.Logger, outdated []corev1.Pod) (*corev1.Pod, string) {
	var dead, transient, alive []*corev1.Pod
	for i := range outdated {
		pod := &outdated[i]
		if isParticipating(pod) {
			continue
		}
		switch classifyEtcdContainer(pod) {
		case containerStateDead:
			dead = append(dead, pod)
		case containerStateTransient:
			transient = append(transient, pod)
		case containerStateAlive:
			alive = append(alive, pod)
		}
	}
	logger.V(1).Info("non-participating outdated pod classification",
		"dead", podNames(dead), "transient", podNames(transient), "alive", podNames(alive))

	if len(dead) > 0 {
		return dead[0], "Dead"
	}
	if len(transient) > 0 {
		return transient[0], "Transient"
	}
	if len(alive) > 0 {
		return alive[0], "Alive"
	}
	return nil, ""
}

// unreadyUpdatedPodNames returns the names of current-revision pods that are
// not participating in quorum.
func unreadyUpdatedPodNames(current []corev1.Pod) []string {
	var names []string
	for i := range current {
		if !isParticipating(&current[i]) {
			names = append(names, current[i].Name)
		}
	}
	return names
}

// terminatingPodNames returns the names of pods with DeletionTimestamp set.
func terminatingPodNames(pods []corev1.Pod) []string {
	var names []string
	for i := range pods {
		if pods[i].DeletionTimestamp != nil {
			names = append(names, pods[i].Name)
		}
	}
	return names
}

// selectParticipatingOutdated picks a Step 4 candidate, follower before leader.
// A lease-read failure is treated as an unknown role and the pod is classified
// as a follower.
func (r *Reconciler) selectParticipatingOutdated(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, outdated []corev1.Pod) (*corev1.Pod, string, error) {
	var followers, leaders []*corev1.Pod
	for i := range outdated {
		pod := &outdated[i]
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

// deleteSelected issues a Delete for the pod and waits until the local cache
// observes the resulting terminating state, so a rapid successive reconcile
// does not see stale pre-delete state.
func (r *Reconciler) deleteSelected(ctx context.Context, logger logr.Logger, pod *corev1.Pod) (ctrl.Result, error) {
	if pod.DeletionTimestamp != nil {
		logger.V(1).Info("selected pod is already terminating, requeuing", "pod", pod.Name)
		return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
	}
	if err := r.client.Delete(ctx, pod); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.V(1).Info("pod already deleted by another actor, requeuing", "pod", pod.Name)
			return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to delete pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	if err := r.waitForPodDeletionObserved(ctx, logger, pod); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeueAfterInFlight}, nil
}

// waitForPodDeletionObserved blocks until the local cache reflects the pod's
// deletion (DeletionTimestamp set or the pod gone) or the poll timeout expires.
// Timing out is non-fatal: the top gate on the next reconcile still holds if
// the cache is still stale.
func (r *Reconciler) waitForPodDeletionObserved(ctx context.Context, logger logr.Logger, pod *corev1.Pod) error {
	err := wait.PollUntilContextTimeout(ctx, podDeletionPollInterval, podDeletionPollTimeout, true,
		func(ctx context.Context) (bool, error) {
			observed := &corev1.Pod{}
			getErr := r.client.Get(ctx, client.ObjectKeyFromObject(pod), observed)
			if apierrors.IsNotFound(getErr) {
				return true, nil
			}
			if getErr != nil {
				return false, getErr
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
