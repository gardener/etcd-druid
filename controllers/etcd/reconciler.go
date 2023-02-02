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
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"reflect"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/common"
	componentconfigmap "github.com/gardener/etcd-druid/pkg/component/etcd/configmap"
	componentlease "github.com/gardener/etcd-druid/pkg/component/etcd/lease"
	componentpdb "github.com/gardener/etcd-druid/pkg/component/etcd/poddisruptionbudget"
	componentservice "github.com/gardener/etcd-druid/pkg/component/etcd/service"
	componentsts "github.com/gardener/etcd-druid/pkg/component/etcd/statefulset"
	druidutils "github.com/gardener/etcd-druid/pkg/utils"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/controllerutils"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"
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

var (
	serviceAccountChartPath = filepath.Join("etcd", "templates", "etcd-serviceaccount.yaml")
	chartPath               = filepath.Join("charts", "etcd")
	roleChartPath           = filepath.Join("etcd", "templates", "etcd-role.yaml")
	roleBindingChartPath    = filepath.Join("etcd", "templates", "etcd-rolebinding.yaml")
)

// Reconciler reconciles Etcd resources.
type Reconciler struct {
	client.Client
	scheme        *runtime.Scheme
	config        *Config
	recorder      record.EventRecorder
	chartRenderer chartrenderer.Interface
	restConfig    *rest.Config
	imageVector   imagevector.ImageVector
	logger        logr.Logger
}

// NewReconciler creates a new reconciler for Etcd.
func NewReconciler(mgr manager.Manager, config *Config) (*Reconciler, error) {
	var (
		chartRenderer chartrenderer.Interface
		imageVector   imagevector.ImageVector
		err           error
	)
	if chartRenderer, err = chartrenderer.NewForConfig(mgr.GetConfig()); err != nil {
		return nil, err
	}
	if imageVector, err = utils.CreateImageVector(); err != nil {
		return nil, err
	}
	return &Reconciler{
		Client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		config:        config,
		recorder:      mgr.GetEventRecorderFor(controllerName),
		chartRenderer: chartRenderer,
		restConfig:    mgr.GetConfig(),
		imageVector:   imageVector,
		logger:        log.Log.WithName(controllerName),
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
		logger.Info("Adding finalizer")
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
	logger.Info("Starting operation")

	stsDeployer := gardenercomponent.OpDestroyAndWait(componentsts.New(r.Client, logger, componentsts.Values{Name: etcd.Name, Namespace: etcd.Namespace}))
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
	k8sversion, err := druidutils.GetClusterK8sVersion(r.restConfig)
	if err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}
	pdbDeployer := componentpdb.New(r.Client, etcd.Namespace, &pdbValues, *k8sversion)
	if err := pdbDeployer.Destroy(ctx); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	if sets.NewString(etcd.Finalizers...).Has(common.FinalizerName) {
		logger.Info("Removing finalizer")
		if err := controllerutils.RemoveFinalizers(ctx, r.Client, etcd, common.FinalizerName); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	}
	logger.Info("Deleted etcd successfully.")
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileServiceAccount(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, values map[string]interface{}) error {
	logger.Info("Reconciling serviceaccount")
	var err error
	decoded := &corev1.ServiceAccount{}
	// TODO(AleksandarSavchev): .Render is deprecated. Refactor or adapt code to use RenderEmbeddedFS https://github.com/gardener/gardener/pull/6165
	renderedChart, err := r.chartRenderer.Render(chartPath, etcd.Name, etcd.Namespace, values) //nolint:staticcheck
	if err != nil {
		return err
	}
	if content, ok := renderedChart.Files()[serviceAccountChartPath]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		if err = decoder.Decode(&decoded); err != nil {
			return err
		}
	}

	saObj := &corev1.ServiceAccount{}
	key := client.ObjectKeyFromObject(decoded)
	if err := r.Get(ctx, key, saObj); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, decoded); err != nil {
			return err
		}
		logger.Info("Creating serviceaccount", "serviceaccount", kutil.Key(decoded.Namespace, decoded.Name).String())
		return nil
	}

	var (
		mustPatch bool
		saObjCopy = saObj.DeepCopy()
	)

	if !reflect.DeepEqual(decoded.Labels, saObj.Labels) {
		saObjCopy.Labels = decoded.Labels
		mustPatch = true
	}

	if !reflect.DeepEqual(decoded.AutomountServiceAccountToken, saObj.AutomountServiceAccountToken) {
		saObjCopy.AutomountServiceAccountToken = decoded.AutomountServiceAccountToken
		mustPatch = true
	}

	if !mustPatch {
		return nil
	}

	logger.Info("Update serviceaccount")
	return r.Patch(ctx, saObjCopy, client.MergeFrom(saObj))
}

func (r *Reconciler) reconcileRole(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, values map[string]interface{}) error {
	logger.Info("Reconciling role")
	var err error
	decoded := &rbac.Role{}
	// TODO(AleksandarSavchev): .Render is deprecated. Refactor or adapt code to use RenderEmbeddedFS https://github.com/gardener/gardener/pull/6165
	renderedChart, err := r.chartRenderer.Render(chartPath, etcd.Name, etcd.Namespace, values) //nolint:staticcheck
	if err != nil {
		return err
	}
	if content, ok := renderedChart.Files()[roleChartPath]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		if err = decoder.Decode(&decoded); err != nil {
			return err
		}
	}

	roleObj := &rbac.Role{}
	key := client.ObjectKeyFromObject(decoded)
	if err := r.Get(ctx, key, roleObj); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, decoded); err != nil {
			return err
		}
		logger.Info("Creating role", "role", kutil.Key(decoded.Namespace, decoded.Name).String())
		return nil
	}

	if !reflect.DeepEqual(decoded.Rules, roleObj.Rules) {
		roleObjCopy := roleObj.DeepCopy()
		roleObjCopy.Rules = decoded.Rules
		if err := r.Patch(ctx, roleObjCopy, client.MergeFrom(roleObj)); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) reconcileRoleBinding(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, values map[string]interface{}) error {
	logger.Info("Reconciling rolebinding")
	var err error
	decoded := &rbac.RoleBinding{}
	// TODO(AleksandarSavchev): .Render is deprecated. Refactor or adapt code to use RenderEmbeddedFS https://github.com/gardener/gardener/pull/6165
	renderedChart, err := r.chartRenderer.Render(chartPath, etcd.Name, etcd.Namespace, values) //nolint:staticcheck
	if err != nil {
		return err
	}
	if content, ok := renderedChart.Files()[roleBindingChartPath]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		if err = decoder.Decode(&decoded); err != nil {
			return err
		}
	}

	roleBindingObj := &rbac.RoleBinding{}
	key := client.ObjectKeyFromObject(decoded)
	if err := r.Get(ctx, key, roleBindingObj); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, decoded); err != nil {
			return err
		}
		logger.Info("Creating rolebinding", "rolebinding", kutil.Key(decoded.Namespace, decoded.Name).String())
		return nil
	}

	if !reflect.DeepEqual(decoded.RoleRef, roleBindingObj.RoleRef) || !reflect.DeepEqual(decoded.Subjects, roleBindingObj.Subjects) {
		roleBindingObjCopy := roleBindingObj.DeepCopy()
		roleBindingObjCopy.RoleRef = decoded.RoleRef
		roleBindingObjCopy.Subjects = decoded.Subjects
		if err := r.Patch(ctx, roleBindingObjCopy, client.MergeFrom(roleBindingObj)); err != nil {
			return err
		}
	}

	return err
}

func (r *Reconciler) reconcileEtcd(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) reconcileResult {
	// Check if Spec.Replicas is odd or even.
	// TODO(timuthy): The following checks should rather be part of a validation. Also re-enqueuing doesn't make sense in case the values are invalid.
	if etcd.Spec.Replicas > 1 && etcd.Spec.Replicas&1 == 0 {
		return reconcileResult{err: fmt.Errorf("Spec.Replicas should not be even number: %d", etcd.Spec.Replicas)}
	}

	etcdImage, etcdBackupImage, err := druidutils.GetEtcdImages(etcd, r.imageVector)
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
	k8sversion, err := druidutils.GetClusterK8sVersion(r.restConfig)
	if err != nil {
		return reconcileResult{err: err}
	}
	pdbDeployer := componentpdb.New(r.Client, etcd.Namespace, &pdbValues, *k8sversion)
	if err := pdbDeployer.Deploy(ctx); err != nil {
		return reconcileResult{err: err}
	}

	values, err := r.getMapFromEtcd(etcd, r.config.DisableEtcdServiceAccountAutomount)
	if err != nil {
		return reconcileResult{err: err}
	}

	err = r.reconcileServiceAccount(ctx, logger, etcd, values)
	if err != nil {
		return reconcileResult{err: err}
	}

	err = r.reconcileRole(ctx, logger, etcd, values)
	if err != nil {
		return reconcileResult{err: err}
	}

	err = r.reconcileRoleBinding(ctx, logger, etcd, values)
	if err != nil {
		return reconcileResult{err: err}
	}

	peerTLSEnabled, err := leaseDeployer.GetPeerURLTLSEnabledStatus(ctx)
	if err != nil {
		return reconcileResult{err: err}
	}

	peerUrlTLSChangedToEnabled := isPeerTLSIsChangedToEnabled(peerTLSEnabled, configMapValues)
	statefulSetValues := componentsts.GenerateValues(etcd,
		&serviceValues.ClientPort,
		&serviceValues.ServerPort,
		&serviceValues.BackupPort,
		*etcdImage,
		*etcdBackupImage,
		map[string]string{
			"checksum/etcd-configmap": configMapValues.ConfigMapChecksum,
		}, peerUrlTLSChangedToEnabled)

	// Create an OpWaiter because after the deployment we want to wait until the StatefulSet is ready.
	var (
		stsDeployer  = componentsts.New(r.Client, logger, statefulSetValues)
		deployWaiter = gardenercomponent.OpWaiter(stsDeployer)
	)

	if err = deployWaiter.Deploy(ctx); err != nil {
		return reconcileResult{err: err}
	}

	sts, err := stsDeployer.Get(ctx)
	return reconcileResult{svcName: &serviceValues.ClientServiceName, sts: sts, err: err}
}

func (r *Reconciler) getMapFromEtcd(etcd *druidv1alpha1.Etcd, disableEtcdServiceAccountAutomount bool) (map[string]interface{}, error) {
	pdbMinAvailable := 0
	if etcd.Spec.Replicas > 1 {
		pdbMinAvailable = int(etcd.Spec.Replicas)
	}

	values := map[string]interface{}{
		"name":                               etcd.Name,
		"uid":                                etcd.UID,
		"labels":                             etcd.Spec.Labels,
		"pdbMinAvailable":                    pdbMinAvailable,
		"serviceAccountName":                 etcd.GetServiceAccountName(),
		"disableEtcdServiceAccountAutomount": disableEtcdServiceAccountAutomount,
		"roleName":                           fmt.Sprintf("druid.gardener.cloud:etcd:%s", etcd.Name),
		"roleBindingName":                    fmt.Sprintf("druid.gardener.cloud:etcd:%s", etcd.Name),
	}

	return values, nil
}

func isPeerTLSIsChangedToEnabled(peerTLSEnabledStatusFromMembers bool, configMapValues *componentconfigmap.Values) bool {
	if peerTLSEnabledStatusFromMembers {
		return false
	}
	return configMapValues.PeerUrlTLS != nil
}

func bootstrapReset(etcd *druidv1alpha1.Etcd) {
	etcd.Status.Members = nil
	etcd.Status.ClusterSize = pointer.Int32(etcd.Spec.Replicas)
}

func clusterInBootstrap(etcd *druidv1alpha1.Etcd) bool {
	return etcd.Status.Replicas == 0 ||
		(etcd.Spec.Replicas > 1 && etcd.Status.Replicas == 1)
}

func (r *Reconciler) updateEtcdErrorStatus(ctx context.Context, etcd *druidv1alpha1.Etcd, result reconcileResult) error {
	lastErrStr := fmt.Sprintf("%v", result.err)
	etcd.Status.LastError = &lastErrStr
	etcd.Status.ObservedGeneration = &etcd.Generation
	if result.sts != nil {
		if clusterInBootstrap(etcd) {
			// Reset members in bootstrap phase to ensure dependent conditions can be calculated correctly.
			bootstrapReset(etcd)
		}
		ready, _ := druidutils.IsStatefulSetReady(etcd.Spec.Replicas, result.sts)
		etcd.Status.Ready = &ready
		etcd.Status.Replicas = pointer.Int32Deref(result.sts.Spec.Replicas, 0)
	}

	return r.Client.Status().Update(ctx, etcd)
}

func (r *Reconciler) updateEtcdStatus(ctx context.Context, etcd *druidv1alpha1.Etcd, result reconcileResult) error {
	if clusterInBootstrap(etcd) {
		// Reset members in bootstrap phase to ensure dependent conditions can be calculated correctly.
		bootstrapReset(etcd)
	}
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
		logger.Info("Removing operation annotation")
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
