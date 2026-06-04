// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/health/status"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mutateEtcdStatusFn is a function which mutates the status of the passed etcd object
type mutateEtcdStatusFn func(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, logger logr.Logger) ctrlutils.ReconcileStepResult

func (r *Reconciler) reconcileStatus(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	sLog := r.logger.WithValues("etcd", client.ObjectKeyFromObject(etcd), "operation", "reconcileStatus").WithValues("runID", ctx.RunID)
	if !druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(etcd.ObjectMeta) {
		sLog.Info("Skipping status checks since etcd runtime component creation is disabled")
		return ctrlutils.ContinueReconcile()
	}
	originalEtcd := etcd.DeepCopy()

	var mutateETCDStatusStepFns = []mutateEtcdStatusFn{
		r.mutateETCDStatusWithMemberStatusAndConditions,
		r.inspectStatefulSetAndMutateETCDStatus,
		r.setSelector,
		r.mutateBootstrapWithExistingClusterStatus,
	}

	for _, fn := range mutateETCDStatusStepFns {
		if stepResult := fn(ctx, etcd, sLog); ctrlutils.ShortCircuitReconcileFlow(stepResult) {
			return stepResult
		}
	}
	if err := r.client.Status().Patch(ctx, etcd, client.MergeFrom(originalEtcd)); err != nil {
		sLog.Error(err, "failed to update etcd status")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) mutateETCDStatusWithMemberStatusAndConditions(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, logger logr.Logger) ctrlutils.ReconcileStepResult {
	statusCheck := status.NewChecker(r.client, r.config.EtcdMember.NotReadyThreshold.Duration, r.config.EtcdMember.UnknownThreshold.Duration)
	if err := statusCheck.Check(ctx, logger, etcd); err != nil {
		logger.Error(err, "Error executing status checks to update member status and conditions")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) inspectStatefulSetAndMutateETCDStatus(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, _ logr.Logger) ctrlutils.ReconcileStepResult {
	sts, err := kubernetes.GetStatefulSet(ctx, r.client, etcd)
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	if sts != nil {
		etcd.Status.Etcd = &druidv1alpha1.CrossVersionObjectReference{
			APIVersion: sts.APIVersion,
			Kind:       sts.Kind,
			Name:       sts.Name,
		}
		expectedReplicas := etcd.Spec.Replicas
		// if the latest Etcd spec has not yet been reconciled by druid, then check sts readiness against sts.spec.replicas instead
		if etcd.Status.ObservedGeneration == nil || *etcd.Status.ObservedGeneration != etcd.Generation {
			expectedReplicas = *sts.Spec.Replicas
		}
		ready, _ := kubernetes.IsStatefulSetReady(expectedReplicas, sts)
		etcd.Status.CurrentReplicas = sts.Status.CurrentReplicas
		etcd.Status.ReadyReplicas = sts.Status.ReadyReplicas
		etcd.Status.Replicas = sts.Status.CurrentReplicas
		etcd.Status.Ready = &ready
	} else {
		etcd.Status.CurrentReplicas = 0
		etcd.Status.ReadyReplicas = 0
		etcd.Status.Ready = ptr.To(false)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) setSelector(_ component.OperatorContext, etcd *druidv1alpha1.Etcd, _ logr.Logger) ctrlutils.ReconcileStepResult {
	labels := druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: labels})
	if err != nil {
		return ctrlutils.ReconcileWithError(err)
	}
	etcd.Status.Selector = ptr.To(selector.String())
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) mutateBootstrapWithExistingClusterStatus(_ component.OperatorContext, etcd *druidv1alpha1.Etcd, _ logr.Logger) ctrlutils.ReconcileStepResult {
	spec := etcd.Spec.Etcd.BootstrapWithExistingCluster
	if spec == nil || len(spec.Members) == 0 {
		return ctrlutils.ContinueReconcile()
	}

	bootstrapSucceeded := false
	for _, cond := range etcd.Status.Conditions {
		if cond.Type == druidv1alpha1.ConditionTypeBootstrapWithExistingCluster && cond.Status == druidv1alpha1.ConditionTrue {
			bootstrapSucceeded = true
			break
		}
	}
	if !bootstrapSucceeded {
		return ctrlutils.ContinueReconcile()
	}

	if len(etcd.Status.BootstrapWithExistingClusterMembers) == 0 {
		now := metav1.Now()
		joined := make([]druidv1alpha1.BootstrapJoinedMember, 0, len(spec.Members))
		for _, m := range spec.Members {
			joined = append(joined, druidv1alpha1.BootstrapJoinedMember{
				Name:     m.Name,
				PeerURLs: m.PeerURLs,
				JoinedAt: now,
			})
		}
		etcd.Status.BootstrapWithExistingClusterMembers = joined
		return ctrlutils.ContinueReconcile()
	}

	peerURLsByName := make(map[string][]string, len(spec.Members))
	for _, m := range spec.Members {
		peerURLsByName[m.Name] = m.PeerURLs
	}
	for i := range etcd.Status.BootstrapWithExistingClusterMembers {
		member := &etcd.Status.BootstrapWithExistingClusterMembers[i]
		if len(member.PeerURLs) == 0 {
			if peerURLs, ok := peerURLsByName[member.Name]; ok {
				member.PeerURLs = peerURLs
			}
		}
	}
	return ctrlutils.ContinueReconcile()
}
