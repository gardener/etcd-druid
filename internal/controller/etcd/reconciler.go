// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package etcd

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/utils"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/operator"
	"github.com/gardener/etcd-druid/internal/operator/clientservice"
	"github.com/gardener/etcd-druid/internal/operator/configmap"
	"github.com/gardener/etcd-druid/internal/operator/memberlease"
	"github.com/gardener/etcd-druid/internal/operator/peerservice"
	"github.com/gardener/etcd-druid/internal/operator/poddistruptionbudget"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/operator/role"
	"github.com/gardener/etcd-druid/internal/operator/rolebinding"
	"github.com/gardener/etcd-druid/internal/operator/serviceaccount"
	"github.com/gardener/etcd-druid/internal/operator/snapshotlease"
	"github.com/gardener/etcd-druid/internal/operator/statefulset"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Reconciler struct {
	client            client.Client
	config            *Config
	recorder          record.EventRecorder
	imageVector       imagevector.ImageVector
	operatorRegistry  operator.Registry
	lastOpErrRecorder ctrlutils.LastOperationErrorRecorder
	logger            logr.Logger
}

// NewReconciler creates a new reconciler for Etcd.
func NewReconciler(mgr manager.Manager, config *Config) (*Reconciler, error) {
	imageVector, err := ctrlutils.CreateImageVector()
	logger := log.Log.WithName(controllerName)
	if err != nil {
		return nil, err
	}
	operatorReg := createAndInitializeOperatorRegistry(mgr.GetClient(), logger, config, imageVector)
	lastOpErrRecorder := ctrlutils.NewLastOperationErrorRecorder(mgr.GetClient(), logger)
	return &Reconciler{
		client:            mgr.GetClient(),
		config:            config,
		recorder:          mgr.GetEventRecorderFor(controllerName),
		imageVector:       imageVector,
		logger:            logger,
		operatorRegistry:  operatorReg,
		lastOpErrRecorder: lastOpErrRecorder,
	}, nil
}

func createAndInitializeOperatorRegistry(client client.Client, logger logr.Logger, config *Config, imageVector imagevector.ImageVector) operator.Registry {
	reg := operator.NewRegistry()
	reg.Register(operator.ConfigMapKind, configmap.New(client, logger))
	reg.Register(operator.ServiceAccountKind, serviceaccount.New(client, logger, config.DisableEtcdServiceAccountAutomount))
	reg.Register(operator.MemberLeaseKind, memberlease.New(client, logger))
	reg.Register(operator.SnapshotLeaseKind, snapshotlease.New(client, logger))
	reg.Register(operator.ClientServiceKind, clientservice.New(client, logger))
	reg.Register(operator.PeerServiceKind, peerservice.New(client, logger))
	reg.Register(operator.PodDisruptionBudgetKind, poddistruptionbudget.New(client, logger))
	reg.Register(operator.RoleKind, role.New(client, logger))
	reg.Register(operator.RoleBindingKind, rolebinding.New(client, logger))
	reg.Register(operator.StatefulSetKind, statefulset.New(client, logger, imageVector, config.FeatureGates))
	return reg
}

/*
	If deletionTimestamp set:
		triggerDeletionFlow(); if err then requeue
	Else:
		If ignore-reconciliation is set to true:
			skip reconcileSpec()
		Else If IgnoreOperationAnnotation flag is true:
			always reconcileSpec()
		Else If IgnoreOperationAnnotation flag is false and reconcile-op annotation is present:
			reconcileSpec()
			if err in getting etcd, return with requeue
			if err in deploying any of the components, then record pending requeue
	reconcileStatus()
	requeue after minimum of X seconds (EtcdStatusSyncPeriod) and previously recorded requeue request
*/

// TODO: where/how is this being used?
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;create;update;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update;patch;triggerDeletionFlow
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;create;update;patch;triggerDeletionFlow
// +kubebuilder:rbac:groups="",resources=serviceaccounts;services;configmaps,verbs=get;list;create;update;patch;triggerDeletionFlow
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;create;update;patch;triggerDeletionFlow
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;create;update;patch;triggerDeletionFlow
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;create;update;patch;triggerDeletionFlow
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;get;list

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	type reconcileFn func(ctx context.Context, objectKey client.ObjectKey, runID string) ctrlutils.ReconcileStepResult
	reconcileFns := []reconcileFn{
		r.reconcileEtcdDeletion,
		r.reconcileSpec,
		r.reconcileStatus,
	}
	runID := uuid.New().String()
	for _, fn := range reconcileFns {
		if result := fn(ctx, req.NamespacedName, runID); ctrlutils.ShortCircuitReconcileFlow(result) {
			return result.ReconcileResult()
		}
	}
	return ctrlutils.DoNotRequeue().ReconcileResult()
}

func (r *Reconciler) reconcileEtcdDeletion(ctx context.Context, etcdObjectKey client.ObjectKey, runID string) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjectKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if etcd.IsMarkedForDeletion() {
		operatorCtx := resource.NewOperatorContext(ctx, r.logger, runID)
		dLog := r.logger.WithValues("etcd", etcdObjectKey, "operation", "delete")
		return r.triggerDeletionFlow(operatorCtx, dLog, etcdObjectKey)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) reconcileSpec(ctx context.Context, etcdObjectKey client.ObjectKey, runID string) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjectKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}

	operatorCtx := resource.NewOperatorContext(ctx, r.logger, runID)
	resourceOperators := r.getOrderedOperatorsForSync()
	for _, kind := range resourceOperators {
		op := r.operatorRegistry.GetOperator(kind)
		if err := op.Sync(operatorCtx, etcd); err != nil {
			return utils.ReconcileWithError(err)
		}
	}
	return utils.ContinueReconcile()
}

func (r *Reconciler) reconcileStatus(ctx context.Context, etcdNamespacedName types.NamespacedName, runID string) ctrlutils.ReconcileStepResult {
	/*
		fetch EtcdMember resources
		fetch member leases
		status.condition checks
		status.members checks
		fetch latest Etcd resource
		update etcd status
	*/

	return ctrlutils.DoNotRequeue()
}

func (r *Reconciler) getLatestEtcd(ctx context.Context, objectKey client.ObjectKey, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	if err := r.client.Get(ctx, objectKey, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlutils.DoNotRequeue()
		}
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

//func (r *Reconciler) runPreSpecReconcileChecks(etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
//	suspendEtcdSpecReconcileAnnotationKey := getSuspendEtcdReconcileAnnotationKey(etcd)
//	if suspendEtcdSpecReconcileAnnotationKey != nil {
//		r.recorder.Eventf(
//			etcd,
//			corev1.EventTypeWarning,
//			"SpecReconciliationSkipped",
//			"spec reconciliation of %s/%s is skipped by etcd-druid due to the presence of annotation %s on the etcd resource",
//			etcd.Namespace,
//			etcd.Name,
//			suspendEtcdSpecReconcileAnnotationKey,
//		)
//
//	}
//}

//func (r *Reconciler) checkAndHandleReconcileSuspension(etcd *druidv1alpha1.Etcd) bool {
//	//TODO: Once no one uses IgnoreReconciliationAnnotation annotation, then we can simplify this code.
//	var annotationKey string
//	if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation) {
//		annotationKey = druidv1alpha1.SuspendEtcdSpecReconcileAnnotation
//	} else if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.IgnoreReconciliationAnnotation) {
//		annotationKey = druidv1alpha1.IgnoreReconciliationAnnotation
//	}
//	if len(annotationKey) > 0 {
//		r.recorder.Eventf(
//			etcd,
//			corev1.EventTypeWarning,
//			"SpecReconciliationSkipped",
//			"spec reconciliation of %s/%s is skipped by etcd-druid due to the presence of annotation %s on the etcd resource",
//			etcd.Namespace,
//			etcd.Name,
//			annotationKey,
//		)
//		return true
//	}
//	return ctrlutils.ContinueReconcile()
//}
