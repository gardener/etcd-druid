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
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/chartrenderer"
	"github.com/gardener/etcd-druid/pkg/utils"

	kubernetes "github.com/gardener/etcd-druid/pkg/client/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	logger  = logrus.New()
	etcdGVK = druidv1alpha1.GroupVersion.WithKind("Etcd")
)

const (
	// FinalizerName is the name of the Plant finalizer.
	FinalizerName = "druid.gardener.cloud/etcd-druid"
	timeout       = time.Second * 30
)

// EtcdReconciler reconciles a Etcd object
type EtcdReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	chartApplier  kubernetes.ChartApplier
	Config        *rest.Config
	ChartApplier  kubernetes.ChartApplier
	RenderedChart *chartrenderer.RenderedChart
}

// NewEtcdReconciler creates a new EtcdReconciler object
func NewEtcdReconciler(mgr manager.Manager) (*EtcdReconciler, error) {
	return (&EtcdReconciler{
		Client: mgr.GetClient(),
		Config: mgr.GetConfig(),
		Scheme: mgr.GetScheme(),
	}).InitializeControllerWithChartApplier()
}

func getChartPath() string {
	return filepath.Join("charts", "etcd")
}

func getChartPathForStatefulSet() string {
	return filepath.Join("etcd", "templates", "etcd-statefulset.yaml")
}

func getChartPathForConfigMap() string {
	return filepath.Join("etcd", "templates", "configmap-etcd-bootstrap.yaml")
}

func getChartPathForService() string {
	return filepath.Join("etcd", "templates", "etcd-service.yaml")
}

func (r *EtcdReconciler) getImageYAMLPath() string {
	return filepath.Join("charts", "images.yaml")
}

// InitializeControllerWithChartApplier will use EtcdReconciler client to intialize a Kubernetes client as well as
// InitializeControllerWithChartApplier will use EtcdReconciler client to intialize a Kubernetes client as well as
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
	r.ChartApplier = kubernetes.NewChartApplier(renderer, applier)
	return r, nil
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;update;patch

// Reconcile reconciles the <req>.
func (r *EtcdReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {

	// your logic here
	etcd := &druidv1alpha1.Etcd{}
	if err := r.Get(context.TODO(), req.NamespacedName, etcd); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}
	etcdCopy := etcd.DeepCopy()
	// Update the found object and write the result back if there are any changes
	if !reflect.DeepEqual(etcd.Spec, etcdCopy.Spec) {
		etcdCopy.Spec = etcd.Spec
	}
	logger.Infof("Reconciling etcd: %s", etcd.GetName())
	if !etcdCopy.DeletionTimestamp.IsZero() {
		logger.Infof("Deletion timestamp set for etcd: %s", etcd.GetName())
		if err := r.removeFinalizersToDependantSecrets(etcdCopy); err != nil {
			if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Second * 5,
				}, nil
			}
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 5,
			}, err
		}

		if sets.NewString(etcd.Finalizers...).Has(FinalizerName) {
			logger.Infof("Removing finalizer (%s) from etcd %s", FinalizerName, etcd.GetName())
			finalizers := sets.NewString(etcdCopy.Finalizers...)
			finalizers.Delete(FinalizerName)
			etcdCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Patch(context.TODO(), etcdCopy, client.MergeFrom(etcd)); err != nil {
				if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
					return ctrl.Result{
						Requeue:      true,
						RequeueAfter: time.Second * 5,
					}, nil
				}
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Second * 5,
				}, err
			}
		}
		logger.Infof("Deleted etcd %s successfully.", etcd.GetName())
		return ctrl.Result{}, nil
	}

	// Add Finalizers to Etcd
	if finalizers := sets.NewString(etcd.Finalizers...); !finalizers.Has(FinalizerName) {
		logger.Infof("Adding finalizer (%s) to etcd %s", FinalizerName, etcd.GetName())
		finalizers.Insert(FinalizerName)
		etcdCopy.Finalizers = finalizers.UnsortedList()
		if err := r.Patch(context.TODO(), etcdCopy, client.MergeFrom(etcd)); err != nil {
			if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Second * 5,
				}, err
			}
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 5,
			}, err
		}
	}
	if err := r.addFinalizersToDependantSecrets(etcdCopy); err != nil {
		if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 5,
			}, err
		}
	}

	svc, ss, err := r.reconcileEtcd(etcdCopy)
	if err != nil {
		if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 5,
			}, err
		}
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Second * 5,
		}, err
	}

	if err := r.updateEtcdStatus(etcdCopy, etcd, svc, ss); err != nil {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Second * 5,
		}, err
	}

	logger.Infof("Successfully reconciled etcd: %s", etcd.GetName())

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Minute * 5,
	}, nil
}

func (r *EtcdReconciler) reconcileServices(etcd *druidv1alpha1.Etcd) (*corev1.Service, error) {
	logger.Infof("Reconciling etcd services for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)

	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		logger.Error(err, "Error converting etcd selector to selector")
		return nil, err
	}

	// list all services to include the services that don't match the etcd`s selector
	// anymore but has the stale controller ref.
	services := &corev1.ServiceList{}
	err = r.List(context.TODO(), services, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		logger.Error(err, "Error listing statefulsets")
		return nil, err
	}
	for _, s := range services.Items {
		logger.Infof("Services: %s\n", s.Name)
	}

	// NOTE: filteredStatefulSets are pointing to deepcopies of the cache, but this could change in the future.
	// Ref: https://github.com/kubernetes-sigs/controller-runtime/blob/release-0.2/pkg/cache/internal/cache_reader.go#L74
	// if you need to modify them, you need to copy it first.
	filteredServices, err := r.claimServices(etcd, selector, services)
	if err != nil {
		logger.Error(err, "Error claiming service")
		return nil, err
	}

	if len(filteredServices) > 0 {
		// TODO: Sync spec and delete OR First delete and then sync spec?

		// Keep only 1 Service. Delete the rest
		for i := 1; i < len(filteredServices); i++ {
			ss := filteredServices[i]
			if err := r.Delete(context.TODO(), ss); err != nil {
				logger.Error(err, "Error in deleting duplicate StatefulSet")
				continue
			}
		}

		// Return the updated Service
		service := &corev1.Service{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: filteredServices[0].Name, Namespace: filteredServices[0].Namespace}, service)
		if err != nil {
			return nil, err
		}

		// Statefulset is claimed by for this etcd. Just sync the specs
		if service, err = r.syncServiceSpec(service, etcd); err != nil {
			return nil, err
		}
		return service, err
	}

	// Required Service doesn't exist. Create new

	ss, err := r.getServiceFromEtcd(etcd)
	if err != nil {
		return nil, err
	}

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Create(context.TODO(), ss)
	})

	// Ignore the precondition violated error, this service is already updated
	// with the desired label.
	if err == errorsutil.ErrPreconditionViolated {
		logger.Infof("Service %s precondition doesn't hold, skip updating it.", ss.Name)
		err = nil
	}

	if err != nil {
		return nil, err
	}

	if err := controllerutil.SetControllerReference(etcd, ss, r.Scheme); err != nil {
		return nil, err
	}

	return ss.DeepCopy(), err
}

func (r *EtcdReconciler) syncServiceSpec(ss *corev1.Service, etcd *druidv1alpha1.Etcd) (*corev1.Service, error) {
	decoded, err := r.getServiceFromEtcd(etcd)
	if err != nil {
		return nil, err
	}

	if reflect.DeepEqual(ss.Spec, decoded.Spec) {
		return ss, nil
	}
	ssCopy := ss.DeepCopy()
	decoded.Spec.DeepCopyInto(&ssCopy.Spec)
	// Copy ClusterIP as the field is immutable
	ssCopy.Spec.ClusterIP = ss.Spec.ClusterIP

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Patch(context.TODO(), ssCopy, client.MergeFrom(ss))
	})

	// Ignore the precondition violated error, this machine is already updated
	// with the desired label.
	if err == errorsutil.ErrPreconditionViolated {
		logger.Infof("Service %s precondition doesn't hold, skip updating it.", ss.Name)
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return ssCopy, err
}

func (r *EtcdReconciler) getServiceFromEtcd(etcd *druidv1alpha1.Etcd) (*corev1.Service, error) {
	var err error
	decoded := &corev1.Service{}
	servicePath := getChartPathForService()
	if _, ok := r.RenderedChart.Files()[servicePath]; !ok {
		return nil, fmt.Errorf("missing service template file in the charts: %v", servicePath)
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(r.RenderedChart.Files()[servicePath])), 1024)

	if err = decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func (r *EtcdReconciler) reconcileConfigMaps(etcd *druidv1alpha1.Etcd) (*corev1.ConfigMap, error) {
	logger.Infof("Reconciling etcd configmap for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)

	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		logger.Error(err, "Error converting etcd selector to selector")
		return nil, err
	}

	// list all configmaps to include the configmaps that don't match the etcd`s selector
	// anymore but has the stale controller ref.
	cms := &corev1.ConfigMapList{}
	err = r.List(context.TODO(), cms, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		logger.Error(err, "Error listing statefulsets")
		return nil, err
	}

	logger.Infof("Configmaps: %d\n", len(cms.Items))

	// NOTE: filteredStatefulSets are pointing to deepcopies of the cache, but this could change in the future.
	// Ref: https://github.com/kubernetes-sigs/controller-runtime/blob/release-0.2/pkg/cache/internal/cache_reader.go#L74
	// if you need to modify them, you need to copy it first.
	filteredCMs, err := r.claimConfigMaps(etcd, selector, cms)
	if err != nil {
		return nil, err
	}

	if len(filteredCMs) > 0 {
		// TODO: Sync spec and delete OR First delete and then sync spec?

		// Keep only 1 Configmap. Delete the rest
		for i := 1; i < len(filteredCMs); i++ {
			ss := filteredCMs[i]
			if err := r.Delete(context.TODO(), ss); err != nil {
				logger.Error(err, "Error in deleting duplicate StatefulSet")
				continue
			}
		}

		// Return the updated Configmap
		cm := &corev1.ConfigMap{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: filteredCMs[0].Name, Namespace: filteredCMs[0].Namespace}, cm)
		if err != nil {
			return nil, err
		}
		// ConfigMap is claimed by for this etcd. Just sync the data
		if cm, err = r.syncConfigMapData(cm, etcd); err != nil {
			return nil, err
		}
		return cm, err
	}

	// Required Configmap doesn't exist. Create new

	cm, err := r.getConfigMapFromEtcd(etcd)
	if err != nil {
		return nil, err
	}

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Create(context.TODO(), cm)
	})

	// Ignore the precondition violated error, this machine is already updated
	// with the desired label.
	if err == errorsutil.ErrPreconditionViolated {
		logger.Infof("Service %s precondition doesn't hold, skip updating it.", cm.Name)
		err = nil
	}

	if err != nil {
		return nil, err
	}

	if err := controllerutil.SetControllerReference(etcd, cm, r.Scheme); err != nil {
		return nil, err
	}

	return cm.DeepCopy(), err
}

func (r *EtcdReconciler) syncConfigMapData(cm *corev1.ConfigMap, etcd *druidv1alpha1.Etcd) (*corev1.ConfigMap, error) {
	decoded, err := r.getConfigMapFromEtcd(etcd)
	if err != nil {
		return nil, err
	}

	if reflect.DeepEqual(cm.Data, decoded.Data) {
		return cm, nil
	}
	cmCopy := cm.DeepCopy()
	cmCopy.Data = decoded.Data

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Patch(context.TODO(), cmCopy, client.MergeFrom(cm))
	})

	// Ignore the precondition violated error, this machine is already updated
	// with the desired label.
	if err == errorsutil.ErrPreconditionViolated {
		logger.Infof("Service %s precondition doesn't hold, skip updating it.", cm.Name)
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return cmCopy, err

}

func (r *EtcdReconciler) getConfigMapFromEtcd(etcd *druidv1alpha1.Etcd) (*corev1.ConfigMap, error) {
	var err error
	decoded := &corev1.ConfigMap{}
	configMapPath := getChartPathForConfigMap()
	if _, ok := r.RenderedChart.Files()[configMapPath]; !ok {
		return nil, fmt.Errorf("missing configmap template file in the charts: %v", configMapPath)
	}
	//logger.Infof("%v: %v", statefulsetPath, renderer.Files()[statefulsetPath])
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(r.RenderedChart.Files()[configMapPath])), 1024)

	if err = decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func (r *EtcdReconciler) reconcileStatefulSet(cm *corev1.ConfigMap, svc *corev1.Service, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, error) {
	logger.Infof("Reconciling etcd statefulset for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)
	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		logger.Error(err, "Error converting etcd selector to selector")
		return nil, err
	}

	// list all statefulsets to include the statefulsets that don't match the etcd`s selector
	// anymore but has the stale controller ref.
	statefulSets := &appsv1.StatefulSetList{}
	err = r.List(context.TODO(), statefulSets, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		logger.Error(err, "Error listing statefulsets")
		return nil, err
	}

	// NOTE: filteredStatefulSets are pointing to deepcopies of the cache, but this could change in the future.
	// Ref: https://github.com/kubernetes-sigs/controller-runtime/blob/release-0.2/pkg/cache/internal/cache_reader.go#L74
	// if you need to modify them, you need to copy it first.
	filteredStatefulSets, err := r.claimStatefulSets(etcd, selector, statefulSets)
	if err != nil {
		return nil, err
	}

	if len(filteredStatefulSets) > 0 {
		// TODO: Sync spec and delete OR First delete and then sync spec?

		// Keep only 1 statefulset. Delete the rest
		for i := 1; i < len(filteredStatefulSets); i++ {
			ss := filteredStatefulSets[i]
			if err := r.Delete(context.TODO(), ss); err != nil {
				logger.Error(err, "Error in deleting duplicate StatefulSet")
				continue
			}
		}

		// Return the updated statefulset
		ss := &appsv1.StatefulSet{}
		err = r.Get(context.TODO(), types.NamespacedName{Name: filteredStatefulSets[0].Name, Namespace: filteredStatefulSets[0].Namespace}, ss)
		// Statefulset is claimed by for this etcd. Just sync the specs
		if ss, err = r.syncStatefulSetSpec(ss, cm, svc, etcd); err != nil {
			return nil, err
		}
		return ss.DeepCopy(), err
	}

	// Required statefulset doesn't exist. Create new
	ss, err := r.getStatefulSetFromEtcd(etcd, cm, svc)
	if err != nil {
		return nil, err
	}

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Create(context.TODO(), ss)
	})

	// Ignore the precondition violated error, this machine is already updated
	// with the desired label.
	if err == errorsutil.ErrPreconditionViolated {
		logger.Infof("Statefulset %s precondition doesn't hold, skip updating it.", ss.Name)
		err = nil
	}
	if err != nil {
		return nil, err
	}

	if err := controllerutil.SetControllerReference(etcd, ss, r.Scheme); err != nil {
		return nil, err
	}
	logger.Info("Deployed etcd statefulset.")
	return ss.DeepCopy(), err
}

func (r *EtcdReconciler) syncStatefulSetSpec(ss *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, error) {
	decoded, err := r.getStatefulSetFromEtcd(etcd, cm, svc)
	if err != nil {
		return nil, err
	}

	if reflect.DeepEqual(ss.Spec, decoded.Spec) {
		return ss, nil
	}
	ssCopy := ss.DeepCopy()
	ssCopy.Spec.Replicas = decoded.Spec.Replicas
	ssCopy.Spec.UpdateStrategy = decoded.Spec.UpdateStrategy
	ssCopy.Spec.Template = decoded.Spec.Template

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Patch(context.TODO(), ssCopy, client.MergeFrom(ss))
	})

	// Ignore the precondition violated error, this machine is already updated
	// with the desired label.
	if err == errorsutil.ErrPreconditionViolated {
		logger.Infof("Statefulset %s precondition doesn't hold, skip updating it.", ss.Name)
		err = nil
	}
	if err != nil {
		logger.Infof("Patching statefulset failed for %s.", ss.Name)
		return nil, err
	}
	return ssCopy, err
}

func (r *EtcdReconciler) getStatefulSetFromEtcd(etcd *druidv1alpha1.Etcd, cm *corev1.ConfigMap, svc *corev1.Service) (*appsv1.StatefulSet, error) {
	var err error
	decoded := &appsv1.StatefulSet{}
	statefulSetPath := getChartPathForStatefulSet()
	if _, ok := r.RenderedChart.Files()[statefulSetPath]; !ok {
		return nil, fmt.Errorf("missing configmap template file in the charts: %v", statefulSetPath)
	}
	//logger.Infof("%v: %v", statefulsetPath, renderer.Files()[statefulsetPath])
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(r.RenderedChart.Files()[statefulSetPath])), 1024)

	if err = decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func (r *EtcdReconciler) reconcileEtcd(etcd *druidv1alpha1.Etcd) (*corev1.Service, *appsv1.StatefulSet, error) {

	values, err := r.getMapFromEtcd(etcd)
	if err != nil {
		return nil, nil, err
	}
	chartPath := getChartPath()
	renderer, err := r.ChartApplier.Render(chartPath, etcd.Name, etcd.Namespace, values)
	if err != nil {
		return nil, nil, err
	}
	r.RenderedChart = renderer

	svc, err := r.reconcileServices(etcd)
	if err != nil {
		return nil, nil, err
	}
	if svc != nil {
		values["serviceName"] = svc.Name
	}
	cm, err := r.reconcileConfigMaps(etcd)
	if err != nil {
		return nil, nil, err
	}

	if cm != nil {
		values["configMapName"] = cm.Name
	}
	//Re-render the chart with updated service and configmap name.
	renderer, err = r.ChartApplier.Render(chartPath, etcd.Name, etcd.Namespace, values)
	if err != nil {
		return nil, nil, err
	}

	r.RenderedChart = renderer

	ss, err := r.reconcileStatefulSet(cm, svc, etcd)
	if err != nil {
		return nil, nil, err
	}

	return svc, ss, nil
}

func checkForEtcdOwnerReference(refs []metav1.OwnerReference, etcd *druidv1alpha1.Etcd) bool {
	for _, ownerRef := range refs {
		if ownerRef.UID == etcd.UID {
			return true
		}
	}
	return false
}

func (r *EtcdReconciler) getMapFromEtcd(etcd *druidv1alpha1.Etcd) (map[string]interface{}, error) {
	var statefulsetReplicas int
	if etcd.Spec.Replicas != 0 {
		statefulsetReplicas = 1
	}

	etcdValues := map[string]interface{}{
		"defragmentationSchedule": etcd.Spec.Etcd.DefragmentationSchedule,
		"serverPort":              etcd.Spec.Etcd.ServerPort,
		"clientPort":              etcd.Spec.Etcd.ClientPort,
		"image":                   etcd.Spec.Etcd.Image,
		"metrics":                 etcd.Spec.Etcd.Metrics,
		"resources":               etcd.Spec.Etcd.Resources,
		"enableTLS":               (etcd.Spec.Etcd.TLS != nil),
		"pullPolicy":              corev1.PullIfNotPresent,
		// "username":                etcd.Spec.Etcd.Username,
		// "password":                etcd.Spec.Etcd.Password,
	}

	var quota int64 = 2 * 1024 * 1024 * 1024 // 2Gib
	if etcd.Spec.Etcd.Quota != nil {
		quota = etcd.Spec.Etcd.Quota.Value()
	}
	backupValues := map[string]interface{}{
		"image":                    etcd.Spec.Backup.Image,
		"fullSnapshotSchedule":     etcd.Spec.Backup.FullSnapshotSchedule,
		"port":                     etcd.Spec.Backup.Port,
		"resources":                etcd.Spec.Backup.Resources,
		"pullPolicy":               corev1.PullIfNotPresent,
		"garbageCollectionPolicy":  etcd.Spec.Backup.GarbageCollectionPolicy,
		"etcdQuotaBytes":           quota,
		"etcdConnectionTimeout":    "30s",
		"snapstoreTempDir":         "/tmp",
		"garbageCollectionPeriod":  etcd.Spec.Backup.GarbageCollectionPeriod,
		"deltaSnapshotPeriod":      etcd.Spec.Backup.DeltaSnapshotPeriod,
		"deltaSnapshotMemoryLimit": etcd.Spec.Backup.DeltaSnapshotMemoryLimit,
	}

	values := map[string]interface{}{
		"etcd":                    etcdValues,
		"backup":                  backupValues,
		"name":                    etcd.Name,
		"replicas":                etcd.Spec.Replicas,
		"labels":                  etcd.Spec.Labels,
		"annotations":             etcd.Spec.Annotations,
		"storageClass":            etcd.Spec.StorageClass,
		"storageCapacity":         etcd.Spec.StorageCapacity,
		"uid":                     etcd.UID,
		"statefulsetReplicas":     statefulsetReplicas,
		"serviceName":             fmt.Sprintf("%s-client", etcd.Name),
		"configMapName":           fmt.Sprintf("etcd-bootstrap-%s", string(etcd.UID[:6])),
		"volumeClaimTemplateName": etcd.Spec.VolumeClaimTemplate,
		"selector":                etcd.Spec.Selector,
	}

	if etcd.Spec.Etcd.TLS != nil {
		values["tlsServerSecret"] = etcd.Spec.Etcd.TLS.ServerTLSSecretRef.Name
		values["tlsClientSecret"] = etcd.Spec.Etcd.TLS.ClientTLSSecretRef.Name
		values["tlsCASecret"] = etcd.Spec.Etcd.TLS.TLSCASecretRef.Name
	}

	if etcd.Spec.Backup.Store != nil {
		storeValues := map[string]interface{}{
			"storageContainer": etcd.Spec.Backup.Store.Container,
			"storePrefix":      etcd.Spec.Backup.Store.Prefix,
			"storageProvider":  utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider),
			"storeSecret":      etcd.Spec.Backup.Store.SecretRef.Name,
		}
		values["store"] = storeValues
	}

	return values, nil
}

func (r *EtcdReconciler) addFinalizersToDependantSecrets(etcd *druidv1alpha1.Etcd) error {
	if etcd.Spec.Backup.Store != nil {
		storeSecret := corev1.Secret{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Backup.Store.SecretRef.Name,
			Namespace: etcd.Namespace,
		}, &storeSecret)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			if finalizers := sets.NewString(storeSecret.Finalizers...); !finalizers.Has(FinalizerName) {
				logger.Infof("Adding finalizer (%s) for secret %s by etcd (%s)", FinalizerName, storeSecret.GetName(), etcd.Name)
				storeSecretCopy := storeSecret.DeepCopy()
				finalizers.Insert(FinalizerName)
				storeSecretCopy.Finalizers = finalizers.UnsortedList()
				if err := r.Update(context.TODO(), storeSecretCopy); err != nil {
					return err
				}
			}
		}
	}
	if etcd.Spec.Etcd.TLS != nil {
		clientSecret := corev1.Secret{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Etcd.TLS.ClientTLSSecretRef.Name,
			Namespace: etcd.Spec.Etcd.TLS.ClientTLSSecretRef.Namespace,
		}, &clientSecret)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			if finalizers := sets.NewString(clientSecret.Finalizers...); !finalizers.Has(FinalizerName) {
				logger.Infof("Adding finalizer (%s) for secret %s by etcd (%s)", FinalizerName, clientSecret.GetName(), etcd.Name)
				clientSecretCopy := clientSecret.DeepCopy()
				finalizers.Insert(FinalizerName)
				clientSecretCopy.Finalizers = finalizers.UnsortedList()
				if err := r.Update(context.TODO(), clientSecretCopy); err != nil {
					return err
				}
			}
		}

		serverSecret := corev1.Secret{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Etcd.TLS.ServerTLSSecretRef.Name,
			Namespace: etcd.Spec.Etcd.TLS.ServerTLSSecretRef.Namespace,
		}, &serverSecret)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			if finalizers := sets.NewString(serverSecret.Finalizers...); !finalizers.Has(FinalizerName) {
				logger.Infof("Adding finalizer (%s) for secret %s by etcd (%s)", FinalizerName, serverSecret.GetName(), etcd.Name)
				serverSecretCopy := serverSecret.DeepCopy()
				finalizers.Insert(FinalizerName)
				serverSecretCopy.Finalizers = finalizers.UnsortedList()
				if err := r.Update(context.TODO(), serverSecretCopy); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *EtcdReconciler) removeFinalizersToDependantSecrets(etcd *druidv1alpha1.Etcd) error {
	if etcd.Spec.Backup.Store != nil {
		storeSecret := corev1.Secret{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Backup.Store.SecretRef.Name,
			Namespace: etcd.Namespace,
		}, &storeSecret)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			if finalizers := sets.NewString(storeSecret.Finalizers...); finalizers.Has(FinalizerName) {
				logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, storeSecret.GetName())
				storeSecretCopy := storeSecret.DeepCopy()
				finalizers.Delete(FinalizerName)
				storeSecretCopy.Finalizers = finalizers.UnsortedList()
				if err := r.Update(context.TODO(), storeSecretCopy); err != nil {
					return err
				}
			}
		}
	}
	if etcd.Spec.Etcd.TLS != nil {
		clientSecret := corev1.Secret{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Etcd.TLS.ClientTLSSecretRef.Name,
			Namespace: etcd.Namespace,
		}, &clientSecret)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			if finalizers := sets.NewString(clientSecret.Finalizers...); finalizers.Has(FinalizerName) {
				logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, clientSecret.GetName())
				clientSecretCopy := clientSecret.DeepCopy()
				finalizers.Delete(FinalizerName)
				clientSecretCopy.Finalizers = finalizers.UnsortedList()
				if err := r.Update(context.TODO(), clientSecretCopy); err != nil {
					return err
				}
			}
		}
		serverSecret := corev1.Secret{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Etcd.TLS.ServerTLSSecretRef.Name,
			Namespace: etcd.Namespace,
		}, &serverSecret)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		if err == nil {
			if finalizers := sets.NewString(serverSecret.Finalizers...); finalizers.Has(FinalizerName) {
				logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, serverSecret.GetName())
				serverSecretCopy := serverSecret.DeepCopy()
				finalizers.Delete(FinalizerName)
				serverSecretCopy.Finalizers = finalizers.UnsortedList()
				if err := r.Update(context.TODO(), serverSecretCopy); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *EtcdReconciler) updateStatusFromServices(etcd *druidv1alpha1.Etcd, svc *corev1.Service) error {

	// Delete the statefulset
	endpoints := corev1.Endpoints{}
	req := types.NamespacedName{
		Name:      svc.Name,
		Namespace: etcd.Namespace,
	}
	err := r.Get(context.TODO(), req, &endpoints)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil
		}
		// Error reading the object - requeue the request.
		return err
	}
	etcd.Status.Endpoints = []corev1.Endpoints{endpoints}

	return nil
}

func (r *EtcdReconciler) updateEtcdErrorStatus(etcdCopy, etcd *druidv1alpha1.Etcd, lastError error) error {

	lastErrStr := fmt.Sprintf("%v", lastError)
	etcdCopy.Status.LastError = &lastErrStr
	if err := r.Status().Update(context.TODO(), etcdCopy); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil
		}
		// Error reading the object - requeue the request.
		return err
	}
	return nil
}

func (r *EtcdReconciler) updateEtcdStatus(etcdCopy, etcd *druidv1alpha1.Etcd, svc *corev1.Service, ss *appsv1.StatefulSet) error {
	svcName := svc.Name
	etcdCopy.Status.Etcd = druidv1alpha1.CrossVersionObjectReference{
		APIVersion: ss.APIVersion,
		Kind:       ss.Kind,
		Name:       ss.Name,
	}
	conditions := []druidv1alpha1.Condition{}
	for _, condition := range ss.Status.Conditions {
		conditions = append(conditions, convertConditionsToEtcd(&condition))
	}
	etcdCopy.Status.Conditions = conditions

	// To be changed once we have multiple replicas.
	etcdCopy.Status.CurrentReplicas = ss.Status.CurrentReplicas
	etcdCopy.Status.ReadyReplicas = ss.Status.ReadyReplicas
	etcdCopy.Status.UpdatedReplicas = ss.Status.UpdatedReplicas
	etcdCopy.Status.Ready = (ss.Status.ReadyReplicas == ss.Status.Replicas)
	etcdCopy.Status.ServiceName = &svcName
	if err := r.updateStatusFromServices(etcdCopy, svc); err != nil {
		return err
	}
	if err := r.Status().Update(context.TODO(), etcdCopy); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil
		}

		// Error reading the object - requeue the request.
		return err
	}
	return nil
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

// SetupWithManager sets up manager with a new controller and r as the reconcile.Reconciler
func (r *EtcdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&druidv1alpha1.Etcd{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}

func (r *EtcdReconciler) claimStatefulSets(etcd *druidv1alpha1.Etcd, selector labels.Selector, ss *appsv1.StatefulSetList) ([]*appsv1.StatefulSet, error) {
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

func (r *EtcdReconciler) claimServices(etcd *druidv1alpha1.Etcd, selector labels.Selector, ss *corev1.ServiceList) ([]*corev1.Service, error) {
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

func (r *EtcdReconciler) claimConfigMaps(etcd *druidv1alpha1.Etcd, selector labels.Selector, ss *corev1.ConfigMapList) ([]*corev1.ConfigMap, error) {
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
