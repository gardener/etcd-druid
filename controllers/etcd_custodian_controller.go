// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"

	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"sigs.k8s.io/controller-runtime/pkg/source"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	controllersconfig "github.com/gardener/etcd-druid/controllers/config"
	"github.com/gardener/etcd-druid/pkg/health/status"
	druidmapper "github.com/gardener/etcd-druid/pkg/mapper"
	druidpredicates "github.com/gardener/etcd-druid/pkg/predicate"
	"github.com/gardener/etcd-druid/pkg/utils"
)

// EtcdCustodian reconciles status of Etcd object
type EtcdCustodian struct {
	client.Client
	Scheme     *runtime.Scheme
	logger     logr.Logger
	config     controllersconfig.EtcdCustodianController
	restConfig *rest.Config
}

// NewEtcdCustodian creates a new EtcdCustodian object
func NewEtcdCustodian(mgr manager.Manager, config controllersconfig.EtcdCustodianController) *EtcdCustodian {
	return &EtcdCustodian{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		logger:     log.Log.WithName("custodian-controller"),
		config:     config,
		restConfig: mgr.GetConfig(),
	}
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;update;patch

// Reconcile reconciles the etcd.
func (ec *EtcdCustodian) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ec.logger.Info("Custodian controller reconciliation started")
	etcd := &druidv1alpha1.Etcd{}
	if err := ec.Get(ctx, req.NamespacedName, etcd); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	logger := ec.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String())

	if etcd.Status.LastError != nil && *etcd.Status.LastError != "" {
		logger.Info(fmt.Sprintf("Requeue item because of last error: %v", *etcd.Status.LastError))
		return ctrl.Result{
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		logger.Error(err, "Error converting etcd selector to selector")
		return ctrl.Result{}, err
	}

	statusCheck := status.NewChecker(ec.Client, ec.config)
	if err := statusCheck.Check(ctx, logger, etcd); err != nil {
		logger.Error(err, "Error executing status checks")
		return ctrl.Result{}, err
	}

	refMgr := NewEtcdDruidRefManager(ec.Client, ec.Scheme, etcd, selector, etcdGVK, nil)

	stsList, err := refMgr.FetchStatefulSet(ctx, etcd)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Requeue if we found more than one or no StatefulSet.
	// The Etcd controller needs to decide what to do in such situations.
	if len(stsList.Items) != 1 {
		if err := ec.updateEtcdStatus(ctx, logger, etcd, nil); err != nil {
			logger.Error(err, "Error while updating ETCD status when no statefulset found")
		}
		return ctrl.Result{
			RequeueAfter: 5 * time.Second,
		}, nil
	}

	if err := ec.updateEtcdStatus(ctx, logger, etcd, &stsList.Items[0]); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: ec.config.SyncPeriod}, nil
}

func (ec *EtcdCustodian) updateEtcdStatus(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) error {
	logger.Info("Updating etcd status with statefulset information")
	var (
		conditions = etcd.Status.Conditions
		members    = etcd.Status.Members
	)

	return controllerutils.TryUpdateStatus(ctx, retry.DefaultBackoff, ec.Client, etcd, func() error {
		etcd.Status.Conditions = conditions
		etcd.Status.Members = members

		// Bootstrap is a special case which is handled by the etcd controller.
		if !inBootstrap(etcd) && len(members) != 0 {
			etcd.Status.ClusterSize = pointer.Int32Ptr(int32(len(members)))
		}

		if sts != nil {
			etcd.Status.Etcd = &druidv1alpha1.CrossVersionObjectReference{
				APIVersion: sts.APIVersion,
				Kind:       sts.Kind,
				Name:       sts.Name,
			}

			ready := utils.CheckStatefulSet(etcd.Spec.Replicas, sts) == nil

			// To be changed once we have multiple replicas.
			etcd.Status.CurrentReplicas = sts.Status.CurrentReplicas
			etcd.Status.ReadyReplicas = sts.Status.ReadyReplicas
			etcd.Status.UpdatedReplicas = sts.Status.UpdatedReplicas
			etcd.Status.Ready = &ready
			logger.Info(fmt.Sprintf("ETCD status updated for statefulset current replicas: %v, ready replicas: %v, updated replicas: %v", sts.Status.CurrentReplicas, sts.Status.ReadyReplicas, sts.Status.UpdatedReplicas))
			return nil
		}

		etcd.Status.CurrentReplicas = 0
		etcd.Status.ReadyReplicas = 0
		etcd.Status.UpdatedReplicas = 0

		etcd.Status.Ready = pointer.BoolPtr(false)
		return nil
	})
}

func inBootstrap(etcd *druidv1alpha1.Etcd) bool {
	if etcd.Status.ClusterSize == nil {
		return true
	}
	return len(etcd.Status.Members) == 0 ||
		(len(etcd.Status.Members) < int(etcd.Spec.Replicas) && etcd.Spec.Replicas == *etcd.Status.ClusterSize)
}

// SetupWithManager sets up manager with a new controller and ec as the reconcile.Reconciler
func (ec *EtcdCustodian) SetupWithManager(ctx context.Context, mgr ctrl.Manager, workers int, ignoreOperationAnnotation bool) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: workers,
	})

	return builder.
		For(
			&druidv1alpha1.Etcd{},
			ctrlbuilder.WithPredicates(druidpredicates.EtcdReconciliationFinished(ignoreOperationAnnotation))).
		Owns(&coordinationv1.Lease{}).
		Watches(
			&source.Kind{Type: &appsv1.StatefulSet{}},
			mapper.EnqueueRequestsFrom(druidmapper.StatefulSetToEtcd(ctx, mgr.GetClient()), mapper.UpdateWithNew),
			ctrlbuilder.WithPredicates(druidpredicates.StatefulSetStatusChange()),
		).
		Complete(ec)
}
