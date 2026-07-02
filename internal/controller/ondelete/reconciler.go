// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package ondelete implements the controller that drives quorum-aware pod updates
// for StatefulSets on the OnDelete update strategy. See DEP-07.
package ondelete

import (
	"context"
	"fmt"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler drives the OnDelete pod-update procedure on watched StatefulSets.
// It is stateless: every invocation re-derives the rollout state from the cluster.
type Reconciler struct {
	client client.Client
	config druidconfigv1alpha1.OnDeleteControllerConfiguration
	logger logr.Logger
}

// NewReconciler constructs a Reconciler for the OnDelete controller. It defaults
// a nil ConcurrentSyncs so a hand-built OperatorConfiguration (unit test, or the
// window before `cd api && make generate` runs) does not nil-deref later.
func NewReconciler(mgr manager.Manager, config druidconfigv1alpha1.OnDeleteControllerConfiguration) *Reconciler {
	if config.ConcurrentSyncs == nil {
		config.ConcurrentSyncs = ptr.To(druidconfigv1alpha1.DefaultOnDeleteControllerConcurrentSyncs)
	}
	return &Reconciler{
		client: mgr.GetClient(),
		config: config,
		logger: log.Log.WithName(controllerName),
	}
}

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch

// Reconcile drives one iteration of DEP-07 Pod Update Procedure. At most one
// pod is deleted per invocation.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("statefulSet", req.NamespacedName)

	sts := &appsv1.StatefulSet{}
	if err := r.client.Get(ctx, req.NamespacedName, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get StatefulSet %s: %w", req.NamespacedName, err)
	}

	// Defence in depth against a stale-cache race with a strategy flip back to RollingUpdate.
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

	pods, err := r.listStsPods(ctx, sts)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.executePodUpdateProcedure(ctx, logger, sts, etcd, pods)
}

// getOwningEtcd returns the Etcd controller-owner of sts, or (nil, nil) if none.
func (r *Reconciler) getOwningEtcd(ctx context.Context, sts *appsv1.StatefulSet) (*druidv1alpha1.Etcd, error) {
	etcdKind := druidv1alpha1.SchemeGroupVersion.WithKind("Etcd").Kind
	var ownerName string
	for _, ref := range sts.OwnerReferences {
		if ref.Kind == etcdKind && ref.APIVersion == druidv1alpha1.SchemeGroupVersion.String() {
			ownerName = ref.Name
			break
		}
	}
	if ownerName == "" {
		return nil, nil
	}
	etcd := &druidv1alpha1.Etcd{}
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: sts.Namespace, Name: ownerName}, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get owning Etcd %s/%s: %w", sts.Namespace, ownerName, err)
	}
	return etcd, nil
}

// listStsPods lists the pods matched by sts.Spec.Selector. Label-based rather
// than owner-ref-based so a freshly-created pod without OwnerReference is observed.
func (r *Reconciler) listStsPods(ctx context.Context, sts *appsv1.StatefulSet) ([]corev1.Pod, error) {
	if sts.Spec.Selector == nil {
		return nil, fmt.Errorf("statefulSet %s/%s has nil spec.selector", sts.Namespace, sts.Name)
	}
	sel, err := metav1.LabelSelectorAsSelector(sts.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse statefulSet %s/%s spec.selector: %w", sts.Namespace, sts.Name, err)
	}
	if sel.Empty() {
		return nil, fmt.Errorf("statefulSet %s/%s spec.selector matched everything; refusing to list", sts.Namespace, sts.Name)
	}
	list := &corev1.PodList{}
	if err := r.client.List(ctx, list, &client.ListOptions{Namespace: sts.Namespace, LabelSelector: sel}); err != nil {
		return nil, fmt.Errorf("failed to list pods for statefulSet %s/%s: %w", sts.Namespace, sts.Name, err)
	}
	return list.Items, nil
}
