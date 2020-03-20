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

package etcd

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/chartrenderer"
	kubernetes "github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	druidpredicates "github.com/gardener/etcd-druid/pkg/predicate"
	"github.com/gardener/etcd-druid/pkg/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
	gardenerretry "github.com/gardener/gardener/pkg/utils/retry"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	logger  = logrus.New()
	etcdGVK = druidv1alpha1.GroupVersion.WithKind("Etcd")
)

const (
	// FinalizerName is the name of the Plant finalizer.
	FinalizerName = "druid.gardener.cloud/etcd-druid"
	// DefaultImageVector is a constant for the path to the default image vector file.
	DefaultImageVector = "images.yaml"
	// DefaultTimeout is the default timeout for retry operations.
	DefaultTimeout = time.Minute
	// DefaultInterval is the default interval for retry operations.
	DefaultInterval = 5 * time.Second
)

// Reconciler reconciles a Etcd object
type Reconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	chartApplier kubernetes.ChartApplier
	Config       *rest.Config
	ImageVector  imagevector.ImageVector
}

// NewReconcilerWithImageVector creates a new Reconciler object with an image vector
func NewReconcilerWithImageVector(mgr manager.Manager) (*Reconciler, error) {
	Reconciler, err := NewEtcdReconciler(mgr)
	if err != nil {
		return nil, err
	}
	return Reconciler.InitializeControllerWithImageVector()
}

// NewEtcdReconciler creates a new Reconciler object
func NewEtcdReconciler(mgr manager.Manager) (*Reconciler, error) {
	return (&Reconciler{
		Client: mgr.GetClient(),
		Config: mgr.GetConfig(),
		Scheme: mgr.GetScheme(),
	}).InitializeControllerWithChartApplier()
}

// NewEtcdReconcilerWithImageVector creates a new EtcdReconciler object
func NewEtcdReconcilerWithImageVector(mgr manager.Manager) (*Reconciler, error) {
	ec, err := (&Reconciler{
		Client: mgr.GetClient(),
		Config: mgr.GetConfig(),
		Scheme: mgr.GetScheme(),
	}).InitializeControllerWithChartApplier()
	if err != nil {
		return nil, err
	}
	return ec.InitializeControllerWithImageVector()
}

func getChartPath() string {
	return filepath.Join(common.ChartPath, "etcd")
}

func getChartPathForStatefulSet() string {
	return filepath.Join("etcd", "templates", "etcd-statefulset.yaml")
}

func getChartPathForConfigMap() string {
	return filepath.Join("etcd", "templates", "etcd-bootstrap-configmap.yaml")
}

func getChartPathForService() string {
	return filepath.Join("etcd", "templates", "etcd-service.yaml")
}

func getImageYAMLPath() string {
	return filepath.Join(common.ChartPath, DefaultImageVector)
}

// InitializeControllerWithChartApplier will use EtcdReconciler client to initialize a Kubernetes client as well as
// a Chart renderer.
func (r *Reconciler) InitializeControllerWithChartApplier() (*Reconciler, error) {
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
func (r *Reconciler) InitializeControllerWithImageVector() (*Reconciler, error) {
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getImageYAMLPath())
	if err != nil {
		return nil, err
	}
	r.ImageVector = imageVector
	return r, nil
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;update;patch

// Reconcile reconciles the etcd.
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.TODO()
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

	annotations := etcd.GetAnnotations()
	if annotations != nil && annotations[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile {
		withOpAnnotation := etcd.DeepCopy()
		delete(annotations, v1beta1constants.GardenerOperation)
		withOpAnnotation.SetAnnotations(annotations)
		if err := r.Patch(ctx, withOpAnnotation, client.MergeFrom(etcd)); err != nil {
			return reconcile.Result{}, err
		}
	}

	logger.Infof("Reconciling etcd: %s/%s", etcd.GetNamespace(), etcd.GetName())
	if !etcd.DeletionTimestamp.IsZero() {
		return r.delete(ctx, etcd)
	}
	return r.reconcile(ctx, etcd)
}

func (r *Reconciler) reconcile(ctx context.Context, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	// Add Finalizers to Etcd
	if finalizers := sets.NewString(etcd.Finalizers...); !finalizers.Has(FinalizerName) {
		logger.Infof("Adding finalizer (%s) to etcd %s", FinalizerName, etcd.GetName())
		finalizers.Insert(FinalizerName)
		etcd.Finalizers = finalizers.UnsortedList()
		if err := r.Update(ctx, etcd); err != nil {
			if err := r.updateEtcdErrorStatus(etcd, nil, err); err != nil {
				return ctrl.Result{
					Requeue: true,
				}, err
			}
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	}
	if err := r.addFinalizersToDependantSecrets(ctx, etcd); err != nil {
		if err := r.updateEtcdErrorStatus(etcd, nil, err); err != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	}

	etcd, err := r.updateEtcdStatusAsNotReady(etcd)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	svc, ss, err := r.reconcileEtcd(ctx, etcd)
	if err != nil {
		if err := r.updateEtcdErrorStatus(etcd, ss, err); err != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	if err := r.updateEtcdStatus(etcd, svc, ss); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}
	logger.Infof("Successfully reconciled etcd: %s/%s", etcd.GetNamespace(), etcd.GetName())
	return ctrl.Result{}, nil
}

func (r *Reconciler) determineInternalServiceName(ctx context.Context, etcd *druidv1alpha1.Etcd) (string, error) {
	return r.determineService(ctx, etcd, common.ServiceScopeInternal)
}

func (r *Reconciler) determineExternalServiceName(ctx context.Context, etcd *druidv1alpha1.Etcd) (string, error) {
	return r.determineService(ctx, etcd, common.ServiceScopeExternal)
}

func (r *Reconciler) determineService(ctx context.Context, etcd *druidv1alpha1.Etcd, scope string) (string, error) {
	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		logger.Errorf("failed to convert label selector to selector, %v", err)
		return "", err
	}
	req, err := labels.NewRequirement(common.ServiceScopeLabel, selection.Equals, []string{scope})
	if err != nil {
		return "", err
	}
	selector = selector.Add(*req)

	logger.Infof("seraching with selector: %v", selector)
	// list all services to include the services that don't match the etcd`s selector
	// anymore but has the stale controller ref.
	services := &corev1.ServiceList{}
	if err := r.List(ctx, services, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		logger.Error(err, "Error listing services")
		return "", err
	}

	// NOTE: filteredServices are pointing to deepcopies of the cache, but this could change in the future.
	// Ref: https://github.com/kubernetes-sigs/controller-runtime/blob/release-0.2/pkg/cache/internal/cache_reader.go#L74
	// if you need to modify them, you need to copy it first.
	filteredServices, err := r.claimServices(etcd, selector, services)
	if err != nil {
		logger.Error(err, "Error claiming service")
		return "", err
	}

	if len(filteredServices) > 0 {
		logger.Infof("Claiming existing etcd services for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)
		// TODO: give preference to default serviceName
		// Keep only 1 Service. Delete the rest
		for i := 1; i < len(filteredServices); i++ {
			svc := filteredServices[i]
			logger.Infof("deleting duplicate service: %s/%s", svc.Name, svc.Namespace)
			if err := r.Delete(ctx, svc); err != nil {
				return "", fmt.Errorf("failed to delete duplicate service: %v", err)
			}
		}

		return filteredServices[0].Name, nil
	}
	// Return the default serviceName.
	return fmt.Sprintf("%s-%s", etcd.Name, scope), nil
}

func (r *Reconciler) reconcileInternalService(ctx context.Context, etcd *druidv1alpha1.Etcd, serviceName string, renderedChart *chartrenderer.RenderedChart) (*corev1.Service, error) {
	logger.Infof("Reconciling etcd internal service for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)
	expectedSVC, err := r.getInternalServiceFromChart(renderedChart)
	if err != nil {
		return nil, err
	}

	existingSVC := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: etcd.Namespace}, existingSVC); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		if err := r.Client.Create(ctx, expectedSVC); err != nil {
			return nil, err
		}
		logger.Infof("Creating new etcd internal service :%s in namespace:%s", expectedSVC.Name, expectedSVC.Namespace)
		return expectedSVC, nil
	}
	logger.Infof("Updating etcd internal service :%s in namespace:%s", existingSVC.Name, existingSVC.Namespace)
	if reflect.DeepEqual(existingSVC.Spec, expectedSVC.Spec) {
		return existingSVC, nil
	}
	svc := existingSVC.DeepCopy()
	expectedSVC.Spec.DeepCopyInto(&svc.Spec)
	svc.Spec.ClusterIP = existingSVC.Spec.ClusterIP

	if err := controllerutil.SetControllerReference(etcd, svc, r.Scheme); err != nil {
		return nil, err
	}

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		//TODO: Create or update
		return r.Patch(ctx, svc, client.MergeFrom(existingSVC))
	}); err != nil {
		return nil, err
	}
	return svc, nil
}

func (r *Reconciler) reconcileExternalService(ctx context.Context, etcd *druidv1alpha1.Etcd, serviceName string) (*corev1.Service, error) {
	logger.Infof("Reconciling etcd external service for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)
	expectedSVC, err := r.getExternalServiceFromEtcd(etcd, serviceName)
	if err != nil {
		return nil, err
	}

	existingSVC := &corev1.Service{}

	if err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: etcd.Namespace}, existingSVC); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		// if existing service not found, create it.
		if err := r.Client.Create(ctx, expectedSVC); err != nil {
			return nil, err
		}
		return expectedSVC, nil
	}

	if reflect.DeepEqual(existingSVC.Spec, expectedSVC.Spec) {
		return existingSVC, nil
	}
	svc := existingSVC.DeepCopy()
	expectedSVC.Spec.DeepCopyInto(&svc.Spec)
	svc.Spec.ClusterIP = existingSVC.Spec.ClusterIP

	if existingSVC.Spec.Selector != nil {
		_, ok := existingSVC.Spec.Selector["healthy"]
		if !ok {
			delete(svc.Spec.Selector, "healthy")
		}
	}

	if err := controllerutil.SetControllerReference(etcd, svc, r.Scheme); err != nil {
		return nil, err
	}

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Patch(ctx, svc, client.MergeFrom(existingSVC))
	}); err != nil {
		return nil, err
	}
	return svc, nil
}

func (r *Reconciler) determineConfigmapName(ctx context.Context, etcd *druidv1alpha1.Etcd) (string, error) {
	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		logger.Errorf("Error converting etcd selector to selector: %v", err)
		return "", err
	}

	// list all configmaps to include the configmaps that don't match the etcd`s selector
	// anymore but has the stale controller ref.
	cms := &corev1.ConfigMapList{}
	err = r.List(ctx, cms, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		logger.Errorf("Error listing configmaps: %v", err)
		return "", err
	}

	// NOTE: filteredStatefulSets are pointing to deepcopies of the cache, but this could change in the future.
	// Ref: https://github.com/kubernetes-sigs/controller-runtime/blob/release-0.2/pkg/cache/internal/cache_reader.go#L74
	// if you need to modify them, you need to copy it first.
	filteredCMs, err := r.claimConfigMaps(etcd, selector, cms)
	if err != nil {
		return "", err
	}

	if len(filteredCMs) > 0 {
		logger.Infof("Claiming existing etcd configmaps for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)

		// Keep only 1 Configmap. Delete the rest
		for i := 1; i < len(filteredCMs); i++ {
			cm := filteredCMs[i]
			if err := r.Delete(ctx, cm); err != nil {
				return "", fmt.Errorf("Error in deleting duplicate configmaps: %v", err)
			}
		}
		return filteredCMs[0].Name, nil
	}
	return etcd.Name, nil
}

func (r *Reconciler) reconcileConfigMap(ctx context.Context, etcd *druidv1alpha1.Etcd, cmName string, renderedChart *chartrenderer.RenderedChart) (*corev1.ConfigMap, error) {
	logger.Infof("Reconciling etcd configmap for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)

	expectedCM, err := r.getConfigmapFromChart(renderedChart)
	if err != nil {
		return nil, err
	}

	existingCM := &corev1.ConfigMap{}
	if err = r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: etcd.Namespace}, existingCM); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		if err := r.Create(ctx, expectedCM); err != nil {
			return nil, err
		}
		return expectedCM, nil
	}

	if reflect.DeepEqual(existingCM.Data, expectedCM.Data) {
		return existingCM, nil
	}

	cmCopy := existingCM.DeepCopy()
	cmCopy.Data = expectedCM.Data

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Patch(ctx, cmCopy, client.MergeFrom(existingCM))
	}); err != nil {
		return nil, err
	}
	return cmCopy, err
}

func (r *Reconciler) determineStatefulSetName(ctx context.Context, etcd *druidv1alpha1.Etcd) (string, error) {
	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		logger.Errorf("Error converting etcd selector to selector: %v", err)
		return "", err
	}

	// list all statefulsets to include the statefulsets that don't match the etcd`s selector
	// anymore but has the stale controller ref.
	statefulSets := &appsv1.StatefulSetList{}
	err = r.List(ctx, statefulSets, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		logger.Errorf("Error listing statefulsets: %v", err)
		return "", err
	}

	// NOTE: filteredStatefulSets are pointing to deepcopies of the cache, but this could change in the future.
	// Ref: https://github.com/kubernetes-sigs/controller-runtime/blob/release-0.2/pkg/cache/internal/cache_reader.go#L74
	// if you need to modify them, you need to copy it first.
	filteredStatefulSets, err := r.claimStatefulSets(etcd, selector, statefulSets)
	if err != nil {
		return "", err
	}

	if len(filteredStatefulSets) > 0 {
		logger.Infof("Claiming existing etcd statefulsets for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)

		// Keep only 1 Configmap. Delete the rest
		for i := 1; i < len(filteredStatefulSets); i++ {
			set := filteredStatefulSets[i]
			if err := r.Delete(ctx, set); err != nil {
				return "", fmt.Errorf("Error in deleting duplicate configmaps: %v", err)
			}
		}
		return filteredStatefulSets[0].Name, nil
	}
	return etcd.Name, nil
}

func (r *Reconciler) reconcileStatefulSet(ctx context.Context, etcd *druidv1alpha1.Etcd, stsName string, renderedChart *chartrenderer.RenderedChart) (*appsv1.StatefulSet, error) {
	logger.Infof("Reconciling etcd statefulset for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)

	expectedSts, err := r.getStatefulSetFromChart(renderedChart)
	if err != nil {
		return nil, err
	}

	existingSts := &appsv1.StatefulSet{}
	if err = r.Get(ctx, types.NamespacedName{Name: stsName, Namespace: etcd.Namespace}, existingSts); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		if err := r.Create(ctx, expectedSts); err != nil {
			return nil, err
		}
		return expectedSts, nil
	}

	// Statefulset is claimed by for this etcd. Just sync the specs
	sts, err := r.syncStatefulSetSpec(ctx, etcd, existingSts, expectedSts)
	if err != nil {
		return nil, err
	}

	// restart etcd pods in crashloop backoff
	selector, err := metav1.LabelSelectorAsSelector(sts.Spec.Selector)
	if err != nil {
		logger.Error(err, "error converting statefulset selector to selector")
		return nil, err
	}
	podList := &v1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}

	for _, pod := range podList.Items {
		if utils.IsPodInCrashloopBackoff(pod.Status) {
			if err := r.Delete(ctx, &pod); err != nil {
				logger.Error(err, fmt.Sprintf("error deleting etcd pod in crashloop: %s/%s", pod.Namespace, pod.Name))
				return nil, err
			}
		}
	}
	return sts, nil
}

func getContainerMapFromPodTemplateSpec(spec v1.PodSpec) map[string]v1.Container {
	containers := map[string]v1.Container{}
	for _, c := range spec.Containers {
		containers[c.Name] = c
	}
	return containers
}

func (r *Reconciler) syncStatefulSetSpec(ctx context.Context, etcd *druidv1alpha1.Etcd, existingSS, expectedSS *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	if reflect.DeepEqual(existingSS.Spec, expectedSS.Spec) {
		return existingSS, nil
	}

	ssCopy := existingSS.DeepCopy()
	ssCopy.Spec.Replicas = expectedSS.Spec.Replicas
	ssCopy.Spec.UpdateStrategy = expectedSS.Spec.UpdateStrategy

	recreateSTS := false
	if !reflect.DeepEqual(ssCopy.Spec.Selector, expectedSS.Spec.Selector) {
		recreateSTS = true
	}

	// Applying suggestions from
	containers := getContainerMapFromPodTemplateSpec(ssCopy.Spec.Template.Spec)
	for i, c := range expectedSS.Spec.Template.Spec.Containers {
		container, ok := containers[c.Name]
		if !ok {
			return nil, fmt.Errorf("container with name %s could not be fetched from statefulset %s", c.Name, expectedSS.Name)
		}
		expectedSS.Spec.Template.Spec.Containers[i].Resources = container.Resources
	}

	ssCopy.Spec.Template = expectedSS.Spec.Template

	var err error
	if recreateSTS {
		logger.Infof("selector changed, recreating statefulset: %s", ssCopy.Name)
		err = r.recreateStatefulset(ctx, expectedSS)
	} else {
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			return r.Patch(ctx, ssCopy, client.MergeFrom(existingSS))
		})
	}

	// Ignore the precondition violated error, this machine is already updated
	// with the desired label.
	if err == errorsutil.ErrPreconditionViolated {
		logger.Infof("statefulset %s precondition doesn't hold, skip updating it", existingSS.Name)
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return ssCopy, err
}

func (r *Reconciler) recreateStatefulset(ctx context.Context, ss *appsv1.StatefulSet) error {
	skipDelete := false
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if !skipDelete {
			if err := r.Delete(ctx, ss); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
		skipDelete = true
		return r.Create(ctx, ss)
	})
	return err
}

func (r *Reconciler) reconcileEtcd(ctx context.Context, etcd *druidv1alpha1.Etcd) (*corev1.Service, *appsv1.StatefulSet, error) {
	externalServiceName, err := r.determineExternalServiceName(ctx, etcd)
	if err != nil {
		return nil, nil, err
	}

	externalService, err := r.reconcileExternalService(ctx, etcd, externalServiceName)
	if err != nil {
		return nil, nil, err
	}

	internalServiceName, err := r.determineInternalServiceName(ctx, etcd)
	if err != nil {
		return nil, nil, err
	}

	configmapName, err := r.determineConfigmapName(ctx, etcd)
	if err != nil {
		return nil, nil, err
	}

	statefulSetName, err := r.determineStatefulSetName(ctx, etcd)
	if err != nil {
		return nil, nil, err
	}

	values, err := r.getMapFromEtcd(etcd, internalServiceName)
	if err != nil {
		return nil, nil, err
	}
	values["serviceName"] = internalServiceName
	values["configmapName"] = configmapName
	values["statefulsetName"] = statefulSetName

	chartPath := getChartPath()
	renderedChart, err := r.chartApplier.Render(chartPath, etcd.Name, etcd.Namespace, values)
	if err != nil {
		return nil, nil, err
	}

	if _, err := r.reconcileInternalService(ctx, etcd, internalServiceName, renderedChart); err != nil {
		return nil, nil, err
	}

	if _, err := r.reconcileConfigMap(ctx, etcd, configmapName, renderedChart); err != nil {
		return nil, nil, err
	}

	sts, err := r.reconcileStatefulSet(ctx, etcd, statefulSetName, renderedChart)
	if err != nil {
		return nil, nil, err
	}

	sts, err = r.waitUntilStatefulSetReady(ctx, sts)
	if err != nil {
		return nil, nil, err
	}

	return externalService, sts, nil
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

func (r *Reconciler) getMapFromEtcd(etcd *druidv1alpha1.Etcd, serviceName string) (map[string]interface{}, error) {
	var (
		images map[string]*imagevector.Image
		err    error
	)

	imageNames := []string{
		common.Etcd,
		common.BackupRestore,
	}

	if etcd.Spec.Etcd.Image == nil || etcd.Spec.Backup.Image == nil {

		images, err = imagevector.FindImages(r.ImageVector, imageNames)
		if err != nil {
			return map[string]interface{}{}, err
		}
	}

	nodes := etcd.GetCompletedEtcdNodes()
	initialCluster := ""
	for i := int32(0); i < nodes; i++ {
		initialCluster = fmt.Sprintf("%s,%s-%d=http://%s-%d.%s:%d", initialCluster, etcd.Name, i, etcd.Name, i, serviceName, etcd.GetCompletedEtcdServerPort())
	}
	etcdValues := map[string]interface{}{
		"replicas":                nodes,
		"defragmentationSchedule": etcd.Spec.Etcd.DefragmentationSchedule,
		"enableTLS":               (etcd.Spec.Etcd.TLS != nil),
		"pullPolicy":              corev1.PullIfNotPresent,
		"initialCluster":          initialCluster,
		// "username":                etcd.Spec.Etcd.Username,
		// "password":                etcd.Spec.Etcd.Password,
	}

	if etcd.Spec.Etcd.Resources != nil {
		etcdValues["resources"] = etcd.Spec.Etcd.Resources
	}

	if etcd.Spec.Etcd.Metrics != nil {
		etcdValues["metrics"] = etcd.Spec.Etcd.Metrics
	}

	if etcd.Spec.Etcd.ServerPort != nil {
		etcdValues["serverPort"] = etcd.Spec.Etcd.ServerPort
	}

	if etcd.Spec.Etcd.ClientPort != nil {
		etcdValues["clientPort"] = etcd.Spec.Etcd.ClientPort
	}

	if etcd.Spec.Etcd.Image == nil {
		val, ok := images[common.Etcd]
		if !ok {
			return map[string]interface{}{}, fmt.Errorf("either etcd resource or image vector should have %s image", common.Etcd)
		}
		etcdValues["image"] = val.String()
	} else {
		etcdValues["image"] = etcd.Spec.Etcd.Image
	}

	var quota int64 = 8 * 1024 * 1024 * 1024 // 8Gi
	if etcd.Spec.Etcd.Quota != nil {
		quota = etcd.Spec.Etcd.Quota.Value()
	}

	var deltaSnapshotMemoryLimit int64 = 100 * 1024 * 1024 // 100Mi
	if etcd.Spec.Backup.DeltaSnapshotMemoryLimit != nil {
		deltaSnapshotMemoryLimit = etcd.Spec.Backup.DeltaSnapshotMemoryLimit.Value()
	}

	backupValues := map[string]interface{}{
		"pullPolicy":               corev1.PullIfNotPresent,
		"etcdQuotaBytes":           quota,
		"etcdConnectionTimeout":    "5m",
		"snapstoreTempDir":         "/var/etcd/data/temp",
		"deltaSnapshotMemoryLimit": deltaSnapshotMemoryLimit,
	}

	if etcd.Spec.Backup.Resources != nil {
		backupValues["resources"] = etcd.Spec.Backup.Resources
	}

	if etcd.Spec.Backup.FullSnapshotSchedule != nil {
		backupValues["fullSnapshotSchedule"] = etcd.Spec.Backup.FullSnapshotSchedule
	}

	if etcd.Spec.Backup.GarbageCollectionPolicy != nil {
		backupValues["garbageCollectionPolicy"] = etcd.Spec.Backup.GarbageCollectionPolicy
	}

	if etcd.Spec.Backup.GarbageCollectionPeriod != nil {
		backupValues["garbageCollectionPeriod"] = etcd.Spec.Backup.GarbageCollectionPeriod
	}

	if etcd.Spec.Backup.DeltaSnapshotPeriod != nil {
		backupValues["deltaSnapshotPeriod"] = etcd.Spec.Backup.DeltaSnapshotPeriod
	}

	if etcd.Spec.Backup.Port != nil {
		backupValues["port"] = etcd.Spec.Backup.Port
	}

	if etcd.Spec.Backup.Image == nil {
		val, ok := images[common.BackupRestore]
		if !ok {
			return map[string]interface{}{}, fmt.Errorf("either etcd resource or image vector should have %s image", common.BackupRestore)
		}
		backupValues["image"] = val.String()
	} else {
		backupValues["image"] = etcd.Spec.Backup.Image
	}

	volumeClaimTemplateName := etcd.Name
	if etcd.Spec.VolumeClaimTemplate != nil && len(*etcd.Spec.VolumeClaimTemplate) != 0 {
		volumeClaimTemplateName = *etcd.Spec.VolumeClaimTemplate
	}

	values := map[string]interface{}{
		"name":                    etcd.Name,
		"uid":                     etcd.UID,
		"selector":                etcd.Spec.Selector,
		"labels":                  etcd.Spec.Labels,
		"annotations":             etcd.Spec.Annotations,
		"etcd":                    etcdValues,
		"backup":                  backupValues,
		"volumeClaimTemplateName": volumeClaimTemplateName,
	}

	if etcd.Spec.StorageCapacity != nil {
		values["storageCapacity"] = etcd.Spec.StorageCapacity
	}

	if etcd.Spec.StorageClass != nil {
		values["storageClass"] = etcd.Spec.StorageClass
	}

	if etcd.Spec.PriorityClassName != nil {
		values["priorityClassName"] = *etcd.Spec.PriorityClassName
	}

	if etcd.Spec.Etcd.TLS != nil {
		values["tlsServerSecret"] = etcd.Spec.Etcd.TLS.ServerTLSSecretRef.Name
		values["tlsClientSecret"] = etcd.Spec.Etcd.TLS.ClientTLSSecretRef.Name
		values["tlsCASecret"] = etcd.Spec.Etcd.TLS.TLSCASecretRef.Name
	}

	if etcd.Spec.Backup.Store != nil {
		storageProvider, err := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
		if err != nil {
			return nil, err
		}
		storeValues := map[string]interface{}{
			"storePrefix":     etcd.Spec.Backup.Store.Prefix,
			"storageProvider": storageProvider,
		}
		if etcd.Spec.Backup.Store.Container != nil {
			storeValues["storageContainer"] = etcd.Spec.Backup.Store.Container
		}
		if etcd.Spec.Backup.Store.SecretRef != nil {
			storeValues["storeSecret"] = etcd.Spec.Backup.Store.SecretRef.Name
		}

		values["store"] = storeValues
	}

	return values, nil
}

func (r *Reconciler) addFinalizersToDependantSecrets(ctx context.Context, etcd *druidv1alpha1.Etcd) error {

	secrets := []*corev1.SecretReference{}
	if etcd.Spec.Etcd.TLS != nil {
		// As the secrets inside TLS field are required, we error in case they are not found.
		secrets = append(secrets,
			&etcd.Spec.Etcd.TLS.ClientTLSSecretRef,
			&etcd.Spec.Etcd.TLS.ServerTLSSecretRef,
			&etcd.Spec.Etcd.TLS.TLSCASecretRef,
		)
	}
	if etcd.Spec.Backup.Store != nil && etcd.Spec.Backup.Store.SecretRef != nil {
		// As the store secret is required, we error in case it is not found as well.
		secrets = append(secrets, etcd.Spec.Backup.Store.SecretRef)
	}

	for _, secretRef := range secrets {
		secret := &corev1.Secret{}
		if err := r.Client.Get(ctx, types.NamespacedName{
			Name:      secretRef.Name,
			Namespace: etcd.Namespace,
		}, secret); err != nil {
			return err
		}
		if finalizers := sets.NewString(secret.Finalizers...); !finalizers.Has(FinalizerName) {
			logger.Infof("Adding finalizer (%s) for secret %s by etcd (%s)", FinalizerName, secret.GetName(), etcd.Name)
			finalizers.Insert(FinalizerName)
			secret.Finalizers = finalizers.UnsortedList()
			if err := r.Update(ctx, secret); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) updateEtcdErrorStatus(etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet, lastError error) error {
	lastErrStr := fmt.Sprintf("%v", lastError)
	etcd.Status.LastError = &lastErrStr
	etcd.Status.ObservedGeneration = &etcd.Generation
	if sts != nil {
		ready := health.CheckStatefulSet(sts) == nil
		etcd.Status.Ready = &ready
	}

	if err := r.Status().Update(context.TODO(), etcd); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return r.removeOperationAnnotation(etcd)
}

func (r *Reconciler) updateEtcdStatus(etcd *druidv1alpha1.Etcd, svc *corev1.Service, sts *appsv1.StatefulSet) error {

	svcName := svc.Name
	etcd.Status.Etcd = druidv1alpha1.CrossVersionObjectReference{
		APIVersion: sts.APIVersion,
		Kind:       sts.Kind,
		Name:       sts.Name,
	}
	ready := health.CheckStatefulSet(sts) == nil
	conditions := []druidv1alpha1.Condition{}
	for _, condition := range sts.Status.Conditions {
		conditions = append(conditions, convertConditionsToEtcd(&condition))
	}
	etcd.Status.Conditions = conditions

	// To be changed once we have multiple replicas.
	etcd.Status.CurrentReplicas = sts.Status.CurrentReplicas
	etcd.Status.ReadyReplicas = sts.Status.ReadyReplicas
	etcd.Status.UpdatedReplicas = sts.Status.UpdatedReplicas
	etcd.Status.ServiceName = &svcName
	etcd.Status.LastError = nil
	etcd.Status.ObservedGeneration = &etcd.Generation
	etcd.Status.Ready = &ready

	if err := r.Status().Update(context.TODO(), etcd); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return r.removeOperationAnnotation(etcd)
}

func (r *Reconciler) waitUntilStatefulSetReady(ctx context.Context, sts *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	ss := &appsv1.StatefulSet{}
	err := gardenerretry.UntilTimeout(ctx, DefaultInterval, DefaultTimeout, func(ctx context.Context) (bool, error) {
		if err := r.Get(ctx, types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}, ss); err != nil {
			if errors.IsNotFound(err) {
				return gardenerretry.MinorError(err)
			}
			return gardenerretry.SevereError(err)
		}
		if err := health.CheckStatefulSet(ss); err != nil {
			return gardenerretry.MinorError(err)
		}
		return gardenerretry.Ok()
	})
	return ss, err
}

func (r *Reconciler) removeOperationAnnotation(etcd *druidv1alpha1.Etcd) error {
	if _, ok := etcd.Annotations[v1beta1constants.GardenerOperation]; ok {
		delete(etcd.Annotations, v1beta1constants.GardenerOperation)
		return r.Update(context.TODO(), etcd)
	}
	return nil
}

func (r *Reconciler) updateEtcdStatusAsNotReady(etcd *druidv1alpha1.Etcd) (*druidv1alpha1.Etcd, error) {
	etcdCopy := etcd.DeepCopy()
	etcdCopy.Status.Ready = nil
	etcdCopy.Status.ReadyReplicas = 0
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Status().Patch(context.TODO(), etcdCopy, client.MergeFrom(etcd))
	})
	return etcdCopy, err
}

func convertConditionsToEtcd(condition *appsv1.StatefulSetCondition) druidv1alpha1.Condition {
	return druidv1alpha1.Condition{
		Type:               druidv1alpha1.ConditionType(condition.Type),
		Status:             druidv1alpha1.ConditionStatus(condition.Status),
		LastTransitionTime: condition.LastTransitionTime,
		Reason:             condition.Reason,
		Message:            condition.Message,
	}
}

func (r *Reconciler) claimStatefulSets(etcd *druidv1alpha1.Etcd, selector labels.Selector, ss *appsv1.StatefulSetList) ([]*appsv1.StatefulSet, error) {
	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Machines (see #42639).
	canAdoptFunc := RecheckDeletionTimestamp(func() (metav1.Object, error) {
		foundEtcd := &druidv1alpha1.Etcd{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, foundEtcd)
		if err != nil {
			return nil, err
		}
		if foundEtcd.UID != etcd.UID {
			return nil, fmt.Errorf("original %v/%v hvpa gone: got uid %v, wanted %v", etcd.Namespace, etcd.Name, foundEtcd.UID, etcd.UID)
		}
		return foundEtcd, nil
	})
	cm := NewEtcdDruidRefManager(r, etcd, selector, etcdGVK, canAdoptFunc)
	return cm.ClaimStatefulsets(ss)
}

func (r *Reconciler) claimServices(etcd *druidv1alpha1.Etcd, selector labels.Selector, ss *corev1.ServiceList) ([]*corev1.Service, error) {
	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Machines (see #42639).
	canAdoptFunc := RecheckDeletionTimestamp(func() (metav1.Object, error) {
		foundEtcd := &druidv1alpha1.Etcd{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, foundEtcd)
		if err != nil {
			return nil, err
		}
		if foundEtcd.UID != etcd.UID {
			return nil, fmt.Errorf("original %v/%v hvpa gone: got uid %v, wanted %v", etcd.Namespace, etcd.Name, foundEtcd.UID, etcd.UID)
		}
		return foundEtcd, nil
	})
	cm := NewEtcdDruidRefManager(r, etcd, selector, etcdGVK, canAdoptFunc)
	return cm.ClaimServices(ss)
}

func (r *Reconciler) claimConfigMaps(etcd *druidv1alpha1.Etcd, selector labels.Selector, ss *corev1.ConfigMapList) ([]*corev1.ConfigMap, error) {
	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Machines (see #42639).
	canAdoptFunc := RecheckDeletionTimestamp(func() (metav1.Object, error) {
		foundEtcd := &druidv1alpha1.Etcd{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, foundEtcd)
		if err != nil {
			return nil, err
		}
		if foundEtcd.UID != etcd.UID {
			return nil, fmt.Errorf("original %v/%v hvpa gone: got uid %v, wanted %v", etcd.Namespace, etcd.Name, foundEtcd.UID, etcd.UID)
		}
		return foundEtcd, nil
	})
	cm := NewEtcdDruidRefManager(r, etcd, selector, etcdGVK, canAdoptFunc)
	return cm.ClaimConfigMaps(ss)
}

// SetupWithManager sets up manager with a new controller and r as the reconcile.Reconciler
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, workers int, ignoreOperationAnnotation bool) error {
	predicates := []predicate.Predicate{
		druidpredicates.GenerationChangedPredicate{},
		druidpredicates.LastOperationNotSuccessful(),
	}
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: workers,
	})
	if !ignoreOperationAnnotation {
		predicates = append(predicates, druidpredicates.HasOperationAnnotation())
	}
	builder = builder.WithEventFilter(druidpredicates.Or(predicates...)).For(&druidv1alpha1.Etcd{})
	if ignoreOperationAnnotation {
		logger.Infoln("Setting up owns handler")
		builder = builder.Owns(&v1.Service{}).
			Owns(&v1.ConfigMap{}).
			Owns(&appsv1.StatefulSet{})
	}
	return builder.Complete(r)
}
