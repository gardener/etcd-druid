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
	"fmt"
	"strconv"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/common"
	druidutils "github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

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
	Config      *Config
	imageVector imagevector.ImageVector
	logger      logr.Logger
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdcopybackupstasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdcopybackupstasks/status;etcdcopybackupstasks/finalizers,verbs=get;update;patch;create

// NewReconciler creates a new reconciler for EtcdCopyBackupsTask.
func NewReconciler(mgr manager.Manager, config *Config) (*Reconciler, error) {
	imageVector, err := utils.CreateImageVector()
	if err != nil {
		return nil, err
	}
	return NewReconcilerWithImageVector(mgr, config, imageVector), nil
}

// NewReconcilerWithImageVector creates a new reconciler for EtcdCopyBackupsTask with an ImageVector.
// This constructor will mostly be used by tests.
func NewReconcilerWithImageVector(mgr manager.Manager, config *Config, imageVector imagevector.ImageVector) *Reconciler {
	return &Reconciler{
		Client:      mgr.GetClient(),
		Config:      config,
		imageVector: imageVector,
		logger:      log.Log.WithName("etcd-copy-backups-task-controller"),
	}
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
			err = fmt.Errorf("could not update status for task {name: %s, namespace: %s} : %w", task.Name, task.Namespace, updateErr)
		}
	}()

	// Reconcile creation or update
	logger.V(1).Info("Reconciling creation or update for etcd-copy-backups-task", "name", task.Name, "namespace", task.Namespace)
	if status, err = r.doReconcile(ctx, task, logger); err != nil {
		return ctrl.Result{}, fmt.Errorf("could not reconcile creation or update: %w", err)
	}
	logger.V(1).Info("Creation or update reconciled for etcd-copy-backups-task", "name", task.Name, "namespace", task.Namespace)

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

	// create a job object from task
	job, err = r.createJobObject(task)
	if err != nil {
		return status, err
	}

	// Create job
	logger.Info("Creating job", "job", kutil.ObjectName(job))
	if err := r.Create(ctx, job); err != nil {
		return status, fmt.Errorf("could not create job %s: %w", kutil.ObjectName(job), err)
	}

	return status, nil
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

func setStatusDetails(status *druidv1alpha1.EtcdCopyBackupsTaskStatus, generation int64, job *batchv1.Job, err error) {
	status.ObservedGeneration = &generation
	if job != nil {
		status.Conditions = getConditions(job.Status.Conditions)
	} else {
		status.Conditions = nil
	}
	if err != nil {
		status.LastError = pointer.String(err.Error())
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

func (r *Reconciler) createJobObject(task *druidv1alpha1.EtcdCopyBackupsTask) (*batchv1.Job, error) {
	images, err := imagevector.FindImages(r.imageVector, []string{common.BackupRestore})
	if err != nil {
		return nil, err
	}
	val, ok := images[common.BackupRestore]
	if !ok {
		return nil, fmt.Errorf("%s image not found", common.BackupRestore)
	}
	image := val.String()

	targetStore := task.Spec.TargetStore
	targetProvider, err := druidutils.StorageProviderFromInfraProvider(targetStore.Provider)
	if err != nil {
		return nil, err
	}

	sourceStore := task.Spec.SourceStore
	sourceProvider, err := druidutils.StorageProviderFromInfraProvider(sourceStore.Provider)
	if err != nil {
		return nil, err
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getCopyBackupsJobName(task),
			Namespace: task.Namespace,
			Annotations: map[string]string{
				"gardener.cloud/owned-by":   task.Namespace + "/" + task.Name,
				"gardener.cloud/owner-type": "etcdcopybackupstask",
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"networking.gardener.cloud/to-dns":             "allowed",
						"networking.gardener.cloud/to-public-networks": "allowed",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: "OnFailure",
					Containers: []corev1.Container{
						{
							Name:            "copy-backups",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{
								"etcdbrctl",
								"copy",
								"--snapstore-temp-directory=/var/etcd/data/tmp",
							},
							Env:          append(getProviderEnvs(&sourceStore, sourceProvider, "SOURCE_", "source-"), getProviderEnvs(&targetStore, targetProvider, "", "")...),
							VolumeMounts: append(getVolumeMounts(&sourceStore, sourceProvider, "source-"), getVolumeMounts(&targetStore, targetProvider, "target-")...),
						},
					},
				},
			},
		},
	}

	if err := setEtcdBackupJobCommands(task, &job.Spec.Template.Spec.Containers[0].Command); err != nil {
		return nil, fmt.Errorf("could not set container commands for job %v: %w", kutil.ObjectName(job), err)
	}

	volumes, err := getVolumes(&sourceStore, sourceProvider, "source-")
	if err != nil {
		return nil, err
	}
	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, volumes...)

	volumes, err = getVolumes(&targetStore, targetProvider, "target-")
	if err != nil {
		return nil, err
	}
	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, volumes...)

	if err := controllerutil.SetControllerReference(task, job, r.Scheme()); err != nil {
		return nil, fmt.Errorf("could not set owner reference for job %v: %w", kutil.ObjectName(job), err)
	}
	return job, nil
}

func getVolumeNamePrefix(prefix string) string {
	switch prefix {
	case "source-":
		return prefix
	default:
		return ""
	}
}

func getVolumes(store *druidv1alpha1.StoreSpec, provider, prefix string) (volumes []corev1.Volume, err error) {
	switch provider {
	case druidutils.Local:
	        hostPathDirectory := corev1.HostPathDirectory
		volumes = append(volumes, corev1.Volume{
			Name: prefix + "host-storage",
			VolumeSource: corev1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
				        Path: *store.Container,
				        Type: &hostPathDirectory,
				},
			},
		})
	case druidutils.GCS, druidutils.S3, druidutils.ABS, druidutils.Swift, druidutils.OCS, druidutils.OSS:
		if store.SecretRef == nil {
			err = fmt.Errorf("no secretRef is configured for backup %sstore", prefix)
			return
		}
		volumes = append(volumes, corev1.Volume{
			Name: getVolumeNamePrefix(prefix) + "etcd-backup",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: store.SecretRef.Name,
				},
			},
		})

	}
	return
}

func getVolumeMounts(store *druidv1alpha1.StoreSpec, provider, volumeMountPrefix string) (volumeMounts []corev1.VolumeMount) {
	switch provider {
	case druidutils.Local:
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volumeMountPrefix + "host-storage",
			MountPath: *store.Container,
		})
	case druidutils.GCS:
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      getVolumeNamePrefix(volumeMountPrefix) + "etcd-backup",
			MountPath: "/root/." + getVolumeNamePrefix(volumeMountPrefix) + "gcp/",
		})
	case druidutils.S3, druidutils.ABS, druidutils.Swift, druidutils.OCS, druidutils.OSS:
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      getVolumeNamePrefix(volumeMountPrefix) + "etcd-backup",
			MountPath: "/root/" + getVolumeNamePrefix(volumeMountPrefix) + "etcd-backup/",
		})
	}
	return
}

func getEnvVarFromValues(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}

func getProviderEnvs(store *druidv1alpha1.StoreSpec, storeProvider, prefix, volumePrefix string) []corev1.EnvVar {
	env := []corev1.EnvVar{getEnvVarFromValues(prefix+"STORAGE_CONTAINER", *store.Container)}
	switch storeProvider {
	case druidutils.S3:
		env = append(env, getEnvVarFromValues(prefix+"AWS_APPLICATION_CREDENTIALS", "/root/"+volumePrefix+"etcd-backup"))
	case druidutils.ABS:
		env = append(env, getEnvVarFromValues(prefix+"AZURE_APPLICATION_CREDENTIALS", "/root/"+volumePrefix+"etcd-backup"))
	case druidutils.GCS:
		env = append(env, getEnvVarFromValues(prefix+"GOOGLE_APPLICATION_CREDENTIALS", "/root/."+volumePrefix+"gcp/serviceaccount.json"))
	case druidutils.Swift:
		env = append(env, getEnvVarFromValues(prefix+"OPENSTACK_APPLICATION_CREDENTIALS", "/root/"+volumePrefix+"etcd-backup"))
	case druidutils.OCS:
		env = append(env, getEnvVarFromValues(prefix+"OPENSHIFT_APPLICATION_CREDENTIALS", "/root/"+volumePrefix+"etcd-backup"))
	case druidutils.OSS:
		env = append(env, getEnvVarFromValues(prefix+"ALICLOUD_APPLICATION_CREDENTIALS", "/root/"+volumePrefix+"etcd-backup"))
	}
	return env
}

func setEtcdBackupJobCommands(task *druidv1alpha1.EtcdCopyBackupsTask, command *[]string) error {
	provider, err := druidutils.StorageProviderFromInfraProvider(task.Spec.TargetStore.Provider)
	if err != nil {
		return err
	}

	if len(provider) > 0 {
		*command = append(*command, "--storage-provider="+provider)
	}

	if len(task.Spec.TargetStore.Prefix) > 0 {
		*command = append(*command, "--store-prefix="+task.Spec.TargetStore.Prefix)
	}

	if task.Spec.TargetStore.Container != nil && len(*task.Spec.TargetStore.Container) > 0 {
		*command = append(*command, "--store-container="+*task.Spec.TargetStore.Container)
	}

	provider, err = druidutils.StorageProviderFromInfraProvider(task.Spec.SourceStore.Provider)
	if err != nil {
		return err
	}

	if len(provider) > 0 {
		*command = append(*command, "--source-storage-provider="+provider)
	}

	if len(task.Spec.SourceStore.Prefix) > 0 {
		*command = append(*command, "--source-store-prefix="+task.Spec.SourceStore.Prefix)
	}

	if task.Spec.SourceStore.Container != nil && len(*task.Spec.SourceStore.Container) > 0 {
		*command = append(*command, "--source-store-container="+*task.Spec.SourceStore.Container)
	}

	if task.Spec.MaxBackupAge != nil && *task.Spec.MaxBackupAge != 0 {
		*command = append(*command, "--max-backup-age="+strconv.Itoa(int(*task.Spec.MaxBackupAge)))
	}

	if task.Spec.MaxBackups != nil && *task.Spec.MaxBackups != 0 {
		*command = append(*command, "--max-backups-to-copy="+strconv.Itoa(int(*task.Spec.MaxBackups)))
	}

	if task.Spec.WaitForFinalSnapshot != nil {
		*command = append(*command, "--wait-for-final-snapshot="+strconv.FormatBool(task.Spec.WaitForFinalSnapshot.Enabled))
		if task.Spec.WaitForFinalSnapshot.Timeout != nil {
			*command = append(*command, "--wait-for-final-snapshot-timeout="+task.Spec.WaitForFinalSnapshot.Timeout.Duration.String())
		}
	}
	return nil
}
