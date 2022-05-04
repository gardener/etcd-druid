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
	componentetcd "github.com/gardener/etcd-druid/pkg/component/etcd"
	componentconfigmap "github.com/gardener/etcd-druid/pkg/component/etcd/configmap"
	componentlease "github.com/gardener/etcd-druid/pkg/component/etcd/lease"
	componentservice "github.com/gardener/etcd-druid/pkg/component/etcd/service"
	druidpredicates "github.com/gardener/etcd-druid/pkg/predicate"
	"github.com/gardener/etcd-druid/pkg/utils"

	extensionspredicate "github.com/gardener/gardener/extensions/pkg/predicate"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	gardenerretry "github.com/gardener/gardener/pkg/utils/retry"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
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
)

var (
	// DefaultTimeout is the default timeout for retry operations.
	DefaultTimeout = 1 * time.Minute
)

// EtcdReconciler reconciles a Etcd object
type EtcdReconciler struct {
	client.Client
	Scheme                             *runtime.Scheme
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

func getChartPathForStatefulSet() string {
	return filepath.Join("etcd", "templates", "etcd-statefulset.yaml")
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

func getChartPathForPodDisruptionBudget() string {
	return filepath.Join("etcd", "templates", "etcd-poddisruptionbudget.yaml")
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
	builder = builder.WithEventFilter(buildPredicate(ignoreOperationAnnotation)).For(&druidv1alpha1.Etcd{})
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
	return r.reconcile(ctx, etcd)
}

func (r *EtcdReconciler) reconcile(ctx context.Context, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String(), "operation", "reconcile")

	// Add Finalizers to Etcd
	if finalizers := sets.NewString(etcd.Finalizers...); !finalizers.Has(FinalizerName) {
		logger.Info("Adding finalizer")
		if err := controllerutils.PatchAddFinalizers(ctx, r.Client, etcd, FinalizerName); err != nil {
			if err := r.updateEtcdErrorStatus(ctx, etcd, nil, err); err != nil {
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

	// Delete any existing cronjob if required.
	// TODO(abdasgupta) : This is for backward compatibility towards ETCD-Druid 0.6.0. Remove it.
	cronJob, err := r.cleanCronJobs(ctx, logger, etcd)
	if err != nil {
		return ctrl.Result{
			Requeue: true,
		}, fmt.Errorf("error while cleaning compaction cron job: %v", err)
	}

	if cronJob != nil {
		logger.Info("The running cron job is: " + cronJob.Name)
	}

	svcName, sts, err := r.reconcileEtcd(ctx, logger, etcd)
	if err != nil {
		if err := r.updateEtcdErrorStatus(ctx, etcd, sts, err); err != nil {
			logger.Error(err, "Error during reconciling ETCD")
			return ctrl.Result{
				Requeue: true,
			}, err
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}
	if err := r.updateEtcdStatus(ctx, etcd, *svcName, sts); err != nil {
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	return ctrl.Result{
		Requeue: false,
	}, nil
}

func (r *EtcdReconciler) cleanCronJobs(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (*batchv1beta1.CronJob, error) {
	cronJob := &batchv1beta1.CronJob{}
	err := r.Get(ctx, types.NamespacedName{Name: utils.GetCronJobName(etcd), Namespace: etcd.Namespace}, cronJob)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		return nil, nil
	}

	// Delete cronJob if there is no active job
	if len(cronJob.Status.Active) == 0 {
		err = client.IgnoreNotFound(r.Delete(ctx, cronJob, client.PropagationPolicy(metav1.DeletePropagationForeground)))
		if err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Calculate time elapsed since the cron job is scheduled
	timeElapsed := time.Since(cronJob.Status.LastScheduleTime.Time).Seconds()
	// Delete the cron job if it's running for more than 3 hours
	if timeElapsed > time.Duration(3*time.Hour).Seconds() {
		if err := client.IgnoreNotFound(r.Delete(ctx, cronJob, client.PropagationPolicy(metav1.DeletePropagationForeground))); err != nil {
			return nil, err
		}
		logger.Info("last cron job was stuck and deleted")
		return nil, nil
	}
	return cronJob, nil
}

func (r *EtcdReconciler) delete(ctx context.Context, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String(), "operation", "delete")
	logger.Info("Starting operation")

	// TODO(abdasgupta) : This is for backward compatibility towards ETCD-Druid 0.6.0. Remove it.
	cronJob := &batchv1beta1.CronJob{}
	if err := client.IgnoreNotFound(r.Get(ctx, types.NamespacedName{Name: utils.GetCronJobName(etcd), Namespace: etcd.Namespace}, cronJob)); err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, fmt.Errorf("error while fetching compaction cron job: %v", err)
	}

	if cronJob.Name == utils.GetCronJobName(etcd) && cronJob.DeletionTimestamp == nil {
		logger.Info("Deleting cron job", "cronjob", kutil.ObjectName(cronJob))
		if err := client.IgnoreNotFound(r.Delete(ctx, cronJob, client.PropagationPolicy(metav1.DeletePropagationForeground))); err != nil {
			return ctrl.Result{
				Requeue: true,
			}, fmt.Errorf("error while deleting compaction cron job: %v", err)
		}
	}

	if waitForStatefulSetCleanup, err := r.removeDependantStatefulset(ctx, logger, etcd); err != nil {
		if err = r.updateEtcdErrorStatus(ctx, etcd, nil, err); err != nil {
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

	leaseDeployer := componentlease.New(r.Client, etcd.Namespace, componentlease.GenerateValues(etcd))
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

func (r *EtcdReconciler) reconcilePodDisruptionBudget(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, values map[string]interface{}) error {
	logger.Info("Reconcile PodDisruptionBudget")
	pdb := &policyv1beta1.PodDisruptionBudget{}
	err := r.Get(ctx, types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, pdb)

	if err == nil {
		// pdb already exists, claim it

		selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
		if err != nil {
			logger.Error(err, "Error converting etcd selector to selector")
			return err
		}

		logger.Info("Claiming pdb object")
		_, err = r.claimPodDisruptionBudget(ctx, etcd, selector, pdb)
		return err
	}

	if !apierrors.IsNotFound(err) {
		return err
	}

	// Required podDisruptionBudget doesn't exist. Create new
	pdb, err = r.getPodDisruptionBudgetFromEtcd(etcd, values, logger)
	if err != nil {
		return err
	}

	logger.Info("Creating PodDisruptionBudget", "poddisruptionbudget", kutil.Key(pdb.Namespace, pdb.Name).String())
	if err := r.Create(ctx, pdb); err != nil {
		return err
	}

	return nil
}

func (r *EtcdReconciler) getPodDisruptionBudgetFromEtcd(etcd *druidv1alpha1.Etcd, values map[string]interface{}, logger logr.Logger) (*policyv1beta1.PodDisruptionBudget, error) {
	var err error
	decoded := &policyv1beta1.PodDisruptionBudget{}
	pdbPath := getChartPathForPodDisruptionBudget()
	chartPath := getChartPath()
	renderedChart, err := r.chartApplier.Render(chartPath, etcd.Name, etcd.Namespace, values)
	if err != nil {
		return nil, err
	}
	if content, ok := renderedChart.Files()[pdbPath]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		if err = decoder.Decode(&decoded); err != nil {
			return nil, err
		}
		return decoded, nil
	}

	return nil, fmt.Errorf("missing podDisruptionBudget template file in the charts: %v", pdbPath)
}

func (r *EtcdReconciler) reconcileStatefulSet(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, values map[string]interface{}) (*appsv1.StatefulSet, error) {
	logger.Info("Reconciling etcd statefulset")

	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Machines (see #42639).
	canAdoptFunc := RecheckDeletionTimestamp(func() (metav1.Object, error) {
		foundEtcd := &druidv1alpha1.Etcd{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, foundEtcd)
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
		return nil, err
	}
	dm := NewEtcdDruidRefManager(r.Client, r.Scheme, etcd, selector, etcdGVK, canAdoptFunc)
	statefulSets, err := dm.FetchStatefulSet(ctx, etcd)
	if err != nil {
		logger.Error(err, "Error while fetching StatefulSet")
		return nil, err
	}

	logger.Info("Claiming existing etcd StatefulSet")
	claimedStatefulSets, err := dm.ClaimStatefulsets(ctx, statefulSets)
	if err != nil {
		return nil, err
	}

	if len(claimedStatefulSets) > 0 {
		// Keep only 1 statefulset. Delete the rest
		for i := 1; i < len(claimedStatefulSets); i++ {
			sts := claimedStatefulSets[i]
			logger.Info("Found duplicate StatefulSet, deleting it", "statefulset", kutil.Key(sts.Namespace, sts.Name).String())
			if err := r.Delete(ctx, sts); err != nil {
				logger.Error(err, "Error in deleting duplicate StatefulSet", "statefulset", kutil.Key(sts.Namespace, sts.Name).String())
				continue
			}
		}

		// Fetch the updated statefulset
		// TODO: (timuthy) Check if this is really needed.
		sts := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: claimedStatefulSets[0].Name, Namespace: claimedStatefulSets[0].Namespace}, sts); err != nil {
			return nil, err
		}

		// Statefulset is claimed by for this etcd. Just sync the specs
		if sts, err = r.syncStatefulSetSpec(ctx, logger, sts, etcd, values); err != nil {
			return nil, err
		}

		// restart etcd pods in crashloop backoff
		selector, err := metav1.LabelSelectorAsSelector(sts.Spec.Selector)
		if err != nil {
			logger.Error(err, "error converting StatefulSet selector to selector")
			return nil, err
		}
		podList := &corev1.PodList{}
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

		sts, err = r.waitUntilStatefulSetReady(ctx, logger, etcd, sts)
		return sts, err
	}

	// Required statefulset doesn't exist. Create new
	sts, err := r.getStatefulSetFromEtcd(etcd, values)
	if err != nil {
		return nil, err
	}

	err = r.Create(ctx, sts)

	// Ignore the precondition violated error, this machine is already updated
	// with the desired label.
	if err == errorsutil.ErrPreconditionViolated {
		logger.Info("StatefulSet %s precondition doesn't hold, skip updating it.", "statefulset", kutil.Key(sts.Namespace, sts.Name).String())
		err = nil
	}
	if err != nil {
		return nil, err
	}

	sts, err = r.waitUntilStatefulSetReady(ctx, logger, etcd, sts)
	return sts, err
}

func getContainerMapFromPodTemplateSpec(spec corev1.PodSpec) map[string]corev1.Container {
	containers := map[string]corev1.Container{}
	for _, c := range spec.Containers {
		containers[c.Name] = c
	}
	return containers
}

func clusterScaledUpToMultiNode(etcd *druidv1alpha1.Etcd) bool {
	if etcd == nil {
		return false
	}
	return etcd.Spec.Replicas != 1 &&
		// Also consider `0` here because this field was not maintained in earlier releases.
		(etcd.Status.Replicas == 0 ||
			etcd.Status.Replicas == 1)
}

func (r *EtcdReconciler) syncStatefulSetSpec(ctx context.Context, logger logr.Logger, ss *appsv1.StatefulSet, etcd *druidv1alpha1.Etcd, values map[string]interface{}) (*appsv1.StatefulSet, error) {
	decoded, err := r.getStatefulSetFromEtcd(etcd, values)
	if err != nil {
		return nil, err
	}

	if reflect.DeepEqual(ss.Spec, decoded.Spec) {
		return ss, nil
	}

	ssCopy := ss.DeepCopy()
	ssCopy.Spec.Replicas = decoded.Spec.Replicas
	ssCopy.Spec.UpdateStrategy = decoded.Spec.UpdateStrategy

	recreateSTS := false
	if !reflect.DeepEqual(ssCopy.Spec.Selector, decoded.Spec.Selector) {
		recreateSTS = true
	}

	// We introduced a peer service for multi-node etcd which must be set
	// when the previous single-node StatefulSet still has the client service configured.
	if ssCopy.Spec.ServiceName != decoded.Spec.ServiceName {
		if clusterScaledUpToMultiNode(etcd) {
			recreateSTS = true
		}
	}

	// Applying suggestions from
	containers := getContainerMapFromPodTemplateSpec(ssCopy.Spec.Template.Spec)
	for i, c := range decoded.Spec.Template.Spec.Containers {
		container, ok := containers[c.Name]
		if !ok {
			return nil, fmt.Errorf("container with name %s could not be fetched from statefulset %s", c.Name, decoded.Name)
		}
		// only copy requested resources from the existing stateful set to avoid copying already removed (from the etcd resource) resource limits
		decoded.Spec.Template.Spec.Containers[i].Resources.Requests = container.Resources.Requests
	}

	ssCopy.Spec.Template = decoded.Spec.Template

	if recreateSTS {
		logger.Info("StatefulSet change requires recreation", "statefulset", kutil.Key(ssCopy.Namespace, ssCopy.Name).String())
		err = r.recreateStatefulset(ctx, decoded)
	} else {
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			return r.Patch(ctx, ssCopy, client.MergeFrom(ss))
		})
	}

	// Ignore the precondition violated error, this machine is already updated
	// with the desired label.
	if err == errorsutil.ErrPreconditionViolated {
		logger.Info("StatefulSet precondition doesn't hold, skip updating it", "statefulset", kutil.Key(ss.Namespace, ss.Name).String())
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return ssCopy, err
}

func (r *EtcdReconciler) recreateStatefulset(ctx context.Context, ss *appsv1.StatefulSet) error {
	skipDelete := false
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if !skipDelete {
			if err := r.Delete(ctx, ss); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		skipDelete = true
		return r.Create(ctx, ss)
	})
	return err
}

func (r *EtcdReconciler) getStatefulSetFromEtcd(etcd *druidv1alpha1.Etcd, values map[string]interface{}) (*appsv1.StatefulSet, error) {
	var err error
	decoded := &appsv1.StatefulSet{}
	statefulSetPath := getChartPathForStatefulSet()
	chartPath := getChartPath()
	renderedChart, err := r.chartApplier.Render(chartPath, etcd.Name, etcd.Namespace, values)
	if err != nil {
		return nil, err
	}
	if _, ok := renderedChart.Files()[statefulSetPath]; !ok {
		return nil, fmt.Errorf("missing configmap template file in the charts: %v", statefulSetPath)
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(renderedChart.Files()[statefulSetPath])), 1024)
	if err = decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
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

func (r *EtcdReconciler) reconcileEtcd(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (*string, *appsv1.StatefulSet, error) {
	// Check if Spec.Replicas is odd or even.
	if etcd.Spec.Replicas > 1 && etcd.Spec.Replicas&1 == 0 {
		return nil, nil, fmt.Errorf("Spec.Replicas should not be even number: %d", etcd.Spec.Replicas)
	}

	val := componentetcd.Values{
		ConfigMap: componentconfigmap.GenerateValues(etcd),
		Lease:     componentlease.GenerateValues(etcd),
		Service:   componentservice.GenerateValues(etcd),
	}

	leaseDeployer := componentlease.New(r.Client, etcd.Namespace, val.Lease)

	if err := leaseDeployer.Deploy(ctx); err != nil {
		return nil, nil, err
	}

	serviceDeployer := componentservice.New(r.Client, etcd.Namespace, val.Service)

	if err := serviceDeployer.Deploy(ctx); err != nil {
		return nil, nil, err
	}

	cmDeployer := componentconfigmap.New(r.Client, etcd.Namespace, val.ConfigMap)
	if err := cmDeployer.Deploy(ctx); err != nil {
		return nil, nil, err
	}

	values, err := r.getMapFromEtcd(r.ImageVector, etcd, val, r.disableEtcdServiceAccountAutomount)

	if err != nil {
		return nil, nil, err
	}

	err = r.reconcileServiceAccount(ctx, logger, etcd, values)
	if err != nil {
		return nil, nil, err
	}

	err = r.reconcileRole(ctx, logger, etcd, values)
	if err != nil {
		return nil, nil, err
	}

	err = r.reconcileRoleBinding(ctx, logger, etcd, values)
	if err != nil {
		return nil, nil, err
	}

	err = r.reconcilePodDisruptionBudget(ctx, logger, etcd, values)
	if err != nil {
		return nil, nil, err
	}

	sts, err := r.reconcileStatefulSet(ctx, logger, etcd, values)
	if err != nil {
		return nil, nil, err
	}

	return &val.Service.ClientServiceName, sts, nil
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

func (r *EtcdReconciler) getMapFromEtcd(im imagevector.ImageVector, etcd *druidv1alpha1.Etcd, val componentetcd.Values, disableEtcdServiceAccountAutomount bool) (map[string]interface{}, error) {
	statefulsetReplicas := int(etcd.Spec.Replicas)

	etcdValues := map[string]interface{}{
		"clientPort":              val.Service.ClientPort,
		"defragmentationSchedule": etcd.Spec.Etcd.DefragmentationSchedule,
		"enableClientTLS":         (etcd.Spec.Etcd.ClientUrlTLS != nil),
		"enablePeerTLS":           (etcd.Spec.Etcd.PeerUrlTLS != nil),
		"pullPolicy":              corev1.PullIfNotPresent,
		"serverPort":              val.Service.ServerPort,
		// "username":                etcd.Spec.Etcd.Username,
		// "password":                etcd.Spec.Etcd.Password,
	}

	if etcd.Spec.Etcd.Resources != nil {
		etcdValues["resources"] = etcd.Spec.Etcd.Resources
	}

	if etcd.Spec.Etcd.Metrics != nil {
		etcdValues["metrics"] = etcd.Spec.Etcd.Metrics
	}

	if etcd.Spec.Etcd.EtcdDefragTimeout != nil {
		etcdValues["etcdDefragTimeout"] = etcd.Spec.Etcd.EtcdDefragTimeout
	}

	etcdImage, etcdBackupImage, err := getEtcdImages(im, etcd)
	if err != nil {
		return map[string]interface{}{}, err
	}

	if etcd.Spec.Etcd.Image == nil {
		if etcdImage == "" {
			return map[string]interface{}{}, fmt.Errorf("either etcd resource or image vector should have %s image", common.Etcd)
		}
		etcdValues["image"] = etcdImage
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
		"enableTLS":                (etcd.Spec.Backup.TLS != nil),
		"pullPolicy":               corev1.PullIfNotPresent,
		"port":                     val.Service.BackupPort,
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

	if etcd.Spec.Backup.EtcdSnapshotTimeout != nil {
		backupValues["etcdSnapshotTimeout"] = etcd.Spec.Backup.EtcdSnapshotTimeout
	}

	if etcd.Spec.Backup.SnapshotCompression != nil {
		compressionValues := make(map[string]interface{})
		if pointer.BoolPtrDerefOr(etcd.Spec.Backup.SnapshotCompression.Enabled, false) {
			compressionValues["enabled"] = etcd.Spec.Backup.SnapshotCompression.Enabled
		}
		if etcd.Spec.Backup.SnapshotCompression.Policy != nil {
			compressionValues["policy"] = etcd.Spec.Backup.SnapshotCompression.Policy
		}
		backupValues["compression"] = compressionValues
	}

	if etcd.Spec.Backup.Image == nil {
		if etcdBackupImage == "" {
			return map[string]interface{}{}, fmt.Errorf("either etcd resource or image vector should have %s image", common.BackupRestore)
		}
		backupValues["image"] = etcdBackupImage
	} else {
		backupValues["image"] = etcd.Spec.Backup.Image
	}

	if etcd.Spec.Backup.OwnerCheck != nil {
		ownerCheckValues := map[string]interface{}{
			"name": etcd.Spec.Backup.OwnerCheck.Name,
			"id":   etcd.Spec.Backup.OwnerCheck.ID,
		}
		if etcd.Spec.Backup.OwnerCheck.Interval != nil {
			ownerCheckValues["interval"] = etcd.Spec.Backup.OwnerCheck.Interval
		}
		if etcd.Spec.Backup.OwnerCheck.Timeout != nil {
			ownerCheckValues["timeout"] = etcd.Spec.Backup.OwnerCheck.Timeout
		}
		if etcd.Spec.Backup.OwnerCheck.DNSCacheTTL != nil {
			ownerCheckValues["dnsCacheTTL"] = etcd.Spec.Backup.OwnerCheck.DNSCacheTTL
		}
		backupValues["ownerCheck"] = ownerCheckValues
	}

	if etcd.Spec.Backup.LeaderElection != nil {
		leaderElectionConfig := make(map[string]interface{})
		if etcd.Spec.Backup.LeaderElection.EtcdConnectionTimeout != nil {
			leaderElectionConfig["etcdConnectionTimeout"] = etcd.Spec.Backup.LeaderElection.EtcdConnectionTimeout
		}
		if etcd.Spec.Backup.LeaderElection.ReelectionPeriod != nil {
			leaderElectionConfig["reelectionPeriod"] = etcd.Spec.Backup.LeaderElection.ReelectionPeriod
		}
		backupValues["leaderElection"] = leaderElectionConfig
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

	schedulingConstraints := map[string]interface{}{
		"affinity":                  etcd.Spec.SchedulingConstraints.Affinity,
		"topologySpreadConstraints": etcd.Spec.SchedulingConstraints.TopologySpreadConstraints,
	}

	annotations := make(map[string]string)
	if etcd.Spec.Annotations != nil {
		for key, value := range etcd.Spec.Annotations {
			annotations[key] = value
		}
	}

	annotations["checksum/etcd-configmap"] = val.ConfigMap.ConfigMapChecksum

	pdbMinAvailable := 0
	if etcd.Spec.Replicas > 1 {
		pdbMinAvailable = int(etcd.Spec.Replicas)
	}

	values := map[string]interface{}{
		"name":                               etcd.Name,
		"uid":                                etcd.UID,
		"selector":                           etcd.Spec.Selector,
		"labels":                             etcd.Spec.Labels,
		"annotations":                        annotations,
		"etcd":                               etcdValues,
		"backup":                             backupValues,
		"sharedConfig":                       sharedConfigValues,
		"schedulingConstraints":              schedulingConstraints,
		"replicas":                           etcd.Spec.Replicas,
		"statefulsetReplicas":                statefulsetReplicas,
		"serviceName":                        val.Service.PeerServiceName,
		"configMapName":                      val.ConfigMap.ConfigMapName,
		"jobName":                            utils.GetJobName(etcd),
		"pdbMinAvailable":                    pdbMinAvailable,
		"volumeClaimTemplateName":            volumeClaimTemplateName,
		"serviceAccountName":                 utils.GetServiceAccountName(etcd),
		"disableEtcdServiceAccountAutomount": disableEtcdServiceAccountAutomount,
		"roleName":                           fmt.Sprintf("druid.gardener.cloud:etcd:%s", etcd.Name),
		"roleBindingName":                    fmt.Sprintf("druid.gardener.cloud:etcd:%s", etcd.Name),
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

	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		values["clientUrlTlsCASecret"] = etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name

		if dataKey := etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey; dataKey != nil {
			values["clientTlsCASecretKey"] = *dataKey
		}

		values["clientUrlTlsServerSecret"] = etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name
		values["clientUrlTlsClientSecret"] = etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name
	}

	if etcd.Spec.Etcd.PeerUrlTLS != nil {
		values["peerUrlTlsCASecret"] = etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name

		if dataKey := etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey; dataKey != nil {
			values["peerTlsCASecretKey"] = *dataKey
		}

		values["peerUrlTlsServerSecret"] = etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name
	}

	if heartBeatDuration := etcd.Spec.Etcd.HeartbeatDuration; heartBeatDuration != nil {
		values["heartbeatDuration"] = heartBeatDuration
	}

	if etcd.Spec.Backup.Store != nil {
		if values["store"], err = utils.GetStoreValues(context.Background(), r.Client, etcd.Spec.Backup.Store, etcd.Namespace); err != nil {
			return nil, err
		}

		backupValues["fullSnapLeaseName"] = val.Lease.FullSnapshotLeaseName
		backupValues["deltaSnapLeaseName"] = val.Lease.DeltaSnapshotLeaseName
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

func (r *EtcdReconciler) removeDependantStatefulset(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (waitForStatefulSetCleanup bool, err error) {
	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		return false, err
	}

	statefulSets := &appsv1.StatefulSetList{}
	if err = r.List(ctx, statefulSets, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return false, err
	}

	waitForStatefulSetCleanup = false

	for _, sts := range statefulSets.Items {
		if canDeleteStatefulset(&sts, etcd) {
			var key = kutil.Key(sts.GetNamespace(), sts.GetName()).String()
			logger.Info("Deleting statefulset", "statefulset", key)
			if err := r.Delete(ctx, &sts, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				return false, err
			}

			// StatefultSet deletion succeeded. Now we need to wait for it to be cleaned up.
			waitForStatefulSetCleanup = true
		}
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
	etcd.Status.ClusterSize = pointer.Int32Ptr(etcd.Spec.Replicas)
}

func clusterInBootstrap(etcd *druidv1alpha1.Etcd) bool {
	return etcd.Status.Replicas == 0 ||
		(etcd.Spec.Replicas > 1 && etcd.Status.Replicas == 1)
}

func (r *EtcdReconciler) updateEtcdErrorStatus(ctx context.Context, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet, lastError error) error {
	return controllerutils.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcd, func() error {
		lastErrStr := fmt.Sprintf("%v", lastError)
		etcd.Status.LastError = &lastErrStr
		etcd.Status.ObservedGeneration = &etcd.Generation
		if sts != nil {
			if clusterInBootstrap(etcd) {
				// Reset members in bootstrap phase to ensure dependent conditions can be calculated correctly.
				bootstrapReset(etcd)
			}

			ready := CheckStatefulSet(etcd, sts) == nil
			etcd.Status.Ready = &ready
			etcd.Status.Replicas = pointer.Int32PtrDerefOr(sts.Spec.Replicas, 0)
		}
		return nil
	})
}

func (r *EtcdReconciler) updateEtcdStatus(ctx context.Context, etcd *druidv1alpha1.Etcd, serviceName string, sts *appsv1.StatefulSet) error {
	return controllerutils.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcd, func() error {
		if clusterInBootstrap(etcd) {
			// Reset members in bootstrap phase to ensure dependent conditions can be calculated correctly.
			bootstrapReset(etcd)
		}

		ready := CheckStatefulSet(etcd, sts) == nil
		etcd.Status.Ready = &ready
		svcName := serviceName
		etcd.Status.ServiceName = &svcName
		etcd.Status.LastError = nil
		etcd.Status.ObservedGeneration = &etcd.Generation
		etcd.Status.Replicas = pointer.Int32PtrDerefOr(sts.Spec.Replicas, 0)
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

func (r *EtcdReconciler) claimPodDisruptionBudget(ctx context.Context, etcd *druidv1alpha1.Etcd, selector labels.Selector, pdb *policyv1beta1.PodDisruptionBudget) (*policyv1beta1.PodDisruptionBudget, error) {
	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Machines (see #42639).
	canAdoptFunc := RecheckDeletionTimestamp(func() (metav1.Object, error) {
		foundEtcd := &druidv1alpha1.Etcd{}
		err := r.Get(ctx, types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, foundEtcd)
		if err != nil {
			return nil, err
		}
		if foundEtcd.UID != etcd.UID {
			return nil, fmt.Errorf("original %v/%v hvpa gone: got uid %v, wanted %v", etcd.Namespace, etcd.Name, foundEtcd.UID, etcd.UID)
		}
		return foundEtcd, nil
	})
	cm := NewEtcdDruidRefManager(r.Client, r.Scheme, etcd, selector, etcdGVK, canAdoptFunc)
	return cm.ClaimPodDisruptionBudget(ctx, pdb)
}
