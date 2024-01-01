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
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/utils"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/health/status"
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
	pkgutils "github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
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

type reconcileFn func(ctx context.Context, objectKey client.ObjectKey, runID string) ctrlutils.ReconcileStepResult

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

// Reconcile manages the reconciliation of the Etcd resource to align it with its desired specifications.
//
// The reconciliation process involves the following steps:
//  1. Deletion Handling: If the Etcd resource has a deletionTimestamp, initiate the deletion workflow. On error, requeue the request.
//  2. Reconciliation Decision: Determine whether the Etcd spec should be reconciled based on annotations and flags.
//  3. Status Reconciliation: Always update the status of the Etcd resource to reflect its current state.
//  4. Scheduled Requeue: Requeue the reconciliation request after a defined period (EtcdStatusSyncPeriod) to maintain sync.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, req.NamespacedName, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result.ReconcileResult()
	}

	var reconcileFns []reconcileFn
	// If the etcd resource is marked for deletion.
	if etcd.DeletionTimestamp != nil {
		reconcileFns = append(reconcileFns, r.reconcileEtcdDeletion)
	} else {
		// Check if we can reconcile spec.
		if r.canReconcileSpec(etcd) {
			reconcileFns = append(reconcileFns, r.reconcileSpec)
		}
	}

	// Always reconcile status.
	reconcileFns = append(reconcileFns, r.reconcileStatus)

	// Execute the reconcile functions.
	runID := uuid.New().String()
	for _, fn := range reconcileFns {
		if result := fn(ctx, req.NamespacedName, runID); ctrlutils.ShortCircuitReconcileFlow(result) {
			return result.ReconcileResult()
		}
	}
	return ctrlutils.ReconcileAfter(r.config.EtcdStatusSyncPeriod, "Reqeue").ReconcileResult()
}

func (r *Reconciler) reconcileEtcdDeletion(ctx context.Context, etcdObjectKey client.ObjectKey, runID string) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjectKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}
	if etcd.IsMarkedForDeletion() {
		operatorCtx := resource.NewOperatorContext(ctx, r.logger, runID)
		dLog := r.logger.WithValues("etcd", etcdObjectKey, "operation", "delete").WithValues("Reconciler", runID)
		return r.triggerDeletionFlow(operatorCtx, dLog, etcdObjectKey)
	}
	return ctrlutils.ContinueReconcile()
}

func (r *Reconciler) reconcileSpec(ctx context.Context, etcdObjectKey client.ObjectKey, runID string) ctrlutils.ReconcileStepResult {
	operatorCtx := resource.NewOperatorContext(ctx, r.logger, runID)
	rLog := r.logger.WithValues("etcd", etcdObjectKey, "operation", "reconcileSpec").WithValues("Reconciler", runID)
	return r.triggerReconcileSpec(operatorCtx, rLog, etcdObjectKey)
}

func (r *Reconciler) reconcileStatus(ctx context.Context, etcdObjectKey client.ObjectKey, runID string) ctrlutils.ReconcileStepResult {
	etcd := &druidv1alpha1.Etcd{}
	if result := r.getLatestEtcd(ctx, etcdObjectKey, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
		return result
	}

	statusCheck := status.NewChecker(r.client, 5*time.Minute, 1*time.Minute)
	if err := statusCheck.Check(ctx, r.logger, etcd); err != nil {
		r.logger.Error(err, "Error executing status checks")
		return utils.ReconcileWithError(err)
	}

	r.logger.Info("Updating etcd status with statefulset information", "namespace", etcd.Namespace, "name", etcd.Name)

	sts, err := pkgutils.GetStatefulSet(ctx, r.client, etcd)
	if err != nil {
		return utils.ReconcileWithError(err)
	}

	updateEtcdStatus := func() error {
		latestetcd := etcd.DeepCopy()
		if err := r.client.Get(ctx, etcd.GetNamespaceName(), latestetcd); err != nil {
			return err
		}
		latestetcd.Status = etcd.Status
		if sts != nil {
			latestetcd.Status.Etcd = &druidv1alpha1.CrossVersionObjectReference{
				APIVersion: sts.APIVersion,
				Kind:       sts.Kind,
				Name:       sts.Name,
			}
			ready, _ := pkgutils.IsStatefulSetReady(etcd.Spec.Replicas, sts)
			latestetcd.Status.CurrentReplicas = sts.Status.CurrentReplicas
			latestetcd.Status.ReadyReplicas = sts.Status.ReadyReplicas
			latestetcd.Status.UpdatedReplicas = sts.Status.UpdatedReplicas
			latestetcd.Status.Replicas = sts.Status.CurrentReplicas
			latestetcd.Status.Ready = &ready
		} else {
			latestetcd.Status.CurrentReplicas = 0
			latestetcd.Status.ReadyReplicas = 0
			latestetcd.Status.UpdatedReplicas = 0
			latestetcd.Status.Ready = pointer.Bool(false)
		}

		return r.client.Status().Update(ctx, latestetcd)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, updateEtcdStatus)
	if err != nil {
		r.logger.Error(err, "Failed to update etcd status")
		return utils.ReconcileWithError(err)
	}

	return utils.ContinueReconcile()
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
