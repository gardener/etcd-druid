// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// executePodUpdateProcedure runs DEP-07 Pod Update Procedure Steps 1-4 and
// issues at most one Delete per invocation. Step 5 (requeue) is achieved by
// returning Result{Requeue: true}, driving controller-runtime's exponential
// backoff.
func (r *Reconciler) executePodUpdateProcedure(
	ctx context.Context,
	logger logr.Logger,
	sts *appsv1.StatefulSet,
	etcd *druidv1alpha1.Etcd,
	pods []corev1.Pod,
) (ctrl.Result, error) {
	outdated, current := partitionByRevision(pods, sts)

	// Step 1: nothing outdated -> rollout complete.
	if len(outdated) == 0 {
		return ctrl.Result{}, nil
	}
	logger.Info("OnDelete rollout in progress",
		"updateRevision", sts.Status.UpdateRevision,
		"outdated", len(outdated), "current", len(current), "totalPods", len(pods),
	)

	// Step 2: non-participating outdated pods first (safe: they contribute nothing to quorum).
	if pod := selectNonParticipatingOutdated(outdated); pod != nil {
		return r.deleteSelected(ctx, logger, pod, "Step2:non-participating-outdated")
	}

	// Step 3: wait for every previously-updated pod to rejoin quorum. Terminating
	// pods are excluded from `current` so we do not stall on a pod that will never
	// become ready. Never skip past a stuck pod even in >3-replica clusters where
	// quorum headroom would technically allow it — that's DEP-07 Future Scope.
	if pod := firstNonParticipatingCurrent(current); pod != nil {
		logger.Info("Waiting for previously-updated pod to rejoin quorum", "waitingOn", pod.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	// Step 4: pick one participating outdated pod, follower before leader.
	if pod, err := r.selectParticipatingOutdated(ctx, etcd, outdated); err != nil {
		return ctrl.Result{}, err
	} else if pod != nil {
		return r.deleteSelected(ctx, logger, pod, "Step4:participating-outdated")
	}

	logger.Info("No pod selected for deletion despite outdated pods existing; requeuing",
		"outdatedCount", len(outdated))
	return ctrl.Result{Requeue: true}, nil
}

// partitionByRevision splits pods into outdated/current, excluding terminating
// pods from both so Step 3 never waits on a pod that is being replaced.
func partitionByRevision(pods []corev1.Pod, sts *appsv1.StatefulSet) (outdated, current []corev1.Pod) {
	for i := range pods {
		pod := pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if isPodOutdated(&pod, sts) {
			outdated = append(outdated, pod)
		} else {
			current = append(current, pod)
		}
	}
	return outdated, current
}

// selectNonParticipatingOutdated returns the highest-priority non-participating
// outdated pod for Step 2, using sub-priority Dead > Transient > Alive. Unknown-
// state pods are skipped: insufficient evidence to say they are non-participating.
func selectNonParticipatingOutdated(outdated []corev1.Pod) *corev1.Pod {
	var dead, transient, alive *corev1.Pod
	for i := range outdated {
		pod := &outdated[i]
		if isParticipating(pod) {
			continue
		}
		switch classifyEtcdContainer(pod) {
		case containerStateDead:
			if dead == nil {
				dead = pod
			}
		case containerStateTransient:
			if transient == nil {
				transient = pod
			}
		case containerStateAlive:
			if alive == nil {
				alive = pod
			}
		}
	}
	switch {
	case dead != nil:
		return dead
	case transient != nil:
		return transient
	case alive != nil:
		return alive
	default:
		return nil
	}
}

func firstNonParticipatingCurrent(current []corev1.Pod) *corev1.Pod {
	for i := range current {
		if !isParticipating(&current[i]) {
			return &current[i]
		}
	}
	return nil
}

// selectParticipatingOutdated picks a Step-4 candidate, preferring a follower.
// Lease read failures fall through to follower ordering (see memberRole doc).
func (r *Reconciler) selectParticipatingOutdated(ctx context.Context, etcd *druidv1alpha1.Etcd, outdated []corev1.Pod) (*corev1.Pod, error) {
	if len(outdated) == 0 {
		return nil, nil
	}
	if etcd.Spec.Replicas == 1 {
		return &outdated[0], nil
	}

	var leader, follower *corev1.Pod
	for i := range outdated {
		pod := &outdated[i]
		role, err := r.memberRole(ctx, etcd, pod)
		if err != nil {
			r.logger.V(1).Info("Falling back to follower ordering after role lookup failure",
				"pod", pod.Name, "err", err.Error())
		}
		if isLeader(role) {
			if leader == nil {
				leader = pod
			}
			continue
		}
		if follower == nil {
			follower = pod
		}
	}
	if follower != nil {
		return follower, nil
	}
	return leader, nil
}

// deleteSelected issues a normal-grace-period Delete against the pod resource
// (not pods/eviction — see DEP-07 Pod Deletion Method) and enforces the
// deletionTimestamp guard to avoid double-delete.
func (r *Reconciler) deleteSelected(ctx context.Context, logger logr.Logger, pod *corev1.Pod, step string) (ctrl.Result, error) {
	if pod.DeletionTimestamp != nil {
		return ctrl.Result{Requeue: true}, nil
	}
	logger.Info("Deleting outdated pod", "pod", pod.Name, "step", step)
	if err := r.client.Delete(ctx, pod); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to delete pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	return ctrl.Result{Requeue: true}, nil
}
