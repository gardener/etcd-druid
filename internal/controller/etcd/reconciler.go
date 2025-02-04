// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	"github.com/gardener/etcd-druid/internal/component/clientservice"
	"github.com/gardener/etcd-druid/internal/component/configmap"
	"github.com/gardener/etcd-druid/internal/component/memberlease"
	"github.com/gardener/etcd-druid/internal/component/peerservice"
	"github.com/gardener/etcd-druid/internal/component/poddistruptionbudget"
	"github.com/gardener/etcd-druid/internal/component/role"
	"github.com/gardener/etcd-druid/internal/component/rolebinding"
	"github.com/gardener/etcd-druid/internal/component/serviceaccount"
	"github.com/gardener/etcd-druid/internal/component/snapshotlease"
	"github.com/gardener/etcd-druid/internal/component/statefulset"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/images"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler reconciles the Etcd resource spec and status.
type Reconciler struct {
	client            client.Client
	config            *Config
	recorder          record.EventRecorder
	imageVector       imagevector.ImageVector
	operatorRegistry  component.Registry
	lastOpErrRecorder ctrlutils.LastOperationAndLastErrorsRecorder
	logger            logr.Logger
}

// NewReconciler creates a new reconciler for Etcd.
func NewReconciler(mgr manager.Manager, config *Config, controllerName string) (*Reconciler, error) {
	imageVector, err := images.CreateImageVector()
	if err != nil {
		return nil, err
	}
	return NewReconcilerWithImageVector(mgr, controllerName, config, imageVector)
}

// NewReconcilerWithImageVector creates a new reconciler for Etcd with the given image vector.
func NewReconcilerWithImageVector(mgr manager.Manager, controllerName string, config *Config, iv imagevector.ImageVector) (*Reconciler, error) {
	logger := log.Log.WithName(controllerName)
	operatorReg := createAndInitializeOperatorRegistry(mgr.GetClient(), config, iv)
	lastOpErrRecorder := ctrlutils.NewLastOperationAndLastErrorsRecorder(mgr.GetClient(), logger)
	return &Reconciler{
		client:            mgr.GetClient(),
		config:            config,
		recorder:          mgr.GetEventRecorderFor(controllerName),
		imageVector:       iv,
		logger:            logger,
		operatorRegistry:  operatorReg,
		lastOpErrRecorder: lastOpErrRecorder,
	}, nil
}

type reconcileFn func(ctx component.OperatorContext, objectKey client.ObjectKey) ctrlutils.ReconcileStepResult

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;create;update;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts;services;configmaps,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;get;list

// Reconcile manages the reconciliation of the Etcd component to align it with its desired specifications.
//
// The reconciliation process involves the following steps:
//  1. Deletion Handling: If the Etcd component has a deletionTimestamp, initiate the deletion workflow.
//     On error, requeue the request.
//  2. Spec Reconciliation : Determine whether the Etcd spec should be reconciled based on annotations and flags,
//     and if there is a need then reconcile spec.
//  3. Status Reconciliation: Always update the status of the Etcd component to reflect its current state,
//     as well as status fields derived from spec reconciliation.
//  4. Scheduled Requeue: Requeue the reconciliation request after a defined period (EtcdStatusSyncPeriod) to maintain sync.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	runID := string(controller.ReconcileIDFromContext(ctx))
	operatorCtx := component.NewOperatorContext(ctx, r.logger, runID)
	if result := r.reconcileEtcdDeletion(operatorCtx, req.NamespacedName); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result.ReconcileResult()
	}

	reconcileSpecResult := r.reconcileSpec(operatorCtx, req.NamespacedName)

	reconcileStatusResult := r.reconcileStatus(operatorCtx, req.NamespacedName, reconcileSpecResult)
	if ctrlutils.ShortCircuitReconcileFlow(reconcileStatusResult) {
		r.logger.Error(reconcileStatusResult.GetCombinedError(), "Failed to reconcile status")
		return reconcileStatusResult.ReconcileResult()
	}

	// Operation annotation is removed at the end of reconciliation flow, after both spec and status
	// have been successfully reconciled. This ensures that if there are any errors in either spec or status
	// reconcile flow upon addition of operation annotation, then the next requeue of reconciliation will attempt
	// spec reconciliation again, ensuring that updation of spec result-related fields in the status
	// (such as observedGeneration) is not missed.
	if result := r.removeOperationAnnotation(operatorCtx, req.NamespacedName, reconcileSpecResult, reconcileStatusResult); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result.ReconcileResult()
	}

	if reconcileSpecResult.NeedsRequeue() {
		return reconcileSpecResult.ReconcileResult()
	}
	return ctrlutils.ReconcileAfter(r.config.EtcdStatusSyncPeriod, "Periodic Requeue").ReconcileResult()
}

// GetOperatorRegistry returns the component registry.
func (r *Reconciler) GetOperatorRegistry() component.Registry {
	return r.operatorRegistry
}

func createAndInitializeOperatorRegistry(client client.Client, config *Config, imageVector imagevector.ImageVector) component.Registry {
	reg := component.NewRegistry()
	reg.Register(component.ConfigMapKind, configmap.New(client))
	reg.Register(component.ServiceAccountKind, serviceaccount.New(client, config.DisableEtcdServiceAccountAutomount))
	reg.Register(component.MemberLeaseKind, memberlease.New(client))
	reg.Register(component.SnapshotLeaseKind, snapshotlease.New(client))
	reg.Register(component.ClientServiceKind, clientservice.New(client))
	reg.Register(component.PeerServiceKind, peerservice.New(client))
	reg.Register(component.PodDisruptionBudgetKind, poddistruptionbudget.New(client))
	reg.Register(component.RoleKind, role.New(client))
	reg.Register(component.RoleBindingKind, rolebinding.New(client))
	reg.Register(component.StatefulSetKind, statefulset.New(client, imageVector))
	return reg
}

func (r *Reconciler) reconcileEtcdDeletion(ctx component.OperatorContext, etcdObjectKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcdPartialObjMetadata := ctrlutils.EmptyEtcdPartialObjectMetadata()
	if result := ctrlutils.GetLatestEtcdPartialObjectMeta(ctx, r.client, etcdObjectKey, etcdPartialObjMetadata); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if druidv1alpha1.IsEtcdMarkedForDeletion(etcdPartialObjMetadata.ObjectMeta) {
		dLog := r.logger.WithValues("etcd", etcdObjectKey, "operation", "delete").WithValues("runId", ctx.RunID)
		ctx.SetLogger(dLog)
		return r.triggerDeletionFlow(ctx, dLog, etcdObjectKey)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) reconcileSpec(ctx component.OperatorContext, etcdObjectKey client.ObjectKey) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := ctrlutils.GetLatestEtcd(ctx, r.client, etcdObjectKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if r.canReconcileSpec(etcd) {
		rLog := r.logger.WithValues("etcd", etcdObjectKey, "operation", "reconcileSpec").WithValues("runID", ctx.RunID)
		ctx.SetLogger(rLog)
		return r.triggerReconcileSpecFlow(ctx, etcdObjectKey)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) removeOperationAnnotation(ctx component.OperatorContext, etcdObjKey client.ObjectKey, reconcileSpecResult, reconcileStatusResult ctrlutils.ReconcileStepResult) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := ctrlutils.GetLatestEtcd(ctx, r.client, etcdObjKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}

	if !r.canReconcileSpec(etcd) ||
		ctrlutils.ShortCircuitReconcileFlow(reconcileSpecResult) ||
		ctrlutils.ShortCircuitReconcileFlow(reconcileStatusResult) {
		return ctrlutils.ContinueReconcile()
	}

	if metav1.HasAnnotation(etcd.ObjectMeta, v1beta1constants.GardenerOperation) {
		ctx.Logger.Info("Removing operation annotation")
		withOpAnnotation := etcd.DeepCopy()
		delete(etcd.Annotations, v1beta1constants.GardenerOperation)
		if err := r.client.Patch(ctx, etcd, client.MergeFrom(withOpAnnotation)); err != nil {
			ctx.Logger.Error(err, "failed to remove operation annotation")
			return ctrlutils.ReconcileWithError(err)
		}
	}
	return ctrlutils.ContinueReconcile()
}
