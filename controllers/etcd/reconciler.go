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
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	ctrlutils "github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/common"
	componentconfigmap "github.com/gardener/etcd-druid/pkg/component/etcd/configmap"
	componentlease "github.com/gardener/etcd-druid/pkg/component/etcd/lease"
	componentpdb "github.com/gardener/etcd-druid/pkg/component/etcd/poddisruptionbudget"
	componentrole "github.com/gardener/etcd-druid/pkg/component/etcd/role"
	componentrolebinding "github.com/gardener/etcd-druid/pkg/component/etcd/rolebinding"
	componentservice "github.com/gardener/etcd-druid/pkg/component/etcd/service"
	componentserviceaccount "github.com/gardener/etcd-druid/pkg/component/etcd/serviceaccount"
	componentsts "github.com/gardener/etcd-druid/pkg/component/etcd/statefulset"
	"github.com/gardener/etcd-druid/pkg/features"
	druidutils "github.com/gardener/etcd-druid/pkg/utils"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	gardenercomponent "github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// IgnoreReconciliationAnnotation is an annotation set by an operator in order to stop reconciliation.
	IgnoreReconciliationAnnotation = "druid.gardener.cloud/ignore-reconciliation"
)

// Reconciler reconciles Etcd resources.
type Reconciler struct {
	client.Client
	scheme      *runtime.Scheme
	config      *Config
	recorder    record.EventRecorder
	restConfig  *rest.Config
	imageVector imagevector.ImageVector
	logger      logr.Logger
}

// NewReconciler creates a new reconciler for Etcd.
func NewReconciler(mgr manager.Manager, config *Config) (*Reconciler, error) {
	imageVector, err := ctrlutils.CreateImageVector()
	if err != nil {
		return nil, err
	}
	return NewReconcilerWithImageVector(mgr, config, imageVector)
}

// NewReconcilerWithImageVector creates a new reconciler for Etcd with an ImageVector.
// This constructor will mostly be used by tests.
func NewReconcilerWithImageVector(mgr manager.Manager, config *Config, imageVector imagevector.ImageVector) (*Reconciler, error) {
	return &Reconciler{
		Client:      mgr.GetClient(),
		scheme:      mgr.GetScheme(),
		config:      config,
		recorder:    mgr.GetEventRecorderFor(controllerName),
		restConfig:  mgr.GetConfig(),
		imageVector: imageVector,
		logger:      log.Log.WithName(controllerName),
	}, nil
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/finalizers,verbs=get;update;patch;create
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts;services;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;get;list;watch;patch;update

// Reconcile reconciles Etcd resources.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger.Info("ETCD controller reconciliation started")
	etcd := &druidv1alpha1.Etcd{}
	if err := r.Get(ctx, req.NamespacedName, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	if !etcd.DeletionTimestamp.IsZero() {
		return r.delete(ctx, etcd)
	}

	if _, ok := etcd.Annotations[IgnoreReconciliationAnnotation]; ok {
		r.recorder.Eventf(
			etcd,
			corev1.EventTypeWarning,
			"ReconciliationIgnored",
			"reconciliation of %s/%s is ignored by etcd-druid due to the presence of annotation %s on the etcd resource",
			etcd.Namespace,
			etcd.Name,
			IgnoreReconciliationAnnotation,
		)

		return ctrl.Result{
			Requeue: false,
		}, nil
	}

	return r.reconcile(ctx, etcd)
}

// reconcileResult captures the result of a reconciliation run.
type reconcileResult struct {
	svcName *string
	sts     *appsv1.StatefulSet
	err     error
}

func (r *Reconciler) reconcile(ctx context.Context, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String(), "operation", "reconcile")

	// Add Finalizers to Etcd
	if finalizers := sets.NewString(etcd.Finalizers...); !finalizers.Has(common.FinalizerName) {
		logger.Info("Adding finalizer", "namespace", etcd.Namespace, "name", etcd.Name, "finalizerName", common.FinalizerName)
		if err := controllerutils.AddFinalizers(ctx, r.Client, etcd, common.FinalizerName); err != nil {
			if err := r.updateEtcdErrorStatus(ctx, etcd, reconcileResult{err: err}); err != nil {
				return ctrl.Result{
					Requeue: true,
				}, err
			}
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	}

	etcd, err := r.updateEtcdStatusAsNotReady(ctx, etcd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	if err = r.removeOperationAnnotation(ctx, logger, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	result := r.reconcileEtcd(ctx, logger, etcd)
	if result.err != nil {
		if updateEtcdErr := r.updateEtcdErrorStatus(ctx, etcd, result); updateEtcdErr != nil {
			logger.Error(updateEtcdErr, "Error during reconciling ETCD")
			return ctrl.Result{
				Requeue: true,
			}, updateEtcdErr
		}
		return ctrl.Result{
			Requeue: true,
		}, result.err
	}
	if err := r.updateEtcdStatus(ctx, etcd, result); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	return ctrl.Result{
		Requeue: false,
	}, nil
}

func (r *Reconciler) delete(ctx context.Context, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String(), "operation", "delete")
	logger.Info("Starting deletion operation", "namespace", etcd.Namespace, "name", etcd.Name)

	stsDeployer := gardenercomponent.OpDestroyAndWait(componentsts.New(r.Client, logger, componentsts.Values{Name: etcd.Name, Namespace: etcd.Namespace}, r.config.FeatureGates))
	if err := stsDeployer.Destroy(ctx); err != nil {
		if err = r.updateEtcdErrorStatus(ctx, etcd, reconcileResult{err: err}); err != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	leaseDeployer := componentlease.New(r.Client, logger, etcd.Namespace, componentlease.GenerateValues(etcd))
	if err := leaseDeployer.Destroy(ctx); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	cmDeployer := componentconfigmap.New(r.Client, etcd.Namespace, componentconfigmap.GenerateValues(etcd))
	if err := cmDeployer.Destroy(ctx); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	pdbValues := componentpdb.GenerateValues(etcd)
	pdbDeployer := componentpdb.New(r.Client, etcd.Namespace, &pdbValues)
	if err := pdbDeployer.Destroy(ctx); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	saValues := componentserviceaccount.GenerateValues(etcd, r.config.DisableEtcdServiceAccountAutomount)
	saDeployer := componentserviceaccount.New(r.Client, saValues)
	if err := saDeployer.Destroy(ctx); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	roleValues := componentrole.GenerateValues(etcd)
	roleDeployer := componentrole.New(r.Client, roleValues)
	if err := roleDeployer.Destroy(ctx); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	roleBindingValues := componentrolebinding.GenerateValues(etcd)
	roleBindingDeployer := componentrolebinding.New(r.Client, roleBindingValues)
	if err := roleBindingDeployer.Destroy(ctx); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	if sets.NewString(etcd.Finalizers...).Has(common.FinalizerName) {
		logger.Info("Removing finalizer", "namespace", etcd.Namespace, "name", etcd.Name, "finalizerName", common.FinalizerName)
		if err := controllerutils.RemoveFinalizers(ctx, r.Client, etcd, common.FinalizerName); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	}
	logger.Info("Deleted etcd successfully", "namespace", etcd.Namespace, "name", etcd.Name)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileEtcd(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) reconcileResult {
	// Check if Spec.Replicas is odd or even.
	// TODO(timuthy): The following checks should rather be part of a validation. Also re-enqueuing doesn't make sense in case the values are invalid.
	if etcd.Spec.Replicas > 1 && etcd.Spec.Replicas&1 == 0 {
		return reconcileResult{err: fmt.Errorf("Spec.Replicas should not be even number: %d", etcd.Spec.Replicas)}
	}

	etcdImage, etcdBackupImage, initContainerImage, err := druidutils.GetEtcdImages(etcd, r.imageVector, r.config.FeatureGates[features.UseEtcdWrapper])
	if err != nil {
		return reconcileResult{err: err}
	}

	leaseValues := componentlease.GenerateValues(etcd)
	leaseDeployer := componentlease.New(r.Client, logger, etcd.Namespace, leaseValues)
	if err := leaseDeployer.Deploy(ctx); err != nil {
		return reconcileResult{err: err}
	}

	serviceValues := componentservice.GenerateValues(etcd)
	serviceDeployer := componentservice.New(r.Client, etcd.Namespace, serviceValues)
	if err := serviceDeployer.Deploy(ctx); err != nil {
		return reconcileResult{err: err}
	}

	configMapValues := componentconfigmap.GenerateValues(etcd)
	cmDeployer := componentconfigmap.New(r.Client, etcd.Namespace, configMapValues)
	if err := cmDeployer.Deploy(ctx); err != nil {
		return reconcileResult{err: err}
	}

	pdbValues := componentpdb.GenerateValues(etcd)
	pdbDeployer := componentpdb.New(r.Client, etcd.Namespace, &pdbValues)
	if err := pdbDeployer.Deploy(ctx); err != nil {
		return reconcileResult{err: err}
	}

	saValues := componentserviceaccount.GenerateValues(etcd, r.config.DisableEtcdServiceAccountAutomount)
	saDeployer := componentserviceaccount.New(r.Client, saValues)
	err = saDeployer.Deploy(ctx)
	if err != nil {
		return reconcileResult{err: err}
	}

	roleValues := componentrole.GenerateValues(etcd)
	roleDeployer := componentrole.New(r.Client, roleValues)
	err = roleDeployer.Deploy(ctx)
	if err != nil {
		return reconcileResult{err: err}
	}

	roleBindingValues := componentrolebinding.GenerateValues(etcd)
	roleBindingDeployer := componentrolebinding.New(r.Client, roleBindingValues)
	err = roleBindingDeployer.Deploy(ctx)
	if err != nil {
		return reconcileResult{err: err}
	}

	peerTLSEnabled, err := druidutils.IsPeerURLTLSEnabled(ctx, r.Client, etcd.Namespace, etcd.Name, logger)
	if err != nil {
		return reconcileResult{err: err}
	}

	peerUrlTLSChangedToEnabled := isPeerTLSChangedToEnabled(peerTLSEnabled, configMapValues)
	statefulSetValues, err := componentsts.GenerateValues(
		etcd,
		&serviceValues.ClientPort,
		&serviceValues.PeerPort,
		&serviceValues.BackupPort,
		*etcdImage,
		*etcdBackupImage,
		*initContainerImage,
		map[string]string{
			"checksum/etcd-configmap": configMapValues.ConfigMapChecksum,
		}, peerUrlTLSChangedToEnabled,
		r.config.FeatureGates[features.UseEtcdWrapper],
	)
	if err != nil {
		return reconcileResult{err: err}
	}

	// Create an OpWaiter because after the deployment we want to wait until the StatefulSet is ready.
	var (
		stsDeployer  = componentsts.New(r.Client, logger, *statefulSetValues, r.config.FeatureGates)
		deployWaiter = gardenercomponent.OpWait(stsDeployer)
	)

	if err = deployWaiter.Deploy(ctx); err != nil {
		return reconcileResult{err: err}
	}

	sts, err := stsDeployer.Get(ctx)
	return reconcileResult{svcName: &serviceValues.ClientServiceName, sts: sts, err: err}
}

func isPeerTLSChangedToEnabled(peerTLSEnabledStatusFromMembers bool, configMapValues *componentconfigmap.Values) bool {
	if peerTLSEnabledStatusFromMembers {
		return false
	}
	return configMapValues.PeerUrlTLS != nil
}

func (r *Reconciler) updateEtcdErrorStatus(ctx context.Context, etcd *druidv1alpha1.Etcd, result reconcileResult) error {
	lastErrStr := result.err.Error()
	etcd.Status.LastError = &lastErrStr
	etcd.Status.ObservedGeneration = &etcd.Generation
	if result.sts != nil {
		ready, _ := druidutils.IsStatefulSetReady(etcd.Spec.Replicas, result.sts)
		etcd.Status.Ready = &ready
		etcd.Status.Replicas = pointer.Int32Deref(result.sts.Spec.Replicas, 0)
	}

	return r.Client.Status().Update(ctx, etcd)
}

func (r *Reconciler) updateEtcdStatus(ctx context.Context, etcd *druidv1alpha1.Etcd, result reconcileResult) error {
	if result.sts != nil {
		ready, _ := druidutils.IsStatefulSetReady(etcd.Spec.Replicas, result.sts)
		etcd.Status.Ready = &ready
		etcd.Status.Replicas = pointer.Int32Deref(result.sts.Spec.Replicas, 0)
	}
	etcd.Status.ServiceName = result.svcName
	etcd.Status.LastError = nil
	etcd.Status.ObservedGeneration = &etcd.Generation

	return r.Client.Status().Update(ctx, etcd)
}

func (r *Reconciler) removeOperationAnnotation(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	if _, ok := etcd.Annotations[v1beta1constants.GardenerOperation]; ok {
		logger.Info("Removing operation annotation", "namespace", etcd.Namespace, "name", etcd.Name, "annotation", v1beta1constants.GardenerOperation)
		withOpAnnotation := etcd.DeepCopy()
		delete(etcd.Annotations, v1beta1constants.GardenerOperation)
		return r.Patch(ctx, etcd, client.MergeFrom(withOpAnnotation))
	}
	return nil
}

func (r *Reconciler) updateEtcdStatusAsNotReady(ctx context.Context, etcd *druidv1alpha1.Etcd) (*druidv1alpha1.Etcd, error) {
	etcd.Status.Ready = nil
	etcd.Status.ReadyReplicas = 0

	return etcd, r.Client.Status().Update(ctx, etcd)
}
