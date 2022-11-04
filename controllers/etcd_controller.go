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
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	componentconfigmap "github.com/gardener/etcd-druid/pkg/component/etcd/configmap"
	componentlease "github.com/gardener/etcd-druid/pkg/component/etcd/lease"
	componentpdb "github.com/gardener/etcd-druid/pkg/component/etcd/poddisruptionbudget"
	componentservice "github.com/gardener/etcd-druid/pkg/component/etcd/service"
	"github.com/gardener/etcd-druid/pkg/component/etcd/statefulset"
	componentsts "github.com/gardener/etcd-druid/pkg/component/etcd/statefulset"
	druidpredicates "github.com/gardener/etcd-druid/pkg/predicate"
	"github.com/gardener/etcd-druid/pkg/utils"

	extensionspredicate "github.com/gardener/gardener/extensions/pkg/predicate"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllerutils"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var (
	etcdGVK = druidv1alpha1.GroupVersion.WithKind("Etcd")

	// UncachedObjectList is a list of objects which should not be cached.
	UncachedObjectList = []client.Object{
		&corev1.Event{},
		&eventsv1beta1.Event{},
		&eventsv1.Event{},
	}
)

const (
	// FinalizerName is the name of the Plant finalizer.
	FinalizerName = "druid.gardener.cloud/etcd-druid"
	// DefaultImageVector is a constant for the path to the default image vector file.
	DefaultImageVector = "images.yaml"
	// DefaultInterval is the default interval for retry operations.
	DefaultInterval = 5 * time.Second
	// EtcdReady implies that etcd is ready
	EtcdReady = true
	// DefaultAutoCompactionRetention defines the default auto-compaction-retention length for etcd.
	DefaultAutoCompactionRetention = "30m"
	// Annotation set by human operator in order to stop reconciliation
	IgnoreReconciliationAnnotation = "druid.gardener.cloud/ignore-reconciliation"
)

var (
	// DefaultTimeout is the default timeout for retry operations.
	DefaultTimeout = 1 * time.Minute
)

// reconcileResult captures the result of a reconciliation run.
type reconcileResult struct {
	svcName *string
	sts     *appsv1.StatefulSet
	err     error
}

// EtcdReconciler reconciles a Etcd object
type EtcdReconciler struct {
	client.Client
	Scheme                             *runtime.Scheme
	recorder                           record.EventRecorder
	chartApplier                       kubernetes.ChartApplier
	Config                             *rest.Config
	ImageVector                        imagevector.ImageVector
	logger                             logr.Logger
	disableEtcdServiceAccountAutomount bool
}

// NewReconcilerWithImageVector creates a new EtcdReconciler object with an image vector
func NewReconcilerWithImageVector(mgr manager.Manager, disableEtcdServiceAccountAutomount bool) (*EtcdReconciler, error) {
	etcdReconciler, err := NewEtcdReconciler(mgr, disableEtcdServiceAccountAutomount)
	if err != nil {
		return nil, err
	}
	return etcdReconciler.InitializeControllerWithImageVector()
}

// NewEtcdReconciler creates a new EtcdReconciler object
func NewEtcdReconciler(mgr manager.Manager, disableEtcdServiceAccountAutomount bool) (*EtcdReconciler, error) {
	return (&EtcdReconciler{
		Client:                             mgr.GetClient(),
		Config:                             mgr.GetConfig(),
		Scheme:                             mgr.GetScheme(),
		recorder:                           mgr.GetEventRecorderFor("etcd-controller"),
		logger:                             log.Log.WithName("etcd-controller"),
		disableEtcdServiceAccountAutomount: disableEtcdServiceAccountAutomount,
	}).InitializeControllerWithChartApplier()
}

// NewEtcdReconcilerWithImageVector creates a new EtcdReconciler object
func NewEtcdReconcilerWithImageVector(mgr manager.Manager, disableEtcdServiceAccountAutomount bool) (*EtcdReconciler, error) {
	ec, err := NewEtcdReconciler(mgr, disableEtcdServiceAccountAutomount)
	if err != nil {
		return nil, err
	}
	return ec.InitializeControllerWithImageVector()
}

func getChartPath() string {
	return filepath.Join("charts", "etcd")
}

func getChartPathForServiceAccount() string {
	return filepath.Join("etcd", "templates", "etcd-serviceaccount.yaml")
}

func getChartPathForRole() string {
	return filepath.Join("etcd", "templates", "etcd-role.yaml")
}

func getChartPathForRoleBinding() string {
	return filepath.Join("etcd", "templates", "etcd-rolebinding.yaml")
}

func getImageYAMLPath() string {
	return filepath.Join(common.ChartPath, DefaultImageVector)
}

// InitializeControllerWithChartApplier will use EtcdReconciler client to initialize a Kubernetes client as well as
// a Chart renderer.
func (r *EtcdReconciler) InitializeControllerWithChartApplier() (*EtcdReconciler, error) {
	if r.chartApplier != nil {
		return r, nil
	}

	renderer, err := chartrenderer.NewForConfig(r.Config)
	if err != nil {
		return nil, err
	}
	applier, err := kubernetes.NewApplierForConfig(r.Config)
	if err != nil {
		return nil, err
	}
	r.chartApplier = kubernetes.NewChartApplier(renderer, applier)
	return r, nil
}

// InitializeControllerWithImageVector will use EtcdReconciler client to initialize image vector for etcd
// and backup restore images.
func (r *EtcdReconciler) InitializeControllerWithImageVector() (*EtcdReconciler, error) {
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getImageYAMLPath())
	if err != nil {
		return nil, err
	}
	r.ImageVector = imageVector
	return r, nil
}

// SetupWithManager sets up manager with a new controller and r as the reconcile.Reconciler
func (r *EtcdReconciler) SetupWithManager(mgr ctrl.Manager, workers int, ignoreOperationAnnotation bool) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: workers,
	})
	builder = builder.
		WithEventFilter(buildPredicate(ignoreOperationAnnotation)).
		For(&druidv1alpha1.Etcd{})
	if ignoreOperationAnnotation {
		builder = builder.Owns(&corev1.Service{}).
			Owns(&corev1.ConfigMap{}).
			Owns(&appsv1.StatefulSet{})
	}
	return builder.Complete(r)
}

func buildPredicate(ignoreOperationAnnotation bool) predicate.Predicate {
	if ignoreOperationAnnotation {
		return predicate.GenerationChangedPredicate{}
	}

	return predicate.Or(
		druidpredicates.HasOperationAnnotation(),
		druidpredicates.LastOperationNotSuccessful(),
		extensionspredicate.IsDeleting(),
	)
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// Reconcile reconciles the etcd.
func (r *EtcdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

func (r *EtcdReconciler) reconcile(ctx context.Context, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String(), "operation", "reconcile")

	// Add Finalizers to Etcd
	if finalizers := sets.NewString(etcd.Finalizers...); !finalizers.Has(FinalizerName) {
		logger.Info("Adding finalizer")
		if err := controllerutils.PatchAddFinalizers(ctx, r.Client, etcd, FinalizerName); err != nil {
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

func (r *EtcdReconciler) delete(ctx context.Context, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
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
	k8sversion, err := utils.GetClusterK8sVersion(r.Config)
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

	if sets.NewString(etcd.Finalizers...).Has(FinalizerName) {
		logger.Info("Removing finalizer")
		if err := controllerutils.PatchRemoveFinalizers(ctx, r.Client, etcd, FinalizerName); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	}
	logger.Info("Deleted etcd successfully.")
	return ctrl.Result{}, nil
}

func decodeObject(renderedChart *chartrenderer.RenderedChart, path string, object interface{}) error {
	if content, ok := renderedChart.Files()[path]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		return decoder.Decode(&object)
	}
	return fmt.Errorf("missing file %s in the rendered chart", path)
}

func (r *EtcdReconciler) reconcileServiceAccount(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, values map[string]interface{}) error {
	logger.Info("Reconciling serviceaccount")
	var err error
	decoded := &corev1.ServiceAccount{}
	serviceAccountPath := getChartPathForServiceAccount()
	chartPath := getChartPath()
	renderedChart, err := r.chartApplier.Render(chartPath, etcd.Name, etcd.Namespace, values)
	if err != nil {
		return err
	}
	if content, ok := renderedChart.Files()[serviceAccountPath]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		if err = decoder.Decode(&decoded); err != nil {
			return err
		}
	}

	obj := &corev1.ServiceAccount{}
	key := client.ObjectKeyFromObject(decoded)
	if err := r.Get(ctx, key, obj); err != nil {
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
		copy      = obj.DeepCopy()
	)

	if !reflect.DeepEqual(decoded.Labels, obj.Labels) {
		copy.Labels = decoded.Labels
		mustPatch = true
	}

	if !reflect.DeepEqual(decoded.AutomountServiceAccountToken, obj.AutomountServiceAccountToken) {
		copy.AutomountServiceAccountToken = decoded.AutomountServiceAccountToken
		mustPatch = true
	}

	if !mustPatch {
		return nil
	}

	logger.Info("Update serviceaccount")
	return r.Patch(ctx, copy, client.MergeFrom(obj))
}

func (r *EtcdReconciler) reconcileRole(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, values map[string]interface{}) error {
	logger.Info("Reconciling role")
	var err error
	decoded := &rbac.Role{}
	rolePath := getChartPathForRole()
	chartPath := getChartPath()
	renderedChart, err := r.chartApplier.Render(chartPath, etcd.Name, etcd.Namespace, values)
	if err != nil {
		return err
	}
	if content, ok := renderedChart.Files()[rolePath]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		if err = decoder.Decode(&decoded); err != nil {
			return err
		}
	}

	obj := &rbac.Role{}
	key := client.ObjectKeyFromObject(decoded)
	if err := r.Get(ctx, key, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, decoded); err != nil {
			return err
		}
		logger.Info("Creating role", "role", kutil.Key(decoded.Namespace, decoded.Name).String())
		return nil
	}

	if !reflect.DeepEqual(decoded.Rules, obj.Rules) {
		copy := obj.DeepCopy()
		copy.Rules = decoded.Rules
		if err := r.Patch(ctx, copy, client.MergeFrom(obj)); err != nil {
			return err
		}
	}

	return nil
}

func (r *EtcdReconciler) reconcileRoleBinding(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, values map[string]interface{}) error {
	logger.Info("Reconciling rolebinding")
	var err error
	decoded := &rbac.RoleBinding{}
	roleBindingPath := getChartPathForRoleBinding()
	chartPath := getChartPath()
	renderedChart, err := r.chartApplier.Render(chartPath, etcd.Name, etcd.Namespace, values)
	if err != nil {
		return err
	}
	if content, ok := renderedChart.Files()[roleBindingPath]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		if err = decoder.Decode(&decoded); err != nil {
			return err
		}
	}

	obj := &rbac.RoleBinding{}
	key := client.ObjectKeyFromObject(decoded)
	if err := r.Get(ctx, key, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, decoded); err != nil {
			return err
		}
		logger.Info("Creating rolebinding", "rolebinding", kutil.Key(decoded.Namespace, decoded.Name).String())
		return nil
	}

	if !reflect.DeepEqual(decoded.RoleRef, obj.RoleRef) || !reflect.DeepEqual(decoded.Subjects, obj.Subjects) {
		copy := obj.DeepCopy()
		copy.RoleRef = decoded.RoleRef
		copy.Subjects = decoded.Subjects
		if err := r.Patch(ctx, copy, client.MergeFrom(obj)); err != nil {
			return err
		}
	}

	return err
}

func (r *EtcdReconciler) reconcileEtcd(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) reconcileResult {
	// Check if Spec.Replicas is odd or even.
	// TODO(timuthy): The following checks should rather be part of a validation. Also re-enqueuing doesn't make sense in case the values are invalid.
	if etcd.Spec.Replicas > 1 && etcd.Spec.Replicas&1 == 0 {
		return reconcileResult{err: fmt.Errorf("Spec.Replicas should not be even number: %d", etcd.Spec.Replicas)}
	}

	etcdImage, etcdBackupImage, err := getEtcdImages(r.ImageVector, etcd)
	if err != nil {
		return reconcileResult{err: err}
	}

	if etcd.Spec.Etcd.Image == nil {
		if etcdImage == "" {
			return reconcileResult{err: fmt.Errorf("either etcd resource or image vector should have %s image while deploying statefulset", common.Etcd)}
		}
	} else {
		etcdImage = *etcd.Spec.Etcd.Image
	}

	if etcd.Spec.Backup.Image == nil {
		if etcdBackupImage == "" {
			return reconcileResult{err: fmt.Errorf("either etcd resource or image vector should have %s image while deploying statefulset", common.BackupRestore)}
		}
	} else {
		etcdBackupImage = *etcd.Spec.Backup.Image
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
	k8sversion, err := utils.GetClusterK8sVersion(r.Config)
	if err != nil {
		return reconcileResult{err: err}
	}
	pdbDeployer := componentpdb.New(r.Client, etcd.Namespace, &pdbValues, *k8sversion)
	if err := pdbDeployer.Deploy(ctx); err != nil {
		return reconcileResult{err: err}
	}

	values, err := r.getMapFromEtcd(etcd, r.disableEtcdServiceAccountAutomount)
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
	statefulSetValues := statefulset.GenerateValues(etcd,
		&serviceValues.ClientPort,
		&serviceValues.ServerPort,
		&serviceValues.BackupPort,
		etcdImage,
		etcdBackupImage,
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

func isPeerTLSIsChangedToEnabled(peerTLSEnabledStatusFromMembers bool, configMapValues *componentconfigmap.Values) bool {
	if peerTLSEnabledStatusFromMembers {
		return false
	}
	return configMapValues.PeerUrlTLS != nil
}

func checkEtcdOwnerReference(refs []metav1.OwnerReference, etcd *druidv1alpha1.Etcd) bool {
	for _, ownerRef := range refs {
		if ownerRef.UID == etcd.UID {
			return true
		}
	}
	return false
}

func checkEtcdAnnotations(annotations map[string]string, etcd metav1.Object) bool {
	var (
		ownedBy, ownerType string
		ok                 bool
	)
	if annotations == nil {
		return false
	}
	if ownedBy, ok = annotations[common.GardenerOwnedBy]; !ok {
		return ok
	}
	if ownerType, ok = annotations[common.GardenerOwnerType]; !ok {
		return ok
	}
	return ownedBy == fmt.Sprintf("%s/%s", etcd.GetNamespace(), etcd.GetName()) &&
		ownerType == strings.ToLower(etcdGVK.Kind)

}

func (r *EtcdReconciler) getMapFromEtcd(etcd *druidv1alpha1.Etcd, disableEtcdServiceAccountAutomount bool) (map[string]interface{}, error) {
	pdbMinAvailable := 0
	if etcd.Spec.Replicas > 1 {
		pdbMinAvailable = int(etcd.Spec.Replicas)
	}

	values := map[string]interface{}{
		"name":                               etcd.Name,
		"uid":                                etcd.UID,
		"labels":                             etcd.Spec.Labels,
		"pdbMinAvailable":                    pdbMinAvailable,
		"serviceAccountName":                 utils.GetServiceAccountName(etcd),
		"disableEtcdServiceAccountAutomount": disableEtcdServiceAccountAutomount,
		"roleName":                           fmt.Sprintf("druid.gardener.cloud:etcd:%s", etcd.Name),
		"roleBindingName":                    fmt.Sprintf("druid.gardener.cloud:etcd:%s", etcd.Name),
	}

	return values, nil
}

func getEtcdImages(im imagevector.ImageVector, etcd *druidv1alpha1.Etcd) (string, string, error) {
	var (
		err                        error
		images                     map[string]*imagevector.Image
		etcdImage, etcdBackupImage string
	)

	imageNames := []string{
		common.Etcd,
		common.BackupRestore,
	}

	if etcd.Spec.Etcd.Image == nil || etcd.Spec.Backup.Image == nil {

		images, err = imagevector.FindImages(im, imageNames)
		if err != nil {
			return "", "", err
		}
	}

	val, ok := images[common.Etcd]
	if !ok {
		etcdImage = ""
	} else {
		etcdImage = val.String()
	}

	val, ok = images[common.BackupRestore]
	if !ok {
		etcdBackupImage = ""
	} else {
		etcdBackupImage = val.String()
	}
	return etcdImage, etcdBackupImage, nil
}

func bootstrapReset(etcd *druidv1alpha1.Etcd) {
	etcd.Status.Members = nil
	etcd.Status.ClusterSize = pointer.Int32Ptr(etcd.Spec.Replicas)
}

func clusterInBootstrap(etcd *druidv1alpha1.Etcd) bool {
	return etcd.Status.Replicas == 0 ||
		(etcd.Spec.Replicas > 1 && etcd.Status.Replicas == 1)
}

func (r *EtcdReconciler) updateEtcdErrorStatus(ctx context.Context, etcd *druidv1alpha1.Etcd, result reconcileResult) error {
	return controllerutils.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcd, func() error {
		lastErrStr := fmt.Sprintf("%v", result.err)
		etcd.Status.LastError = &lastErrStr
		etcd.Status.ObservedGeneration = &etcd.Generation
		if result.sts != nil {
			if clusterInBootstrap(etcd) {
				// Reset members in bootstrap phase to ensure dependent conditions can be calculated correctly.
				bootstrapReset(etcd)
			}
			ready := utils.CheckStatefulSet(etcd.Spec.Replicas, result.sts) == nil
			etcd.Status.Ready = &ready
			etcd.Status.Replicas = pointer.Int32PtrDerefOr(result.sts.Spec.Replicas, 0)
		}
		return nil
	})
}

func (r *EtcdReconciler) updateEtcdStatus(ctx context.Context, etcd *druidv1alpha1.Etcd, result reconcileResult) error {
	return controllerutils.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcd, func() error {
		if clusterInBootstrap(etcd) {
			// Reset members in bootstrap phase to ensure dependent conditions can be calculated correctly.
			bootstrapReset(etcd)
		}
		if result.sts != nil {
			ready := utils.CheckStatefulSet(etcd.Spec.Replicas, result.sts) == nil
			etcd.Status.Ready = &ready
			etcd.Status.Replicas = pointer.Int32PtrDerefOr(result.sts.Spec.Replicas, 0)
		}
		etcd.Status.ServiceName = result.svcName
		etcd.Status.LastError = nil
		etcd.Status.ObservedGeneration = &etcd.Generation
		return nil
	})
}

func (r *EtcdReconciler) removeOperationAnnotation(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	if _, ok := etcd.Annotations[v1beta1constants.GardenerOperation]; ok {
		logger.Info("Removing operation annotation")
		withOpAnnotation := etcd.DeepCopy()
		delete(etcd.Annotations, v1beta1constants.GardenerOperation)
		return r.Patch(ctx, etcd, client.MergeFrom(withOpAnnotation))
	}
	return nil
}

func (r *EtcdReconciler) updateEtcdStatusAsNotReady(ctx context.Context, etcd *druidv1alpha1.Etcd) (*druidv1alpha1.Etcd, error) {
	err := controllerutils.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcd, func() error {
		etcd.Status.Ready = nil
		etcd.Status.ReadyReplicas = 0
		return nil
	})
	return etcd, err
}
