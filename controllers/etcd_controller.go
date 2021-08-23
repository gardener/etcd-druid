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
	"sort"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/chartrenderer"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	druidpredicates "github.com/gardener/etcd-druid/pkg/predicate"
	"github.com/gardener/etcd-druid/pkg/utils"

	extensionspredicate "github.com/gardener/gardener/extensions/pkg/predicate"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	gardenerretry "github.com/gardener/gardener/pkg/utils/retry"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"

	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	// FinalizerName is the name of the Plant finalizer.
	FinalizerName = "druid.gardener.cloud/etcd-druid"
	// DefaultImageVector is a constant for the path to the default image vector file.
	DefaultImageVector = "images.yaml"
	// EtcdReady implies that etcd is ready
	EtcdReady = true
	// DefaultAutoCompactionRetention defines the default auto-compaction-retention length for etcd.
	DefaultAutoCompactionRetention = "30m"

	metadataFieldName = "metadata"
)

var (
	// DefaultInterval is the default interval for retry operations.
	DefaultInterval = 5 * time.Second
	// DefaultTimeout is the default timeout for retry operations.
	DefaultTimeout = 1 * time.Minute

	etcdGVK = druidv1alpha1.GroupVersion.WithKind("Etcd")

	// UncachedObjectList is a list of objects which should not be cached.
	UncachedObjectList = []client.Object{
		&corev1.Event{},
		&eventsv1beta1.Event{},
		&eventsv1.Event{},
	}

	gkStatefulSet = schema.GroupKind{Group: appsv1.GroupName, Kind: "StatefulSet"}
	gkService     = schema.GroupKind{Group: corev1.GroupName, Kind: "Service"}

	immutableFieldPathsForGroupKinds = map[schema.GroupKind][][]string{
		gkStatefulSet: [][]string{
			{"spec", "selector"},
			{"spec", "volumeClaimTemplates"},
		},
	}

	preserveFieldPathsForGroupKinds = map[schema.GroupKind][][]string{
		gkService: [][]string{
			{"spec", "clusterIP"},
		},
		schema.GroupKind{Group: batchv1beta1.GroupName, Kind: "Job"}: [][]string{
			{"spec", "selector"},
		},
		schema.GroupKind{Group: batchv1beta1.GroupName, Kind: "CronJob"}: [][]string{
			{"spec", "jobTemplate", "spec", "selector"},
		},
	}

	forceOverWriteFieldPaths = [][]string{
		{"metadata", "annotations"},
		{"metadata", "labels"},
		{"metadata", "ownerReferences"},
	}
)

// EtcdReconciler reconciles a Etcd object
type EtcdReconciler struct {
	client.Client
	Scheme                          *runtime.Scheme
	chartApplier                    kubernetes.ChartApplier
	Config                          *rest.Config
	ImageVector                     imagevector.ImageVector
	logger                          logr.Logger
	enableBackupCompactionJobTempFS bool
}

// NewReconcilerWithImageVector creates a new EtcdReconciler object with an image vector
func NewReconcilerWithImageVector(mgr manager.Manager) (*EtcdReconciler, error) {
	etcdReconciler, err := NewEtcdReconciler(mgr, false)
	if err != nil {
		return nil, err
	}
	return etcdReconciler.InitializeControllerWithImageVector()
}

// NewEtcdReconciler creates a new EtcdReconciler object
func NewEtcdReconciler(mgr manager.Manager, enableBackupCompactionJobTempFS bool) (*EtcdReconciler, error) {
	return NewEtcdReconcilerWithAllFields(
		mgr.GetClient(),
		mgr.GetScheme(),
		nil,
		mgr.GetConfig(),
		enableBackupCompactionJobTempFS,
		nil,
		log.Log.WithName("etcd-controller"),
	).InitializeControllerWithChartApplier()
}

// NewEtcdReconcilerWithImageVector creates a new EtcdReconciler object
func NewEtcdReconcilerWithImageVector(mgr manager.Manager, enableBackupCompactionJobTempFS bool) (*EtcdReconciler, error) {
	ec, err := NewEtcdReconciler(mgr, enableBackupCompactionJobTempFS)
	if err != nil {
		return nil, err
	}
	return ec.InitializeControllerWithImageVector()
}

// NewEtcdReconcilerWithAllFields creates a new EtcdReconciler object.
// It must be directly used only for testing purposes.
func NewEtcdReconcilerWithAllFields(
	c client.Client,
	sch *runtime.Scheme,
	chartApplier kubernetes.ChartApplier,
	config *rest.Config,
	enableBackupCompactionJobTempFS bool,
	iv imagevector.ImageVector,
	logger logr.Logger,
) *EtcdReconciler {
	return &EtcdReconciler{
		Client:                          c,
		Scheme:                          sch,
		chartApplier:                    chartApplier,
		Config:                          config,
		enableBackupCompactionJobTempFS: enableBackupCompactionJobTempFS,
		ImageVector:                     iv,
		logger:                          logger,
	}
}

func getChartPath() string {
	return filepath.Join("charts", "etcd")
}

func getChartPathForCronJob() string {
	return filepath.Join("etcd", "templates", "etcd-compaction-cronjob.yaml")
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

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;update;patch

// Reconcile reconciles the etcd.
func (r *EtcdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
	return r.reconcile(ctx, etcd)
}

func (r *EtcdReconciler) reconcile(ctx context.Context, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String(), "operation", "reconcile")
	logger.Info("Starting operation")

	// Add Finalizers to Etcd
	if finalizers := sets.NewString(etcd.Finalizers...); !finalizers.Has(FinalizerName) {
		logger.Info("Adding finalizer")
		finalizers.Insert(FinalizerName)
		etcd.Finalizers = finalizers.UnsortedList()
		if err := r.Update(ctx, etcd); err != nil {
			if err := r.updateEtcdErrorStatus(ctx, noOp, etcd, nil, err); err != nil {
				return ctrl.Result{
					Requeue: true,
				}, err
			}
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	}
	if err := r.addFinalizersToDependantSecrets(ctx, logger, etcd); err != nil {
		if err := r.updateEtcdErrorStatus(ctx, noOp, etcd, nil, err); err != nil {
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

	op, svc, sts, err := r.reconcileEtcd(ctx, logger, etcd)
	if err != nil {
		if err := r.updateEtcdErrorStatus(ctx, op, etcd, sts, err); err != nil {
			logger.Error(err, "Error during reconciling ETCD")
			return ctrl.Result{
				Requeue: true,
			}, err
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}
	if err := r.updateEtcdStatus(ctx, op, etcd, svc, sts); err != nil {
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

	if waitForStatefulSetCleanup, err := r.removeDependantStatefulset(ctx, logger, etcd); err != nil {
		if err = r.updateEtcdErrorStatus(ctx, deleteOp, etcd, nil, err); err != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	} else if waitForStatefulSetCleanup {
		return ctrl.Result{
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	if err := r.removeFinalizersToDependantSecrets(ctx, logger, etcd); err != nil {
		if err := r.updateEtcdErrorStatus(ctx, deleteOp, etcd, nil, err); err != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	if sets.NewString(etcd.Finalizers...).Has(FinalizerName) {
		logger.Info("Removing finalizer")

		// Deep copy of etcd resource required here to patch the object. Update call results in
		// StorageError. See also: https://github.com/kubernetes/kubernetes/issues/71139
		etcdCopy := etcd.DeepCopy()
		finalizers := sets.NewString(etcdCopy.Finalizers...)
		finalizers.Delete(FinalizerName)
		etcdCopy.Finalizers = finalizers.UnsortedList()
		if err := r.Patch(ctx, etcdCopy, client.MergeFrom(etcd)); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	}
	logger.Info("Deleted etcd successfully.")
	return ctrl.Result{}, nil
}

type operationResult string

const (
	bootstrapOp operationResult = "bootstrap"
	reconcileOp operationResult = "reconcile"
	deleteOp    operationResult = "delete"
	noOp        operationResult = "none"
)

func (r *EtcdReconciler) reconcileEtcd(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (operationResult, *corev1.Service, *appsv1.StatefulSet, error) {
	var (
		svc = &corev1.Service{}
		sts = &appsv1.StatefulSet{}
		op  = noOp
	)

	values, err := r.getMapFromEtcd(etcd)
	if err != nil {
		return op, svc, sts, err
	}

	chartPath := getChartPath()
	renderedChart, err := r.chartApplier.Render(chartPath, etcd.Name, etcd.Namespace, values)
	if err != nil {
		return op, svc, sts, err
	}

	renderedFiles := renderedChart.Files()
	fileNames := make([]string, 0, len(renderedFiles))
	for name := range renderedFiles {
		fileNames = append(fileNames, name)
	}

	sort.Strings(fileNames) // Ensure that the files are applied in a fixed order

	for _, fileName := range fileNames {
		var content = renderedFiles[fileName]

		if len(content) <= 0 && fileName == getChartPathForCronJob() {
			// No backup compaction schedule. Remove the cronjob if it exists.
			if err := deleteIfExists(
				ctx,
				r.Client,
				&batchv1beta1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      getCronJobName(etcd),
						Namespace: etcd.Namespace,
					},
				},
				client.PropagationPolicy(metav1.DeletePropagationForeground),
			); err != nil {
				return op, svc, sts, err
			}

			continue // Skip applying the empty rendered content
		}

		var (
			obj           = &unstructured.Unstructured{}
			renderedObj   = &unstructured.Unstructured{}
			decoder       = yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
			isStatefulSet = false
		)

		if err := decoder.Decode(renderedObj); err != nil {
			return op, svc, sts, err
		}

		gvk, err := apiutil.GVKForObject(renderedObj, r.Scheme)
		if err != nil {
			return op, svc, sts, err
		}

		gk := gvk.GroupKind()

		if gk == gkStatefulSet {
			isStatefulSet = true
		}

		renderedObj.DeepCopyInto(obj)

		result, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
			if immutableFieldPaths, ok := immutableFieldPathsForGroupKinds[gk]; ok {
				if err := validateImmutableFields(gk, obj, renderedObj, immutableFieldPaths); err != nil {
					return err
				}
			}

			preserveFieldPaths := preserveFieldPathsForGroupKinds[gk]
			preserveFieldPaths = append(preserveFieldPaths, []string{metadataFieldName})

			oldObj := &unstructured.Unstructured{}
			obj.DeepCopyInto(oldObj)

			renderedObj.DeepCopyInto(obj)

			/* TODO
			 * 1. Preserve resources in pod template
			 */
			if err := preserveFields(gk, oldObj, obj, preserveFieldPaths); err != nil {
				return err
			}

			if isStatefulSet {
				if err := preserveStatefulSetResources(r.Scheme, oldObj, obj); err != nil {
					return err
				}
			}

			return forceOverwriteFields(gk, renderedObj, obj, forceOverWriteFieldPaths)
		})

		if err != nil {
			if apierrors.IsConflict(err) {
				return op, svc, sts, err
			}

			if apierrors.IsInvalid(err) && result == controllerutil.OperationResultUpdated {
				if err = recreate(ctx, r.Client, obj, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
					return op, svc, sts, err
				}

				result = controllerutil.OperationResultCreated
			}
		}

		switch gk {
		case gkStatefulSet:
			if err := r.Scheme.Convert(obj, sts, nil); err != nil {
				return op, svc, sts, err
			}

		case gkService:
			if err := r.Scheme.Convert(obj, svc, nil); err != nil {
				return op, svc, sts, err
			}
		}

		if isStatefulSet {
			op, sts, err = r.processPostStatefulSetReconcile(ctx, logger, etcd, sts, result)
			if err != nil {
				return op, svc, sts, err
			}
		}
	}

	return op, svc, sts, nil
}

func (r *EtcdReconciler) processPostStatefulSetReconcile(
	ctx context.Context,
	logger logr.Logger,
	etcd *druidv1alpha1.Etcd,
	sts *appsv1.StatefulSet,
	result controllerutil.OperationResult,
) (operationResult, *appsv1.StatefulSet, error) {
	var op = noOp

	switch result {
	case controllerutil.OperationResultCreated:
		op = bootstrapOp
	case controllerutil.OperationResultUpdated:
		// restart etcd pods in crashloop backoff
		selector, err := metav1.LabelSelectorAsSelector(sts.Spec.Selector)
		if err != nil {
			logger.Error(err, "error converting StatefulSet selector to selector")
			return op, nil, err
		}
		podList := &v1.PodList{}
		if err := r.List(ctx, podList, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
			return op, nil, err
		}

		for _, pod := range podList.Items {
			if utils.IsPodInCrashloopBackoff(pod.Status) {
				if err := r.Delete(ctx, &pod); err != nil {
					logger.Error(err, fmt.Sprintf("error deleting etcd pod in crashloop: %s/%s", pod.Namespace, pod.Name))
					return op, nil, err
				}
			}
		}

		op = reconcileOp
	}

	sts, err := r.waitUntilStatefulSetReady(ctx, logger, etcd, sts)

	return op, sts, err
}

func deleteIfExists(ctx context.Context, c client.Client, obj client.Object, opts ...client.DeleteOption) error {
	if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Already deleted.
		}

		return err
	}

	return c.Delete(ctx, obj, opts...)
}

func recreate(ctx context.Context, c client.Client, obj client.Object, opts ...client.DeleteOption) error {
	var metaObj = &metav1.PartialObjectMetadata{}

	if err := c.Scheme().Convert(obj, metaObj, nil); err != nil {
		return err
	}

	if err := c.Delete(ctx, metaObj, opts...); err != nil {
		return err
	}

	return c.Create(ctx, obj)
}

func validateImmutableFields(gk schema.GroupKind, oldObj, newObj *unstructured.Unstructured, immutableFieldPaths [][]string) error {
	for _, fp := range immutableFieldPaths {
		oldV, oldFound, _ := unstructured.NestedFieldNoCopy(oldObj.UnstructuredContent(), fp...)
		newV, newFound, _ := unstructured.NestedFieldNoCopy(newObj.UnstructuredContent(), fp...)

		if !oldFound || (newFound && reflect.DeepEqual(oldV, newV)) {
			continue
		}

		return apierrors.NewInvalid(gk, newObj.GetName(), field.ErrorList{
			field.Invalid(toPath(fp...), newV, "immutable"),
		})
	}

	return nil
}

func toPath(fields ...string) *field.Path {
	var p *field.Path
	for _, f := range fields {
		if p == nil {
			p = field.NewPath(f)
		} else {
			p = p.Child(f)
		}
	}

	return p
}

func preserveFields(gk schema.GroupKind, oldObj, newObj *unstructured.Unstructured, preserveFieldPaths [][]string) error {
	for _, fp := range preserveFieldPaths {
		oldV, oldFound, _ := unstructured.NestedFieldCopy(oldObj.UnstructuredContent(), fp...)

		if !oldFound {
			continue
		}

		if err := unstructured.SetNestedField(newObj.UnstructuredContent(), oldV, fp...); err != nil {
			return apierrors.NewInvalid(gk, newObj.GetName(), field.ErrorList{
				field.Invalid(toPath(fp...), oldV, err.Error()),
			})
		}
	}

	return nil
}

func preserveStatefulSetResources(sch *runtime.Scheme, oldObj, newObj *unstructured.Unstructured) error {
	var (
		oldSts         = &appsv1.StatefulSet{}
		newSts         = &appsv1.StatefulSet{}
		emptyResources = &corev1.ResourceRequirements{}
	)

	if err := sch.Convert(oldObj, oldSts, nil); err != nil {
		return err
	}

	if err := sch.Convert(newObj, newSts, nil); err != nil {
		return err
	}

	for i := range oldSts.Spec.Template.Spec.Containers {
		var c = &oldSts.Spec.Template.Spec.Containers[i]
		if reflect.DeepEqual(&c.Resources, emptyResources) {
			continue // Do not overwrite with empty resources
		}

		deepCopyContainerResources(c.Name, &c.Resources, newSts.Spec.Template.Spec.Containers)
	}

	return sch.Convert(newSts, newObj, nil)
}

func deepCopyContainerResources(containerName string, res *corev1.ResourceRequirements, containers []corev1.Container) {
	for i := range containers {
		var c = &containers[i]
		if c.Name == containerName {
			res.DeepCopyInto(&c.Resources)
			return
		}
	}
}

func forceOverwriteFields(gk schema.GroupKind, src, target *unstructured.Unstructured, fieldPaths [][]string) error {
	for _, fp := range fieldPaths {
		srcV, _, _ := unstructured.NestedFieldCopy(src.UnstructuredContent(), fp...)

		if err := unstructured.SetNestedField(target.UnstructuredContent(), srcV, fp...); err != nil {
			return apierrors.NewInvalid(gk, target.GetName(), field.ErrorList{
				field.Invalid(toPath(fp...), srcV, err.Error()),
			})
		}
	}

	return nil
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

func (r *EtcdReconciler) getMapFromEtcd(etcd *druidv1alpha1.Etcd) (map[string]interface{}, error) {
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

	var statefulsetReplicas int
	if etcd.Spec.Replicas != 0 {
		statefulsetReplicas = 1
	}

	etcdValues := map[string]interface{}{
		"defragmentationSchedule": etcd.Spec.Etcd.DefragmentationSchedule,
		"enableTLS":               (etcd.Spec.Etcd.TLS != nil),
		"pullPolicy":              corev1.PullIfNotPresent,
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

	if etcd.Spec.Etcd.EtcdDefragTimeout != nil {
		etcdValues["etcdDefragTimeout"] = etcd.Spec.Etcd.EtcdDefragTimeout
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

	var enableProfiling = false
	if etcd.Spec.Backup.EnableProfiling != nil {
		enableProfiling = *etcd.Spec.Backup.EnableProfiling

	}

	backupValues := map[string]interface{}{
		"pullPolicy":               corev1.PullIfNotPresent,
		"etcdQuotaBytes":           quota,
		"etcdConnectionTimeout":    "5m",
		"snapstoreTempDir":         "/var/etcd/data/temp",
		"deltaSnapshotMemoryLimit": deltaSnapshotMemoryLimit,
		"enableProfiling":          enableProfiling,
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

	if etcd.Spec.Backup.BackupCompactionSchedule != nil {
		backupValues["backupCompactionSchedule"] = etcd.Spec.Backup.BackupCompactionSchedule
	}

	backupValues["enableBackupCompactionJobTempFS"] = r.enableBackupCompactionJobTempFS

	if etcd.Spec.Backup.EtcdSnapshotTimeout != nil {
		backupValues["etcdSnapshotTimeout"] = etcd.Spec.Backup.EtcdSnapshotTimeout
	}

	if etcd.Spec.Backup.Port != nil {
		backupValues["port"] = etcd.Spec.Backup.Port
	}

	if etcd.Spec.Backup.SnapshotCompression != nil {
		compressionValues := make(map[string]interface{})
		if etcd.Spec.Backup.SnapshotCompression.Enabled {
			compressionValues["enabled"] = etcd.Spec.Backup.SnapshotCompression.Enabled
		}
		if etcd.Spec.Backup.SnapshotCompression.Policy != nil {
			compressionValues["policy"] = etcd.Spec.Backup.SnapshotCompression.Policy
		}
		backupValues["compression"] = compressionValues
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

	sharedConfigValues := map[string]interface{}{
		"autoCompactionMode":      druidv1alpha1.Periodic,
		"autoCompactionRetention": DefaultAutoCompactionRetention,
	}

	if etcd.Spec.Common.AutoCompactionMode != nil {
		sharedConfigValues["autoCompactionMode"] = etcd.Spec.Common.AutoCompactionMode
	}

	if etcd.Spec.Common.AutoCompactionRetention != nil {
		sharedConfigValues["autoCompactionRetention"] = etcd.Spec.Common.AutoCompactionRetention
	}

	values := map[string]interface{}{
		"name":                    etcd.Name,
		"uid":                     etcd.UID,
		"selector":                etcd.Spec.Selector,
		"labels":                  etcd.Spec.Labels,
		"annotations":             etcd.Spec.Annotations,
		"etcd":                    etcdValues,
		"backup":                  backupValues,
		"sharedConfig":            sharedConfigValues,
		"replicas":                etcd.Spec.Replicas,
		"statefulsetReplicas":     statefulsetReplicas,
		"serviceName":             getServiceNameFor(etcd),
		"configMapName":           getConfigMapNameFor(etcd),
		"cronJobName":             getCronJobName(etcd),
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

func getServiceNameFor(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-client", etcd.Name)
}

func getConfigMapNameFor(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("etcd-bootstrap-%s", string(etcd.UID[:6]))
}

func getStatefulSetNameFor(etcd *druidv1alpha1.Etcd) string {
	return etcd.Name
}

func (r *EtcdReconciler) addFinalizersToDependantSecrets(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
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
			logger.Info("Adding finalizer for secret", "secret", kutil.Key(secret.Namespace, secret.Name).String())
			finalizers.Insert(FinalizerName)
			secret.Finalizers = finalizers.UnsortedList()
			if err := r.Update(ctx, secret); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *EtcdReconciler) removeFinalizersToDependantSecrets(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	secrets := []*corev1.SecretReference{}
	if etcd.Spec.Etcd.TLS != nil {
		secrets = append(secrets,
			&etcd.Spec.Etcd.TLS.ClientTLSSecretRef,
			&etcd.Spec.Etcd.TLS.ServerTLSSecretRef,
			&etcd.Spec.Etcd.TLS.TLSCASecretRef,
		)
	}
	if etcd.Spec.Backup.Store != nil && etcd.Spec.Backup.Store.SecretRef != nil {
		secrets = append(secrets, etcd.Spec.Backup.Store.SecretRef)
	}

	for _, secretRef := range secrets {
		secret := &corev1.Secret{}
		if err := r.Client.Get(ctx, types.NamespacedName{
			Name:      secretRef.Name,
			Namespace: etcd.Namespace,
		}, secret); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		} else if finalizers := sets.NewString(secret.Finalizers...); finalizers.Has(FinalizerName) {
			logger.Info("Removing finalizer from secret", "secret", kutil.Key(secret.Namespace, secret.Name).String())
			finalizers.Delete(FinalizerName)
			secret.Finalizers = finalizers.UnsortedList()
			if err := r.Update(ctx, secret); client.IgnoreNotFound(err) != nil {
				return err
			}
		}
	}
	return nil
}

func (r *EtcdReconciler) removeDependantStatefulset(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (waitForStatefulSetCleanup bool, err error) {
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, client.ObjectKey{Name: getStatefulSetNameFor(etcd), Namespace: etcd.Namespace}, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil // Nothing to delete
		}

		return false, err
	}

	waitForStatefulSetCleanup = false

	if canDeleteStatefulset(sts, etcd) {
		logger.Info("Deleting statefulset", "statefulset", kutil.Key(sts.GetNamespace(), sts.GetName()).String())
		if err := r.Delete(ctx, sts, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			return false, err
		}

		// StatefultSet deletion succeeded. Now we need to wait for it to be cleaned up.
		waitForStatefulSetCleanup = true
	}

	return waitForStatefulSetCleanup, nil
}

func canDeleteStatefulset(sts *appsv1.StatefulSet, etcd *druidv1alpha1.Etcd) bool {
	// Adding check for ownerReference to have the same delete path for statefulset.
	// The statefulset with ownerReference will be deleted automatically when etcd is
	// delete but we would like to explicitly delete it to maintain uniformity in the
	// delete path.
	return checkEtcdOwnerReference(sts.GetOwnerReferences(), etcd) ||
		checkEtcdAnnotations(sts.GetAnnotations(), etcd)
}

func bootstrapReset(etcd *druidv1alpha1.Etcd) {
	etcd.Status.Members = nil
	etcd.Status.ClusterSize = pointer.Int32Ptr(int32(etcd.Spec.Replicas))
}

func (r *EtcdReconciler) updateEtcdErrorStatus(ctx context.Context, op operationResult, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet, lastError error) error {
	return kutil.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcd, func() error {
		lastErrStr := fmt.Sprintf("%v", lastError)
		etcd.Status.LastError = &lastErrStr
		etcd.Status.ObservedGeneration = &etcd.Generation
		if sts != nil {
			ready := CheckStatefulSet(etcd, sts) == nil
			etcd.Status.Ready = &ready

			if op == bootstrapOp {
				// Reset members in bootstrap phase to ensure depending conditions can be calculated correctly.
				bootstrapReset(etcd)
			}
		}
		return nil
	})
}

func (r *EtcdReconciler) updateEtcdStatus(ctx context.Context, op operationResult, etcd *druidv1alpha1.Etcd, svc *corev1.Service, sts *appsv1.StatefulSet) error {
	return kutil.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcd, func() error {
		ready := CheckStatefulSet(etcd, sts) == nil
		etcd.Status.Ready = &ready
		svcName := svc.Name
		etcd.Status.ServiceName = &svcName
		etcd.Status.LastError = nil
		etcd.Status.ObservedGeneration = &etcd.Generation

		if op == bootstrapOp {
			// Reset members in bootstrap phase to ensure depending conditions can be calculated correctly.
			bootstrapReset(etcd)
		}
		return nil
	})
}

func (r *EtcdReconciler) waitUntilStatefulSetReady(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	var (
		ss = &appsv1.StatefulSet{}
	)

	err := gardenerretry.UntilTimeout(ctx, DefaultInterval, DefaultTimeout, func(ctx context.Context) (bool, error) {
		if err := r.Get(ctx, types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}, ss); err != nil {
			if apierrors.IsNotFound(err) {
				return gardenerretry.MinorError(err)
			}
			return gardenerretry.SevereError(err)
		}
		if err := CheckStatefulSet(etcd, ss); err != nil {
			return gardenerretry.MinorError(err)
		}
		return gardenerretry.Ok()
	})
	if err != nil {
		messages, err2 := r.fetchPVCEventsFor(ctx, ss)
		if err2 != nil {
			logger.Error(err2, "Error while fetching events for depending PVC")
			// don't expose this error since fetching events is a best effort
			// and shouldn't be confused with the actual error
			return ss, err
		}
		if messages != "" {
			return ss, fmt.Errorf("%w\n\n%s", err, messages)
		}
	}

	return ss, err
}

func (r *EtcdReconciler) fetchPVCEventsFor(ctx context.Context, ss *appsv1.StatefulSet) (string, error) {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcs, client.InNamespace(ss.GetNamespace())); err != nil {
		return "", err
	}

	var (
		pvcMessages  string
		volumeClaims = ss.Spec.VolumeClaimTemplates
	)
	for _, volumeClaim := range volumeClaims {
		for _, pvc := range pvcs.Items {
			if !strings.HasPrefix(pvc.GetName(), fmt.Sprintf("%s-%s", volumeClaim.Name, ss.Name)) || pvc.Status.Phase == corev1.ClaimBound {
				continue
			}
			messages, err := kutil.FetchEventMessages(ctx, r.Client.Scheme(), r.Client, &pvc, corev1.EventTypeWarning, 2)
			if err != nil {
				return "", err
			}
			if messages != "" {
				pvcMessages += fmt.Sprintf("Warning for PVC %s:\n%s\n", pvc.Name, messages)
			}
		}
	}
	return pvcMessages, nil
}

func (r *EtcdReconciler) removeOperationAnnotation(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	if _, ok := etcd.Annotations[v1beta1constants.GardenerOperation]; ok {
		logger.Info("Removing operation annotation")
		delete(etcd.Annotations, v1beta1constants.GardenerOperation)
		return r.Update(ctx, etcd)
	}
	return nil
}

func (r *EtcdReconciler) updateEtcdStatusAsNotReady(ctx context.Context, etcd *druidv1alpha1.Etcd) (*druidv1alpha1.Etcd, error) {
	err := kutil.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcd, func() error {
		etcd.Status.Ready = nil
		etcd.Status.ReadyReplicas = 0
		return nil
	})
	return etcd, err
}

// SetupWithManager sets up manager with a new controller and r as the reconcile.Reconciler
func (r *EtcdReconciler) SetupWithManager(mgr ctrl.Manager, workers int, ignoreOperationAnnotation bool) error {
	builder := ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: workers,
	})
	builder = builder.WithEventFilter(buildPredicate(ignoreOperationAnnotation)).For(&druidv1alpha1.Etcd{})
	if ignoreOperationAnnotation {
		builder = builder.Owns(&v1.Service{}).
			Owns(&v1.ConfigMap{}).
			Owns(&appsv1.StatefulSet{})
	}
	return builder.Complete(r)
}

func getCronJobName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-compact-backup", etcd.Name)
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
