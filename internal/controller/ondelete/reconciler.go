// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package ondelete implements the controller that drives quorum-aware pod updates
// for StatefulSets on the OnDelete update strategy. See DEP-07:https://github.com/gardener/etcd-druid/blob/master/docs/proposals/07-quorum-aware-pod-updates.md
package ondelete

import (
	"context"
	"fmt"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdclient "github.com/gardener/etcd-druid/internal/client/etcd"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler performs the OnDelete pod-update procedure on watched StatefulSets.
type Reconciler struct {
	client        client.Client
	config        druidconfigv1alpha1.OnDeleteControllerConfiguration
	logger        logr.Logger
	clientFactory etcdclient.Factory
}

// NewReconciler constructs a Reconciler for the OnDelete controller. The etcd
// client factory is required: the pod-update procedure validates every delete
// against live cluster membership which requires an etcd client.
func NewReconciler(mgr manager.Manager, config druidconfigv1alpha1.OnDeleteControllerConfiguration, clientFactory etcdclient.Factory) *Reconciler {
	if config.ConcurrentSyncs == nil {
		config.ConcurrentSyncs = ptr.To(druidconfigv1alpha1.DefaultOnDeleteControllerConcurrentSyncs)
	}
	return &Reconciler{
		client:        mgr.GetClient(),
		config:        config,
		logger:        log.Log.WithName(controllerName),
		clientFactory: clientFactory,
	}
}

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch

// Reconcile reconciles to perform the pod update procedure.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.WithValues("sts", req.NamespacedName, "runID", string(controller.ReconcileIDFromContext(ctx)))

	sts := &appsv1.StatefulSet{}
	if err := r.client.Get(ctx, req.NamespacedName, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get StatefulSet %s: %w", req.NamespacedName, err)
	}

	if sts.Spec.UpdateStrategy.Type != appsv1.OnDeleteStatefulSetStrategyType {
		return ctrl.Result{}, nil
	}
	if sts.Status.UpdateRevision == "" {
		return ctrl.Result{}, nil
	}

	etcd, err := r.getOwningEtcd(ctx, sts)
	if err != nil {
		return ctrl.Result{}, err
	}
	if etcd == nil || etcd.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	pods, err := kubernetes.ListStatefulSetPods(ctx, r.client, sts)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.executePodUpdateProcedure(ctx, logger, sts, etcd, pods)
}

// getOwningEtcd returns the Etcd controller-owner of sts
func (r *Reconciler) getOwningEtcd(ctx context.Context, sts *appsv1.StatefulSet) (*druidv1alpha1.Etcd, error) {
	etcdKind := druidv1alpha1.SchemeGroupVersion.WithKind("Etcd").Kind
	var ownerEtcdName string
	for _, ref := range sts.OwnerReferences {
		if ref.Kind == etcdKind && ref.APIVersion == druidv1alpha1.SchemeGroupVersion.String() {
			ownerEtcdName = ref.Name
			break
		}
	}
	if ownerEtcdName == "" {
		return nil, nil
	}
	etcd := &druidv1alpha1.Etcd{}
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: sts.Namespace, Name: ownerEtcdName}, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get owning Etcd %s/%s: %w", sts.Namespace, ownerEtcdName, err)
	}
	return etcd, nil
}
