// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/health/status"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mutateEtcdStatusFn is a function which mutates the status of the passed etcd object
type mutateEtcdStatusFn func(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, logger logr.Logger) ctrlutils.ReconcileStepResult

func (r *Reconciler) reconcileStatus(ctx component.OperatorContext, etcdObjectKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := ctrlutils.GetLatestEtcd(ctx, r.client, etcdObjectKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	sLog := r.logger.WithValues("etcd", etcdObjectKey, "operation", "reconcileStatus").WithValues("runID", ctx.RunID)
	originalEtcd := etcd.DeepCopy()
	mutateETCDStatusStepFns := []mutateEtcdStatusFn{
		r.mutateETCDStatusWithMemberStatusAndConditions,
		r.inspectStatefulSetAndMutateETCDStatus,
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
	statusCheck := status.NewChecker(r.client, r.config.EtcdMember.NotReadyThreshold, r.config.EtcdMember.UnknownThreshold)
	if err := statusCheck.Check(ctx, logger, etcd); err != nil {
		logger.Error(err, "Error executing status checks to update member status and conditions")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) inspectStatefulSetAndMutateETCDStatus(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, _ logr.Logger) ctrlutils.ReconcileStepResult {
	sts, err := utils.GetStatefulSet(ctx, r.client, etcd)
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
		ready, _ := utils.IsStatefulSetReady(expectedReplicas, sts)
		etcd.Status.CurrentReplicas = sts.Status.CurrentReplicas
		etcd.Status.ReadyReplicas = sts.Status.ReadyReplicas
		etcd.Status.UpdatedReplicas = sts.Status.UpdatedReplicas
		etcd.Status.Replicas = sts.Status.CurrentReplicas
		etcd.Status.Ready = &ready
	} else {
		etcd.Status.CurrentReplicas = 0
		etcd.Status.ReadyReplicas = 0
		etcd.Status.UpdatedReplicas = 0
		etcd.Status.Ready = ptr.To(false)
	}
	return ctrlutils.ContinueReconcile()
}
