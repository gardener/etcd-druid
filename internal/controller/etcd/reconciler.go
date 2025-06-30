// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
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
	"github.com/gardener/etcd-druid/internal/utils/imagevector"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// ControllerName is the name of the etcd controller.
const ControllerName = "etcd-controller"

// Reconciler reconciles the Etcd resource spec and status.
type Reconciler struct {
	client            client.Client
	config            druidconfigv1alpha1.EtcdControllerConfiguration
	recorder          record.EventRecorder
	imageVector       imagevector.ImageVector
	operatorRegistry  component.Registry
	lastOpErrRecorder ctrlutils.LastOperationAndLastErrorsRecorder
	logger            logr.Logger
}

// NewReconciler creates a new reconciler for Etcd.
func NewReconciler(mgr manager.Manager, config druidconfigv1alpha1.EtcdControllerConfiguration) (*Reconciler, error) {
	imageVector, err := images.CreateImageVector()
	if err != nil {
		return nil, err
	}
	return NewReconcilerWithImageVector(mgr, ControllerName, config, imageVector)
}

// NewReconcilerWithImageVector creates a new reconciler for Etcd with the given image vector.
func NewReconcilerWithImageVector(mgr manager.Manager, controllerName string, config druidconfigv1alpha1.EtcdControllerConfiguration, iv imagevector.ImageVector) (*Reconciler, error) {
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
//  4. Remove operation-reconcile annotation if it was set and if spec reconciliation had succeeded.
//  5. Scheduled Requeue: Requeue the reconciliation request after a defined period (EtcdStatusSyncPeriod) to maintain sync.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	runID := string(controller.ReconcileIDFromContext(ctx))
	operatorCtx := component.NewOperatorContext(ctx, r.logger, runID)
	if result := r.reconcileEtcdDeletion(operatorCtx, req.NamespacedName); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result.ReconcileResult()
	}

	etcd := &druidv1alpha1.Etcd{}
	if result := ctrlutils.GetLatestEtcd(ctx, r.client, req.NamespacedName, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result.ReconcileResult()
	}
	shouldReconcileSpec := r.shouldReconcileSpec(etcd)

	var reconcileSpecResult ctrlutils.ReconcileStepResult
	if shouldReconcileSpec {
		reconcileSpecResult = r.reconcileSpec(operatorCtx, req.NamespacedName)
	}

	if result := r.reconcileStatus(operatorCtx, req.NamespacedName); ctrlutils.ShortCircuitReconcileFlow(result) {
		r.logger.Error(result.GetCombinedError(), "Failed to reconcile status")
		return result.ReconcileResult()
	}

	if reconcileSpecResult.NeedsRequeue() {
		return reconcileSpecResult.ReconcileResult()
	}

	// Spec reconciliation involves some steps that must be executed after status reconciliation,
	// to ensure consistency of the status and to ensure that any intermittent failures result in a
	// requeue to re-attempt the spec reconciliation.
	// Specifically, status.observedGeneration must be updated after the rest of the status fields are updated,
	// because consumers of the Etcd status must check the observed generation to confirm that reconciliation is
	// in fact complete, and the status fields reflect the latest possible state of the etcd cluster after the
	// spec was reconciled.
	// Additionally, the operation annotation needs to be removed only at the end of reconciliation, to ensure that
	// if any failure is encountered during reconciliation, then reconciliation is re-attempted upon the next requeue.
	// r.completeReconcile() is executed only if the spec was reconciled, as denoted by the `shouldReconcileSpec` flag.
	if shouldReconcileSpec {
		if result := r.completeReconcile(operatorCtx, req.NamespacedName); ctrlutils.ShortCircuitReconcileFlow(result) {
			return result.ReconcileResult()
		}
	}

	return ctrlutils.ReconcileAfter(r.config.EtcdStatusSyncPeriod.Duration, "Periodic Requeue").ReconcileResult()
}

// GetOperatorRegistry returns the component registry.
func (r *Reconciler) GetOperatorRegistry() component.Registry {
	return r.operatorRegistry
}

func createAndInitializeOperatorRegistry(client client.Client, config druidconfigv1alpha1.EtcdControllerConfiguration, imageVector imagevector.ImageVector) component.Registry {
	reg := component.NewRegistry()
	reg.Register(component.ServiceAccountKind, serviceaccount.New(client, config.DisableEtcdServiceAccountAutomount))
	reg.Register(component.RoleKind, role.New(client))
	reg.Register(component.RoleBindingKind, rolebinding.New(client))
	reg.Register(component.MemberLeaseKind, memberlease.New(client))
	reg.Register(component.SnapshotLeaseKind, snapshotlease.New(client))
	reg.Register(component.PodDisruptionBudgetKind, poddistruptionbudget.New(client))
	reg.Register(component.ClientServiceKind, clientservice.New(client))
	reg.Register(component.PeerServiceKind, peerservice.New(client))
	reg.Register(component.ConfigMapKind, configmap.New(client))
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
