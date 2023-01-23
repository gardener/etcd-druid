// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package etcdcopybackupstask

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/common"
	druidutils "github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const workerSuffix = "-worker"

// Reconciler reconciles EtcdCopyBackupsTask object.
type Reconciler struct {
	client.Client
	Config       *Config
	imageVector  imagevector.ImageVector
	chartApplier kubernetes.ChartApplier
	logger       logr.Logger
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdcopybackupstasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdcopybackupstasks/status;etcdcopybackupstasks/finalizers,verbs=get;update;patch;create

// NewReconciler creates a new EtcdCopyBackupsTaskReconciler.
func NewReconciler(mgr manager.Manager, config *Config, withImageVector bool) (*Reconciler, error) {
	var (
		chartApplier kubernetes.ChartApplier
		imageVector  imagevector.ImageVector
		err          error
	)

	if chartApplier, err = utils.CreateChartApplier(mgr.GetConfig()); err != nil {
		return nil, err
	}
	if withImageVector {
		if imageVector, err = utils.CreateDefaultImageVector(); err != nil {
			return nil, err
		}
	}
	return &Reconciler{
		Client:       mgr.GetClient(),
		Config:       config,
		chartApplier: chartApplier,
		imageVector:  imageVector,
		logger:       log.Log.WithName("etcd-copy-backups-task-controller"),
	}, nil
}

// Reconcile reconciles the EtcdCopyBackupsTask.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	task := &druidv1alpha1.EtcdCopyBackupsTask{}
	if err := r.Get(ctx, req.NamespacedName, task); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !task.DeletionTimestamp.IsZero() {
		return r.delete(ctx, task)
	}
	return r.reconcile(ctx, task)
}

func (r *Reconciler) reconcile(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask) (result ctrl.Result, err error) {
	logger := r.logger.WithValues("task", kutil.ObjectName(task), "operation", "reconcile")

	// Ensure finalizer
	if !controllerutil.ContainsFinalizer(task, common.FinalizerName) {
		logger.V(1).Info("Adding finalizer")
		if err := controllerutils.AddFinalizers(ctx, r.Client, task, common.FinalizerName); err != nil {
			return ctrl.Result{}, fmt.Errorf("could not add finalizer: %w", err)
		}
	}

	var status *druidv1alpha1.EtcdCopyBackupsTaskStatus
	defer func() {
		// Update status, on failure return the update error unless there is another error
		if updateErr := r.updateStatus(ctx, task, status); updateErr != nil && err == nil {
			err = fmt.Errorf("could not update status: %w", updateErr)
		}
	}()

	// Reconcile creation or update
	logger.V(1).Info("Reconciling creation or update")
	if status, err = r.doReconcile(ctx, task, logger); err != nil {
		return ctrl.Result{}, fmt.Errorf("could not reconcile creation or update: %w", err)
	}
	logger.V(1).Info("Creation or update reconciled")

	return ctrl.Result{}, nil
}

func (r *Reconciler) delete(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask) (result ctrl.Result, err error) {
	logger := r.logger.WithValues("task", kutil.ObjectName(task), "operation", "delete")

	// Check finalizer
	if !controllerutil.ContainsFinalizer(task, common.FinalizerName) {
		logger.V(1).Info("Skipping as it does not have a finalizer")
		return ctrl.Result{}, nil
	}

	var status *druidv1alpha1.EtcdCopyBackupsTaskStatus
	var removeFinalizer bool
	defer func() {
		// Only update status if the finalizer is not removed to prevent errors if the object is already gone
		if !removeFinalizer {
			// Update status, on failure return the update error unless there is another error
			if updateErr := r.updateStatus(ctx, task, status); updateErr != nil && err == nil {
				err = fmt.Errorf("could not update status: %w", updateErr)
			}
		}
	}()

	// Reconcile deletion
	logger.V(1).Info("Reconciling deletion")
	if status, removeFinalizer, err = r.doDelete(ctx, task, logger); err != nil {
		return ctrl.Result{}, fmt.Errorf("could not reconcile deletion: %w", err)
	}
	logger.V(1).Info("Deletion reconciled")

	// Remove finalizer if requested
	if removeFinalizer {
		logger.V(1).Info("Removing finalizer")
		if err := controllerutils.RemoveFinalizers(ctx, r.Client, task, common.FinalizerName); err != nil {
			return ctrl.Result{}, fmt.Errorf("could not remove finalizer: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) doReconcile(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask, logger logr.Logger) (status *druidv1alpha1.EtcdCopyBackupsTaskStatus, err error) {
	status = task.Status.DeepCopy()

	var job *batchv1.Job
	defer func() {
		setStatusDetails(status, task.Generation, job, err)
	}()

	// Get job from cluster
	job, err = r.getJob(ctx, task)
	if err != nil {
		return status, err
	}
	if job != nil {
		return status, nil
	}

	// Get chart values
	values, err := r.getChartValues(ctx, task)
	if err != nil {
		return status, fmt.Errorf("could not get chart values: %w", err)
	}

	// Render chart
	// TODO(AleksandarSavchev): .Render is deprecated. Refactor or adapt code to use RenderEmbeddedFS https://github.com/gardener/gardener/pull/6165
	renderedChart, err := r.chartApplier.Render(getEtcdCopyBackupsChartPath(), task.Name, task.Namespace, values) //nolint:staticcheck
	if err != nil {
		return status, fmt.Errorf("could not render chart: %w", err)
	}

	// Decode job object from chart
	job = &batchv1.Job{}
	if err := utils.DecodeObject(renderedChart, getJobPath(), &job); err != nil {
		return status, fmt.Errorf("could not decode job object from chart: %w", err)
	}

	// Create job
	logger.Info("Creating job", "job", kutil.ObjectName(job))
	if err := r.Create(ctx, job); err != nil {
		return status, fmt.Errorf("could not create job %s: %w", kutil.ObjectName(job), err)
	}

	return status, nil
}

func (r *Reconciler) doDelete(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask, logger logr.Logger) (status *druidv1alpha1.EtcdCopyBackupsTaskStatus, removeFinalizer bool, err error) {
	status = task.Status.DeepCopy()

	var job *batchv1.Job
	defer func() {
		setStatusDetails(status, task.Generation, job, err)
	}()

	// Get job from cluster
	job, err = r.getJob(ctx, task)
	if err != nil {
		return status, false, err
	}
	if job == nil {
		return status, true, nil
	}

	// Delete job if needed
	if job.DeletionTimestamp == nil {
		logger.Info("Deleting job", "job", kutil.ObjectName(job))
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground)); client.IgnoreNotFound(err) != nil {
			return status, false, fmt.Errorf("could not delete job %s: %w", kutil.ObjectName(job), err)
		}
	}

	return status, false, nil
}

func (r *Reconciler) getJob(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	if err := r.Get(ctx, kutil.Key(task.Namespace, getCopyBackupsJobName(task)), job); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return job, nil
}

func (r *Reconciler) updateStatus(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask, status *druidv1alpha1.EtcdCopyBackupsTaskStatus) error {
	if status == nil {
		return nil
	}
	patch := client.MergeFromWithOptions(task.DeepCopy(), client.MergeFromWithOptimisticLock{})
	task.Status = *status
	return r.Client.Status().Patch(ctx, task, patch)
}

func (r *Reconciler) getChartValues(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask) (map[string]interface{}, error) {
	values := map[string]interface{}{
		"name":      getCopyBackupsJobName(task),
		"ownerName": task.Name,
		"ownerUID":  task.UID,
	}

	sourceStoreValues, err := druidutils.GetStoreValues(ctx, r.Client, r.logger, &task.Spec.SourceStore, task.Namespace)
	if err != nil {
		return nil, err
	}
	targetStoreValues, err := druidutils.GetStoreValues(ctx, r.Client, r.logger, &task.Spec.TargetStore, task.Namespace)
	if err != nil {
		return nil, err
	}
	values["sourceStore"] = sourceStoreValues
	values["targetStore"] = targetStoreValues

	if task.Spec.MaxBackupAge != nil {
		values["maxBackupAge"] = *task.Spec.MaxBackupAge
	}
	if task.Spec.MaxBackups != nil {
		values["maxBackups"] = *task.Spec.MaxBackups
	}

	if task.Spec.WaitForFinalSnapshot != nil {
		values["waitForFinalSnapshot"] = task.Spec.WaitForFinalSnapshot.Enabled
		if task.Spec.WaitForFinalSnapshot.Timeout != nil {
			values["waitForFinalSnapshotTimeout"] = task.Spec.WaitForFinalSnapshot.Timeout
		}
	}

	images, err := imagevector.FindImages(r.imageVector, []string{common.BackupRestore})
	if err != nil {
		return nil, err
	}
	val, ok := images[common.BackupRestore]
	if !ok {
		return nil, errors.New("etcdbrctl image not found")
	}
	values["image"] = val.String()

	return values, nil
}

func setStatusDetails(status *druidv1alpha1.EtcdCopyBackupsTaskStatus, generation int64, job *batchv1.Job, err error) {
	status.ObservedGeneration = &generation
	if job != nil {
		status.Conditions = getConditions(job.Status.Conditions)
	} else {
		status.Conditions = nil
	}
	if err != nil {
		status.LastError = pointer.StringPtr(err.Error())
	} else {
		status.LastError = nil
	}
}

func getConditions(jobConditions []batchv1.JobCondition) []druidv1alpha1.Condition {
	var conditions []druidv1alpha1.Condition
	for _, jobCondition := range jobConditions {
		if conditionType := getConditionType(jobCondition.Type); conditionType != "" {
			conditions = append(conditions, druidv1alpha1.Condition{
				Type:               conditionType,
				Status:             druidv1alpha1.ConditionStatus(jobCondition.Status),
				LastTransitionTime: jobCondition.LastTransitionTime,
				LastUpdateTime:     jobCondition.LastProbeTime,
				Reason:             jobCondition.Reason,
				Message:            jobCondition.Message,
			})
		}
	}
	return conditions
}

func getConditionType(jobConditionType batchv1.JobConditionType) druidv1alpha1.ConditionType {
	switch jobConditionType {
	case batchv1.JobComplete:
		return druidv1alpha1.EtcdCopyBackupsTaskSucceeded
	case batchv1.JobFailed:
		return druidv1alpha1.EtcdCopyBackupsTaskFailed
	}
	return ""
}

func getCopyBackupsJobName(task *druidv1alpha1.EtcdCopyBackupsTask) string {
	return task.Name + workerSuffix
}

func getEtcdCopyBackupsChartPath() string {
	return filepath.Join("charts", "etcd-copy-backups")
}

func getJobPath() string {
	return filepath.Join("etcd-copy-backups", "templates", "etcd-copy-backups-job.yaml")
}