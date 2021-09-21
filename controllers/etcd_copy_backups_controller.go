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
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/chartrenderer"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/pkg/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const workerSuffix = "-worker"

// EtcdCopyBackupsTaskReconciler reconciles a Etcd object
type EtcdCopyBackupsTaskReconciler struct {
	client.Client
	config       *rest.Config
	imageVector  imagevector.ImageVector
	chartApplier kubernetes.ChartApplier
	logger       logr.Logger
}

// NewEtcdCopyBackupsTaskReconciler creates a new NewEtcdCopyBackupsTaskReconciler object
func NewEtcdCopyBackupsTaskReconciler(mgr manager.Manager) (*EtcdCopyBackupsTaskReconciler, error) {
	return (&EtcdCopyBackupsTaskReconciler{
		Client: mgr.GetClient(),
		config: mgr.GetConfig(),
		logger: log.Log.WithName("etcd-copy-backups-task-controller"),
	}).InitializeControllerWithChartApplier()
}

// NewEtcdCopyBackupsTaskReconcilerWithImageVector creates a new NewEtcdCopyBackupsTaskReconciler object and initializes it`s imageVector
func NewEtcdCopyBackupsTaskReconcilerWithImageVector(mgr manager.Manager) (*EtcdCopyBackupsTaskReconciler, error) {
	ec, err := NewEtcdCopyBackupsTaskReconciler(mgr)
	if err != nil {
		return nil, err
	}
	return ec.InitializeControllerWithImageVector()
}

// InitializeControllerWithImageVector will use EtcdCopyBackupsTaskReconciler client to initialize image vector for etcd
// and backup restore images.
func (r *EtcdCopyBackupsTaskReconciler) InitializeControllerWithImageVector() (*EtcdCopyBackupsTaskReconciler, error) {
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getImageYAMLPath())
	if err != nil {
		return nil, err
	}
	r.imageVector = imageVector
	return r, nil
}

// Reconcile reconciles the EtcdCopyBackupsTask.
func (r *EtcdCopyBackupsTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	etcdCopyBackupsTask := &druidv1alpha1.EtcdCopyBackupsTask{}
	if err := r.Get(ctx, req.NamespacedName, etcdCopyBackupsTask); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	if !etcdCopyBackupsTask.DeletionTimestamp.IsZero() {
		return r.delete(ctx, etcdCopyBackupsTask)
	}
	return r.reconcile(ctx, etcdCopyBackupsTask)
}

func (r *EtcdCopyBackupsTaskReconciler) reconcile(ctx context.Context, etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask) (ctrl.Result, error) {
	logger := r.logger.WithValues("task", kutil.Key(etcdCopyBackupsTask.Namespace, etcdCopyBackupsTask.Name).String(), "operation", "reconcile")
	logger.Info("Starting operation")

	if err := r.addFinalizerToObject(ctx, etcdCopyBackupsTask, logger); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.removeOperationAnnotation(ctx, logger, etcdCopyBackupsTask); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	job := &batchv1.Job{}
	if err := r.Get(ctx, kutil.Key(etcdCopyBackupsTask.Namespace, etcdCopyBackupsTask.Name+workerSuffix), job); err != nil && apierrors.IsNotFound(err) {
		logger.Info("Deploying ETCD copy backups job")
		if err := r.checkForDuplicateOperation(ctx, etcdCopyBackupsTask); err != nil {
			if statusError := r.updateEtcdCopyTaskStatusLastError(ctx, etcdCopyBackupsTask, err); statusError != nil {
				return ctrl.Result{}, fmt.Errorf("%s; %s", err, statusError)
			}
			return ctrl.Result{}, err
		}

		values, err := r.getValuesMap(etcdCopyBackupsTask)
		if err != nil {
			return ctrl.Result{}, err

		}
		if job, err = r.runCopyJob(ctx, etcdCopyBackupsTask, values, logger); err != nil {
			if statusError := r.updateEtcdCopyTaskStatusLastError(ctx, etcdCopyBackupsTask, err); statusError != nil {
				return ctrl.Result{}, fmt.Errorf("%s; %s", err, statusError)
			}
			return ctrl.Result{}, err

		}
		if err := r.addFinalizerToObject(ctx, job, logger); err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.updateEtcdCopyBackupsTaskStatus(ctx, etcdCopyBackupsTask); err != nil {
		return ctrl.Result{}, err
	}

	if etcdCopyBackupsTask.Status.LastOperation != nil &&
		(reflect.DeepEqual(etcdCopyBackupsTask.Status.LastOperation.State, druidv1alpha1.LastOperationStateSucceeded) ||
			reflect.DeepEqual(etcdCopyBackupsTask.Status.LastOperation.State, druidv1alpha1.LastOperationStateFailed)) {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{
		RequeueAfter: 30 * time.Second,
	}, nil
}

func (r *EtcdCopyBackupsTaskReconciler) delete(ctx context.Context, etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask) (ctrl.Result, error) {
	logger := r.logger.WithValues("task", kutil.Key(etcdCopyBackupsTask.Namespace, etcdCopyBackupsTask.Name).String(), "operation", "delete")
	logger.Info("Starting operation")

	if etcdCopyBackupsTask.Status.ObjectRef != nil {
		if err := r.updateEtcdCopyBackupsTaskStatus(ctx, etcdCopyBackupsTask); err != nil {
			return ctrl.Result{}, err
		}
		if etcdCopyBackupsTask.Status.LastOperation != nil && reflect.DeepEqual(etcdCopyBackupsTask.Status.LastOperation.State, druidv1alpha1.LastOperationStateActive) {
			return ctrl.Result{
				RequeueAfter: 30 * time.Second,
			}, errors.New("The resource can not be deleted while still processing")
		}

		job := &batchv1.Job{}
		if err := r.Get(ctx, kutil.Key(etcdCopyBackupsTask.Namespace, etcdCopyBackupsTask.Name+workerSuffix), job); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		if err := r.removeFinalizerFromObject(ctx, job, logger); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		if err := r.Delete(ctx, job); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}
	if err := r.removeFinalizerFromObject(ctx, etcdCopyBackupsTask, logger); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info(fmt.Sprintf("Deleted ETCDCopyBackupTask %s/%s successfully.", etcdCopyBackupsTask.Namespace, etcdCopyBackupsTask.Name))
	return ctrl.Result{}, nil
}

func (r *EtcdCopyBackupsTaskReconciler) updateEtcdCopyTaskStatusLastError(ctx context.Context, etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask, err error) error {
	return kutil.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcdCopyBackupsTask, func() error {
		etcdCopyBackupsTask.Status.LastError = pointer.StringPtr(err.Error())
		etcdCopyBackupsTask.Status.ObservedGeneration = &etcdCopyBackupsTask.Generation
		return nil
	})
}

func (r *EtcdCopyBackupsTaskReconciler) updateEtcdCopyBackupsTaskStatus(ctx context.Context, etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask) error {
	return kutil.TryUpdateStatus(ctx, retry.DefaultBackoff, r.Client, etcdCopyBackupsTask, func() error {
		lastOperation, err := r.getLastOperation(ctx, etcdCopyBackupsTask)
		if err != nil {
			if apierrors.IsNotFound(err) {
				etcdCopyBackupsTask.Status.ObjectRef = nil
				etcdCopyBackupsTask.Status.LastOperation = nil
				return nil
			} else {
				etcdCopyBackupsTask.Status.LastError = pointer.StringPtr(err.Error())
				return err
			}
		} else {
			etcdCopyBackupsTask.Status.ObjectRef = &corev1.TypedLocalObjectReference{
				Name:     etcdCopyBackupsTask.Name + workerSuffix,
				Kind:     "Job",
				APIGroup: pointer.StringPtr(batchv1.SchemeGroupVersion.String()),
			}
			etcdCopyBackupsTask.Status.LastOperation = lastOperation
		}
		etcdCopyBackupsTask.Status.ObservedGeneration = &etcdCopyBackupsTask.Generation
		return nil
	})
}

func (r *EtcdCopyBackupsTaskReconciler) getLastOperation(ctx context.Context, etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask) (*druidv1alpha1.LastOperation, error) {
	lastOperation := &druidv1alpha1.LastOperation{}
	job := &batchv1.Job{}

	if err := r.Get(ctx, kutil.Key(etcdCopyBackupsTask.Namespace, etcdCopyBackupsTask.Name+workerSuffix), job); err != nil {
		return nil, err
	}

	if job.Status.Active > 0 {
		lastOperation.State = druidv1alpha1.LastOperationStateActive
		lastOperation.Description = fmt.Sprintf("Job %s is still active", job.Name)
	}
	if job.Status.Succeeded > 0 {
		lastOperation.State = druidv1alpha1.LastOperationStateSucceeded
		lastOperation.Description = fmt.Sprintf("Job %s succeeded", job.Name)
	}
	if job.Status.Failed > 0 {
		lastOperation.State = druidv1alpha1.LastOperationStateFailed
		lastOperation.Description = fmt.Sprintf("Job %s failed", job.Name)
	}
	lastOperation.LastUpdateTime = v1.Now()

	return lastOperation, nil
}

func (r *EtcdCopyBackupsTaskReconciler) removeOperationAnnotation(ctx context.Context, logger logr.Logger, etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask) error {
	if _, ok := etcdCopyBackupsTask.Annotations[v1beta1constants.GardenerOperation]; ok {
		objectCopy := etcdCopyBackupsTask.DeepCopy()
		annotations := objectCopy.GetAnnotations()
		delete(annotations, v1beta1constants.GardenerOperation)
		objectCopy.SetAnnotations(annotations)
		if err := r.Patch(ctx, objectCopy, client.MergeFrom(etcdCopyBackupsTask)); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func (r *EtcdCopyBackupsTaskReconciler) getCopyJob(etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask, values map[string]interface{}, logger logr.Logger) (*batchv1.Job, error) {
	decoded := &batchv1.Job{}
	copyJobPath := filepath.Join("etcd", "templates", "etcd-copy-backups-job.yaml")
	chartPath := getChartPath()

	renderedChart, err := r.chartApplier.Render(chartPath, etcdCopyBackupsTask.Name, etcdCopyBackupsTask.Namespace, values)
	if err != nil {
		return nil, err
	}

	if err := decodeObject(renderedChart, copyJobPath, &decoded); err != nil {
		return nil, err
	}

	return decoded, nil
}

func (r *EtcdCopyBackupsTaskReconciler) runCopyJob(ctx context.Context, etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask, values map[string]interface{}, logger logr.Logger) (*batchv1.Job, error) {
	job, err := r.getCopyJob(etcdCopyBackupsTask, values, logger)
	if err != nil {
		return nil, err
	}
	return job, r.Create(ctx, job)
}

func (r *EtcdCopyBackupsTaskReconciler) getValuesMap(etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask) (map[string]interface{}, error) {
	values := map[string]interface{}{
		"jobName":  etcdCopyBackupsTask.Name + workerSuffix,
		"taskName": etcdCopyBackupsTask.Name,
		"uid":      etcdCopyBackupsTask.UID,
	}

	sourceStoreValues, err := utils.GetStoreValues(etcdCopyBackupsTask.Spec.SourceStore)
	if err != nil {
		return nil, err
	}
	targetStoreValues, err := utils.GetStoreValues(etcdCopyBackupsTask.Spec.TargetStore)
	if err != nil {
		return nil, err
	}
	values["sourceStore"] = sourceStoreValues
	values["targetStore"] = targetStoreValues

	if etcdCopyBackupsTask.Spec.MaxBackupAge != nil {
		values["maxBackupAge"] = *etcdCopyBackupsTask.Spec.MaxBackupAge
	}
	if etcdCopyBackupsTask.Spec.MaxBackups != nil {
		values["maxBackups"] = *etcdCopyBackupsTask.Spec.MaxBackups
	}

	if etcdCopyBackupsTask.Spec.Image != nil {
		values["image"] = etcdCopyBackupsTask.Spec.Image
	} else {
		images, err := imagevector.FindImages(r.imageVector, []string{common.BackupRestore})
		if err != nil {
			return map[string]interface{}{}, err
		}
		val, ok := images[common.BackupRestore]
		if !ok {
			return nil, errors.New("Unable to find etcdbrctl image")
		}
		values["image"] = val.String()
	}

	return values, nil
}

func (r *EtcdCopyBackupsTaskReconciler) addFinalizerToObject(ctx context.Context, object client.Object, logger logr.Logger) error {
	if !sets.NewString(object.GetFinalizers()...).Has(FinalizerName) {
		objectCopy := object.DeepCopyObject().(client.Object)
		finalizers := sets.NewString(objectCopy.GetFinalizers()...)
		finalizers.Insert(FinalizerName)
		objectCopy.SetFinalizers(finalizers.UnsortedList())
		if err := r.Patch(ctx, objectCopy, client.MergeFrom(object)); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func (r *EtcdCopyBackupsTaskReconciler) removeFinalizerFromObject(ctx context.Context, object client.Object, logger logr.Logger) error {
	if sets.NewString(object.GetFinalizers()...).Has(FinalizerName) {
		objectCopy := object.DeepCopyObject().(client.Object)
		finalizers := sets.NewString(objectCopy.GetFinalizers()...)
		finalizers.Delete(FinalizerName)
		objectCopy.SetFinalizers(finalizers.UnsortedList())
		if err := r.Patch(ctx, objectCopy, client.MergeFrom(object)); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func (r *EtcdCopyBackupsTaskReconciler) checkForDuplicateOperation(ctx context.Context, etcdCopyBackupsTask *druidv1alpha1.EtcdCopyBackupsTask) error {
	taskList := druidv1alpha1.EtcdCopyBackupsTaskList{}

	if err := r.List(ctx, &taskList, client.InNamespace(etcdCopyBackupsTask.Namespace)); err != nil {
		return err
	}
	for _, task := range taskList.Items {
		if !reflect.DeepEqual(task.Name, etcdCopyBackupsTask.Name) &&
			reflect.DeepEqual(task.Spec.TargetStore.Provider, etcdCopyBackupsTask.Spec.TargetStore.Provider) &&
			reflect.DeepEqual(task.Spec.TargetStore.Container, etcdCopyBackupsTask.Spec.TargetStore.Container) &&
			reflect.DeepEqual(task.Spec.TargetStore.Prefix, etcdCopyBackupsTask.Spec.TargetStore.Prefix) {
			return fmt.Errorf("duplicate target configuration with task %s", task.Name)
		}
	}

	return nil
}

// SetupWithManager sets up manager with a new controller and ec as the reconcile.Reconciler
func (r *EtcdCopyBackupsTaskReconciler) SetupWithManager(mgr ctrl.Manager, ignoreOperationAnnotation bool) error {
	builder := ctrl.NewControllerManagedBy(mgr)
	builder = builder.WithEventFilter(buildPredicate(ignoreOperationAnnotation)).For(&druidv1alpha1.EtcdCopyBackupsTask{})
	if ignoreOperationAnnotation {
		builder = builder.Owns(&batchv1.Job{})
	}
	return builder.Complete(r)
}

// InitializeControllerWithChartApplier will use EtcdCopyBackupsTaskReconciler client to initialize a Kubernetes client as well as
// a Chart renderer.
func (r *EtcdCopyBackupsTaskReconciler) InitializeControllerWithChartApplier() (*EtcdCopyBackupsTaskReconciler, error) {
	if r.chartApplier != nil {
		return r, nil
	}

	renderer, err := chartrenderer.NewForConfig(r.config)
	if err != nil {
		return nil, err
	}
	applier, err := kubernetes.NewApplierForConfig(r.config)
	if err != nil {
		return nil, err
	}
	r.chartApplier = kubernetes.NewChartApplier(renderer, applier)
	return r, nil
}
