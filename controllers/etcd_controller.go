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
	componentservice "github.com/gardener/etcd-druid/pkg/component/etcd/service"
	componentsts "github.com/gardener/etcd-druid/pkg/component/etcd/statefulset"
	druidpredicates "github.com/gardener/etcd-druid/pkg/predicate"
	"github.com/gardener/etcd-druid/pkg/utils"
	coordinationv1 "k8s.io/api/coordination/v1"

	extensionspredicate "github.com/gardener/gardener/extensions/pkg/predicate"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllerutils"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
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
	//waitTimeForTests allow some waiting time between certain operations. This variable help some unit test cases
	waitTimeForTests time.Duration
}

// NewEtcdReconciler creates a new EtcdReconciler object
func NewEtcdReconciler(mgr manager.Manager, disableEtcdServiceAccountAutomount bool, waitTimeForTests time.Duration) (*EtcdReconciler, error) {
	return (&EtcdReconciler{
		Client:                             mgr.GetClient(),
		Config:                             mgr.GetConfig(),
		Scheme:                             mgr.GetScheme(),
		logger:                             log.Log.WithName("etcd-controller"),
		disableEtcdServiceAccountAutomount: disableEtcdServiceAccountAutomount,
		waitTimeForTests:                   waitTimeForTests,
	}).InitializeControllerWithChartApplier()
}

// NewEtcdReconcilerWithImageVector creates a new EtcdReconciler object
func NewEtcdReconcilerWithImageVector(mgr manager.Manager, disableEtcdServiceAccountAutomount bool, waitTimeForTests time.Duration) (*EtcdReconciler, error) {
	ec, err := NewEtcdReconciler(mgr, disableEtcdServiceAccountAutomount, waitTimeForTests)
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
		return predicate.Or(
			predicate.GenerationChangedPredicate{},
			druidpredicates.HasQuorumLossAnnotation(),
		)
	}

	return predicate.Or(
		druidpredicates.HasOperationAnnotation(),
		druidpredicates.HasQuorumLossAnnotation(),
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

	// Check if annotation for quorum loss is present in the ETCD annotaions
	// if yes, take necessarry actions
	if val, ok := etcd.Annotations[druidpredicates.QuorumLossAnnotation]; ok {
		if val == "true" {
			// scale down the statefulset to 0
			sts := &appsv1.StatefulSet{}
			err := r.Get(ctx, req.NamespacedName, sts)
			if err != nil {
				return ctrl.Result{
					RequeueAfter: 10 * time.Second,
				}, fmt.Errorf("cound not fetch statefulset though the annotaion action/quorum-loss is set in ETCD CR: %v", err)
			}

			r.logger.Info("Scaling down the statefulset to 0 while tackling quorum loss scenario in ETCD multi node cluster")
			if _, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.Client, sts, func() error {
				sts.Spec.Replicas = pointer.Int32(0)
				return nil
			}); err != nil {
				return ctrl.Result{
					RequeueAfter: 10 * time.Second,
				}, fmt.Errorf("cound not scale down statefulset to 0 while tackling quorum loss scenario in ETCD multi node cluster: %v", err)
			}
			time.Sleep(r.waitTimeForTests)

			r.logger.Info("Deleting PVCs while tackling quorum loss scenario in ETCD multi node cluster")
			// delete the pvcs
			if err := r.DeleteAllOf(ctx, &corev1.PersistentVolumeClaim{},
				client.InNamespace(sts.GetNamespace()),
				client.MatchingLabels(getMatchingLabels(sts))); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{
					RequeueAfter: 10 * time.Second,
				}, fmt.Errorf("cound not delete pvcs while tackling quorum loss scenario in ETCD multi node cluster : %v", err)
			}

			r.logger.Info("Update the lease renewal time as nil")
			leases := &coordinationv1.LeaseList{}
			if err := r.List(ctx, leases, client.InNamespace(etcd.Namespace), client.MatchingLabels{
				common.GardenerOwnedBy: etcd.Name, v1beta1constants.GardenerPurpose: componentlease.PurposeMemberLease}); err != nil {
				r.logger.Error(err, "failed to get leases for etcd member readiness check")
			}

			for _, lease := range leases.Items {
				copyLease := lease.DeepCopy()
				withNilRenewal := copyLease.DeepCopy()
				copyLease.Spec.RenewTime = nil
				err := r.Patch(ctx, copyLease, client.MergeFrom(withNilRenewal))

				if err != nil {
					return ctrl.Result{
						RequeueAfter: 10 * time.Second,
					}, fmt.Errorf("could not set all the lease items with nil as renewal time: %v", err)
				}
			}

			r.logger.Info("Scaling up the statefulset to 1 while tackling quorum loss scenario in ETCD multi node cluster")
			// scale up the statefulset to 1
			if _, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.Client, sts, func() error {
				sts.Spec.Replicas = pointer.Int32(1)
				return nil
			}); err != nil {
				return ctrl.Result{
					RequeueAfter: 10 * time.Second,
				}, fmt.Errorf("cound not scale up statefulset to 1 while tackling quorum loss scenario in ETCD multi node cluster : %v", err)
			}

			if err := r.waitUntilStatefulSetReady(ctx, r.logger, req.NamespacedName, 1); err != nil {
				return ctrl.Result{
					RequeueAfter: 10 * time.Second,
				}, fmt.Errorf("statefulset with 1 replica is not ready yet while tackling quorum loss scenario in ETCD multi node cluster : %v", err)
			}

			// scale up the statefulset to ETCD replicas
			r.logger.Info("Scaling up the statefulset to the number of replicas mentioned in ETCD spec")
			if _, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.Client, sts, func() error {
				sts.Spec.Replicas = &etcd.Spec.Replicas
				return nil
			}); err != nil {
				return ctrl.Result{
					RequeueAfter: 10 * time.Second,
				}, fmt.Errorf("cound not scale up statefulset to replica number while tackling quorum loss scenario in ETCD multi node cluster : %v", err)
			}

			// Quorum loss case has been handled by Druid side. The annotation will be removed now.
			if err = r.removeQuorumLossAnnotation(ctx, r.logger, etcd); err != nil {
				if apierrors.IsNotFound(err) {
					return ctrl.Result{}, nil
				}
				return ctrl.Result{
					Requeue: true,
				}, err
			}
			return ctrl.Result{
				Requeue: false,
			}, nil
		}
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

	stsDeployer := gardenercomponent.OpDestroyAndWait(componentsts.New(r.Client, logger, componentsts.Values{Name: etcd.Name, Namespace: etcd.Namespace}))
	if err := stsDeployer.Destroy(ctx); err != nil {
		if err = r.updateEtcdErrorStatus(ctx, etcd, nil, err); err != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
		return ctrl.Result{
			Requeue: true,
		}, err
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
	// TODO(timuthy): The following checks should rather be part of a validation. Also re-enqueuing doesn't make sense in case the values are invalid.
	if etcd.Spec.Replicas > 1 && etcd.Spec.Replicas&1 == 0 {
		return nil, nil, fmt.Errorf("Spec.Replicas should not be even number: %d", etcd.Spec.Replicas)
	}

	etcdImage, etcdBackupImage, err := getEtcdImages(r.ImageVector, etcd)
	if err != nil {
		return nil, nil, err
	}

	if etcd.Spec.Etcd.Image == nil {
		if etcdImage == "" {
			return nil, nil, fmt.Errorf("either etcd resource or image vector should have %s image while deploying statefulset", common.Etcd)
		}
	} else {
		etcdImage = *etcd.Spec.Etcd.Image
	}

	if etcd.Spec.Backup.Image == nil {
		if etcdBackupImage == "" {
			return nil, nil, fmt.Errorf("either etcd resource or image vector should have %s image while deploying statefulset", common.BackupRestore)
		}
	} else {
		etcdBackupImage = *etcd.Spec.Backup.Image
	}

	leaseValues := componentlease.GenerateValues(etcd)
	leaseDeployer := componentlease.New(r.Client, etcd.Namespace, leaseValues)
	if err := leaseDeployer.Deploy(ctx); err != nil {
		return nil, nil, err
	}

	serviceValues := componentservice.GenerateValues(etcd)
	serviceDeployer := componentservice.New(r.Client, etcd.Namespace, serviceValues)
	if err := serviceDeployer.Deploy(ctx); err != nil {
		return nil, nil, err
	}

	configMapValues := componentconfigmap.GenerateValues(etcd)
	cmDeployer := componentconfigmap.New(r.Client, etcd.Namespace, configMapValues)
	if err := cmDeployer.Deploy(ctx); err != nil {
		return nil, nil, err
	}

	values, err := r.getMapFromEtcd(etcd, r.disableEtcdServiceAccountAutomount)
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

	statefulSetValues := componentsts.GenerateValues(etcd,
		&serviceValues.ClientPort,
		&serviceValues.ServerPort,
		&serviceValues.BackupPort,
		etcdImage,
		etcdBackupImage,
		map[string]string{
			"checksum/etcd-configmap": configMapValues.ConfigMapChecksum,
		})

	// Create an OpWaiter because after the depoyment we want to wait until the StatefulSet is ready.
	var (
		stsDeployer  = componentsts.New(r.Client, logger, statefulSetValues)
		deployWaiter = gardenercomponent.OpWaiter(stsDeployer)
	)

	if _, err := stsDeployer.Get(ctx); apierrors.IsNotFound(err) {

		logger.Info("Statefulsets does not exist, bootstrapping new one. Adding bootstrap annotation to ETCD CR")
		etcdCopy := etcd.DeepCopy()
		annotations := make(map[string]string)
		if etcd.Annotations != nil {
			for key, value := range etcd.Annotations {
				annotations[key] = value
			}
		}
		// Set annotaion in ETCD to take corrective measure
		annotations[utils.BootstrapAnnotation] = "true"
		etcd.Annotations = annotations
		logger.Info(fmt.Sprintf("Updating ETCD with the annotation %v", utils.BootstrapAnnotation))
		if err := r.Patch(ctx, etcd, client.MergeFrom(etcdCopy)); err != nil {
			return nil, nil, err
		}
	}

	if err := deployWaiter.Deploy(ctx); err != nil {
		return nil, nil, err
	}

	if err = r.removeBootstrapAnnotation(ctx, r.logger, etcd); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, nil, err
		}
	}

	sts, err := stsDeployer.Get(ctx)

	return &serviceValues.ClientServiceName, sts, err
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

func getMatchingLabels(sts *appsv1.StatefulSet) map[string]string {
	labels := make(map[string]string)

	labels["name"] = sts.Labels["name"]
	labels["instance"] = sts.Labels["instance"]

	return labels
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

			ready := utils.CheckStatefulSet(etcd.Spec.Replicas, sts) == nil
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

		ready := utils.CheckStatefulSet(etcd.Spec.Replicas, sts) == nil
		etcd.Status.Ready = &ready
		svcName := serviceName
		etcd.Status.ServiceName = &svcName
		etcd.Status.LastError = nil
		etcd.Status.ObservedGeneration = &etcd.Generation
		etcd.Status.Replicas = pointer.Int32PtrDerefOr(sts.Spec.Replicas, 0)
		return nil
	})
}

func (r *EtcdReconciler) waitUntilStatefulSetReady(ctx context.Context, logger logr.Logger, ns types.NamespacedName, replicas int32) error {
	sts := &appsv1.StatefulSet{}
	err := gardenerretry.UntilTimeout(ctx, DefaultInterval, DefaultTimeout, func(ctx context.Context) (bool, error) {
		if err := r.Get(ctx, ns, sts); err != nil {
			if apierrors.IsNotFound(err) {
				return gardenerretry.MinorError(err)
			}
			return gardenerretry.SevereError(err)
		}
		if err := checkStatefulSet(sts, replicas); err != nil {
			return gardenerretry.MinorError(err)
		}
		return gardenerretry.Ok()
	})

	return err
}

// checkStatefulSet checks whether the given StatefulSet is healthy.
// A StatefulSet is considered healthy if its controller observed its current revision,
// it is not in an update (i.e. UpdateRevision is empty) and if its current replicas are equal to
// desired replicas specified in ETCD specs.
func checkStatefulSet(statefulSet *appsv1.StatefulSet, replicas int32) error {
	if statefulSet.Status.ObservedGeneration < statefulSet.Generation {
		return fmt.Errorf("observed generation outdated (%d/%d)", statefulSet.Status.ObservedGeneration, statefulSet.Generation)
	}

	if statefulSet.Status.ReadyReplicas < replicas {
		return fmt.Errorf("not enough ready replicas (%d/%d)", statefulSet.Status.ReadyReplicas, replicas)
	}

	return nil
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

func (r *EtcdReconciler) removeQuorumLossAnnotation(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	if _, ok := etcd.Annotations[druidpredicates.QuorumLossAnnotation]; ok {
		logger.Info("Removing quorum loss annotation")
		withQlAnnotation := etcd.DeepCopy()
		delete(etcd.Annotations, druidpredicates.QuorumLossAnnotation)
		return r.Patch(ctx, etcd, client.MergeFrom(withQlAnnotation))
	}
	return nil
}

func (r *EtcdReconciler) removeBootstrapAnnotation(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	if _, ok := etcd.Annotations[utils.BootstrapAnnotation]; ok {
		logger.Info("Removing bootstrap annotation")
		withBsAnnotation := etcd.DeepCopy()
		delete(etcd.Annotations, utils.BootstrapAnnotation)
		return r.Patch(ctx, etcd, client.MergeFrom(withBsAnnotation))
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
