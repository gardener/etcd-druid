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

package custodian

import (
	"context"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/gardener/etcd-druid/pkg/health/status"
	"github.com/gardener/etcd-druid/pkg/utils"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler reconciles status of Etcd object
type Reconciler struct {
	client.Client
	scheme     *runtime.Scheme
	config     *Config
	logger     logr.Logger
	restConfig *rest.Config
}

// NewReconciler creates a new reconciler for Custodian.
func NewReconciler(mgr manager.Manager, config *Config) *Reconciler {
	return &Reconciler{
		Client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		config:     config,
		restConfig: mgr.GetConfig(),
		logger:     log.Log.WithName(controllerName),
	}
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;update;patch;create

// Reconcile reconciles the etcd.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger.Info("Custodian controller reconciliation started")
	etcd := &druidv1alpha1.Etcd{}
	if err := r.Get(ctx, req.NamespacedName, etcd); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String())

	if etcd.Status.LastError != nil && *etcd.Status.LastError != "" {
		logger.Info("Requeue item because of last error", "namespace", etcd.Namespace, "name", etcd.Name, "lastError", *etcd.Status.LastError)
		return ctrl.Result{
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	statusCheck := status.NewChecker(r.Client, r.config.EtcdMember.NotReadyThreshold, r.config.EtcdMember.UnknownThreshold)
	if err := statusCheck.Check(ctx, logger, etcd); err != nil {
		logger.Error(err, "Error executing status checks")
		return ctrl.Result{}, err
	}

	sts, err := utils.GetStatefulSet(ctx, r.Client, etcd)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.updateEtcdStatus(ctx, logger, etcd, sts); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.config.SyncPeriod}, nil
}

func (r *Reconciler) updateEtcdStatus(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) error {
	logger.Info("Updating etcd status with statefulset information", "namespace", etcd.Namespace, "name", etcd.Name)

	if sts != nil {
		etcd.Status.Etcd = &druidv1alpha1.CrossVersionObjectReference{
			APIVersion: sts.APIVersion,
			Kind:       sts.Kind,
			Name:       sts.Name,
		}

		ready, _ := utils.IsStatefulSetReady(etcd.Spec.Replicas, sts)

		// To be changed once we have multiple replicas.
		etcd.Status.CurrentReplicas = sts.Status.CurrentReplicas
		etcd.Status.ReadyReplicas = sts.Status.ReadyReplicas
		etcd.Status.UpdatedReplicas = sts.Status.UpdatedReplicas
		etcd.Status.Ready = &ready
		logger.Info("ETCD status updated for statefulset", "namespace", etcd.Namespace, "name", etcd.Name,
			"currentReplicas", sts.Status.CurrentReplicas, "readyReplicas", sts.Status.ReadyReplicas, "updatedReplicas", sts.Status.UpdatedReplicas)
	} else {
		etcd.Status.CurrentReplicas = 0
		etcd.Status.ReadyReplicas = 0
		etcd.Status.UpdatedReplicas = 0

		etcd.Status.Ready = pointer.Bool(false)
	}

	return r.Client.Status().Update(ctx, etcd)
}
