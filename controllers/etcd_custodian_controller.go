// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// EtcdCustodian reconciles status of Etcd object
type EtcdCustodian struct {
	client.Client
	Scheme *runtime.Scheme
}

// NewEtcdCustodian creates a new EtcdCustodian object
func NewEtcdCustodian(mgr manager.Manager) *EtcdCustodian {
	return &EtcdCustodian{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;update;patch

// Reconcile reconciles the etcd.
func (ec *EtcdCustodian) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger.Info("Custodian controller reconciliation started")
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

	if etcd.Status.LastError != nil {
		if *etcd.Status.LastError != "" {
			return ctrl.Result{
				RequeueAfter: time.Duration(30 * time.Second),
			}, fmt.Errorf(*etcd.Status.LastError)
		}
	}

	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Machines (see #42639).
	canAdoptFunc := RecheckDeletionTimestamp(func() (metav1.Object, error) {
		foundEtcd := &druidv1alpha1.Etcd{}
		err := ec.Get(ctx, types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, foundEtcd)
		if err != nil {
			return nil, err
		}

		if foundEtcd.GetDeletionTimestamp() != nil {
			return nil, fmt.Errorf("%v/%v etcd is marked for deletion", etcd.Namespace, etcd.Name)
		}
		if foundEtcd.UID != etcd.UID {
			return nil, fmt.Errorf("original %v/%v etcd gone: got uid %v, wanted %v", etcd.Namespace, etcd.Name, foundEtcd.UID, etcd.UID)
		}
		return foundEtcd, nil
	})

	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		logger.Error(err, "Error converting etcd selector to selector")
		return ctrl.Result{}, err
	}

	cm := NewEtcdDruidRefManager(ec.Client, ec.Scheme, etcd, selector, etcdGVK, canAdoptFunc)

	ss, err := cm.FetchStatefulSet(etcd)
	if err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	// If no statefulsets could be fetched, requeue for reconcilation
	if len(ss) < 1 {
		ec.updateEtcdStatusWithNoSts(ctx, etcd)
		return ctrl.Result{
			RequeueAfter: time.Duration(5 * time.Second),
		}, nil
	}

	if err := ec.updateEtcdStatus(ctx, etcd, ss[0]); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	return ctrl.Result{
		Requeue: false,
	}, nil
}

func (ec *EtcdCustodian) updateEtcdStatus(ctx context.Context, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) error {
	logger.Infof("Reconciling etcd status in Custodian Controller for etcd statefulset status:%s in namespace:%s", etcd.Name, etcd.Namespace)

	return kutil.TryUpdateStatus(ctx, retry.DefaultBackoff, ec.Client, etcd, func() error {
		etcd.Status.Etcd = druidv1alpha1.CrossVersionObjectReference{
			APIVersion: sts.APIVersion,
			Kind:       sts.Kind,
			Name:       sts.Name,
		}
		ready := CheckStatefulSet(etcd, sts) == nil
		conditions := []druidv1alpha1.Condition{}
		for _, condition := range sts.Status.Conditions {
			conditions = append(conditions, convertConditionsToEtcd(&condition))
		}
		etcd.Status.Conditions = conditions

		// To be changed once we have multiple replicas.
		etcd.Status.CurrentReplicas = sts.Status.CurrentReplicas
		etcd.Status.ReadyReplicas = sts.Status.ReadyReplicas
		etcd.Status.UpdatedReplicas = sts.Status.UpdatedReplicas
		etcd.Status.Ready = &ready
		logger.Infof("ETCD status updated for statefulset current replicas: %v, ready replicas: %v, updated replicas: %v", sts.Status.CurrentReplicas, sts.Status.ReadyReplicas, sts.Status.UpdatedReplicas)
		return nil
	})
}

func (ec *EtcdCustodian) updateEtcdStatusWithNoSts(ctx context.Context, etcd *druidv1alpha1.Etcd) {
	logger.Infof("Reconciling etcd status in Custodian Controller when no statefulset found:%s in namespace:%s", etcd.Name, etcd.Namespace)

	err := kutil.TryUpdateStatus(ctx, retry.DefaultBackoff, ec.Client, etcd, func() error {
		conditions := []druidv1alpha1.Condition{}
		etcd.Status.Conditions = conditions

		// To be changed once we have multiple replicas.
		etcd.Status.CurrentReplicas = 0
		etcd.Status.ReadyReplicas = 0
		etcd.Status.UpdatedReplicas = 0

		ready := false
		etcd.Status.Ready = &ready
		return nil
	})

	if err != nil {
		logger.Errorf("Error while updating ETCD status for no statefulset in custodian controller: %v", err)
	}
}

// SetupWithManager sets up manager with a new controller and ec as the reconcile.Reconciler
func (ec *EtcdCustodian) SetupWithManager(mgr ctrl.Manager, workers int) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: workers,
	})

	return builder.For(&appsv1.StatefulSet{}).Owns(&druidv1alpha1.Etcd{}).Complete(ec)
}
