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
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/pkg/features"
	druidutils "github.com/gardener/etcd-druid/pkg/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
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

const (
	sourcePrefix = "source-"
	targetPrefix = "target-"
)

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
	imageVector, err := utils.CreateImageVector(config.FeatureGates[string(features.UseEtcdWrapper)])
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
	logger := r.logger.WithValues("etcdCopyBackupsTask", kutil.ObjectName(task), "operation", "reconcile")

	// Ensure finalizer
	if !controllerutil.ContainsFinalizer(task, common.FinalizerName) {
		logger.V(1).Info("Adding finalizer", "finalizerName", common.FinalizerName)
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
	job, err = r.createJobObject(ctx, task)
	if err != nil {
		return status, err
	}

	// Create job
	logger.Info("Creating job", "namespace", job.Namespace, "name", job.Name)
	if err := r.Create(ctx, job); err != nil {
		return status, fmt.Errorf("could not create job %s: %w", kutil.ObjectName(job), err)
	}

	return status, nil
}

func (r *Reconciler) delete(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask) (result ctrl.Result, err error) {
	logger := r.logger.WithValues("task", kutil.ObjectName(task), "operation", "delete")

	// Check finalizer
	if !controllerutil.ContainsFinalizer(task, common.FinalizerName) {
		logger.V(1).Info("Skipping since finalizer not present", "finalizerName", common.FinalizerName)
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
		logger.V(1).Info("Removing finalizer", "finalizerName", common.FinalizerName)
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
		logger.Info("Deleting job", "namespace", job.Namespace, "name", job.Name)
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground)); client.IgnoreNotFound(err) != nil {
			return status, false, fmt.Errorf("could not delete job %s: %w", kutil.ObjectName(job), err)
		}
	}

	return status, false, nil
}

func (r *Reconciler) getJob(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	if err := r.Get(ctx, kutil.Key(task.Namespace, task.GetJobName()), job); err != nil {
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

func (r *Reconciler) createJobObject(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask) (*batchv1.Job, error) {
	etcdBackupImage, err := druidutils.GetEtcdBackupRestoreImage(r.imageVector)
	if err != nil {
		return nil, err
	}

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

	// Formulate the job's arguments.
	args := createJobArgs(task, sourceProvider, targetProvider)

	// Formulate the job environment variables.
	env := append(createEnvVarsFromStore(&sourceStore, sourceProvider, "SOURCE_", sourcePrefix), createEnvVarsFromStore(&targetStore, targetProvider, "", "")...)

	// Formulate the job's volume mounts.
	volumeMounts := append(createVolumeMountsFromStore(&sourceStore, sourceProvider, sourcePrefix), createVolumeMountsFromStore(&targetStore, targetProvider, targetPrefix)...)

	// Formulate the job's volumes from the source store.
	sourceVolumes, err := r.createVolumesFromStore(ctx, &sourceStore, task.Namespace, sourceProvider, sourcePrefix)
	if err != nil {
		return nil, err
	}

	// Formulate the job's volumes from the target store.
	targetVolumes, err := r.createVolumesFromStore(ctx, &targetStore, task.Namespace, targetProvider, targetPrefix)
	if err != nil {
		return nil, err
	}

	// Combine the source and target volumes.
	volumes := append(sourceVolumes, targetVolumes...)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      task.GetJobName(),
			Namespace: task.Namespace,
			Annotations: map[string]string{
				common.GardenerOwnedBy:   client.ObjectKeyFromObject(task).String(),
				common.GardenerOwnerType: strings.ToLower(task.Kind),
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						v1beta1constants.LabelNetworkPolicyToDNS:            v1beta1constants.LabelNetworkPolicyAllowed,
						v1beta1constants.LabelNetworkPolicyToPublicNetworks: v1beta1constants.LabelNetworkPolicyAllowed,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:            "copy-backups",
							Image:           *etcdBackupImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            args,
							Env:             env,
							VolumeMounts:    volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(task, job, r.Scheme()); err != nil {
		return nil, fmt.Errorf("could not set owner reference for job %v: %w", kutil.ObjectName(job), err)
	}
	return job, nil
}

func createJobArgs(task *druidv1alpha1.EtcdCopyBackupsTask, sourceObjStoreProvider string, targetObjStoreProvider string) []string {
	// Create the initial arguments for the copy-backups job.
	args := []string{
		"copy",
		"--snapstore-temp-directory=/var/etcd/data/tmp",
	}

	// Formulate the job's arguments.
	args = append(args, createJobArgumentFromStore(&task.Spec.TargetStore, targetObjStoreProvider, "")...)
	args = append(args, createJobArgumentFromStore(&task.Spec.SourceStore, sourceObjStoreProvider, sourcePrefix)...)
	if task.Spec.MaxBackupAge != nil {
		args = append(args, "--max-backup-age="+strconv.Itoa(int(*task.Spec.MaxBackupAge)))
	}

	if task.Spec.MaxBackups != nil {
		args = append(args, "--max-backups-to-copy="+strconv.Itoa(int(*task.Spec.MaxBackups)))
	}

	if task.Spec.WaitForFinalSnapshot != nil {
		args = append(args, "--wait-for-final-snapshot="+strconv.FormatBool(task.Spec.WaitForFinalSnapshot.Enabled))
		if task.Spec.WaitForFinalSnapshot.Timeout != nil {
			args = append(args, "--wait-for-final-snapshot-timeout="+task.Spec.WaitForFinalSnapshot.Timeout.Duration.String())
		}
	}
	return args
}

// getVolumeNamePrefix returns the appropriate volume name prefix based on the provided prefix.
// If the provided prefix is "source-", it returns the prefix; otherwise, it returns an empty string.
func getVolumeNamePrefix(prefix string) string {
	if prefix == sourcePrefix {
		return prefix
	}
	return ""
}

// createVolumesFromStore generates a slice of VolumeMounts for an EtcdCopyBackups job based on the given StoreSpec and
// provider. The prefix is used to differentiate between source and target volume.
// This function creates the necessary Volume configurations for various storage providers.
func (r *Reconciler) createVolumesFromStore(ctx context.Context, store *druidv1alpha1.StoreSpec, namespace, provider, prefix string) (volumes []corev1.Volume, err error) {
	switch provider {
	case druidutils.Local:
		hostPathDirectory := corev1.HostPathDirectory
		hostPathPrefix, err := druidutils.GetHostMountPathFromSecretRef(ctx, r.Client, r.logger, store, namespace)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, corev1.Volume{
			Name: prefix + "host-storage",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPathPrefix + "/" + *store.Container,
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

// createVolumesFromStore generates a slice of volumes for an EtcdCopyBackups job based on the given StoreSpec, namespace,
// provider, and prefix. The prefix is used to differentiate between source and target volumes.
// This function creates the necessary Volume configurations for various storage providers and returns any errors encountered.
func createVolumeMountsFromStore(store *druidv1alpha1.StoreSpec, provider, volumeMountPrefix string) (volumeMounts []corev1.VolumeMount) {
	switch provider {
	case druidutils.Local:
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volumeMountPrefix + "host-storage",
			MountPath: *store.Container,
		})
	case druidutils.GCS:
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      getVolumeNamePrefix(volumeMountPrefix) + "etcd-backup",
			MountPath: "/var/." + getVolumeNamePrefix(volumeMountPrefix) + "gcp/",
		})
	case druidutils.S3, druidutils.ABS, druidutils.Swift, druidutils.OCS, druidutils.OSS:
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      getVolumeNamePrefix(volumeMountPrefix) + "etcd-backup",
			MountPath: "/var/" + getVolumeNamePrefix(volumeMountPrefix) + "etcd-backup/",
		})
	}
	return
}

func mapToEnvVar(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}

// createEnvVarsFromStore generates a slice of environment variables for an EtcdCopyBackups job based on the given StoreSpec,
// storeProvider, prefix, and volumePrefix. The prefix is used to differentiate between source and target environment variables.
// This function creates the necessary environment variables for various storage providers and configurations. The generated
// environment variables include storage container information and provider-specific credentials.
func createEnvVarsFromStore(store *druidv1alpha1.StoreSpec, storeProvider, envKeyPrefix, volumePrefix string) (envVars []corev1.EnvVar) {
	envVars = append(envVars, mapToEnvVar(envKeyPrefix+common.STORAGE_CONTAINER, *store.Container))
	switch storeProvider {
	case druidutils.S3:
		envVars = append(envVars, mapToEnvVar(envKeyPrefix+common.AWS_APPLICATION_CREDENTIALS, "/var/"+volumePrefix+"etcd-backup"))
	case druidutils.ABS:
		envVars = append(envVars, mapToEnvVar(envKeyPrefix+common.AZURE_APPLICATION_CREDENTIALS, "/var/"+volumePrefix+"etcd-backup"))
	case druidutils.GCS:
		envVars = append(envVars, mapToEnvVar(envKeyPrefix+common.GOOGLE_APPLICATION_CREDENTIALS, "/var/."+volumePrefix+"gcp/serviceaccount.json"))
	case druidutils.Swift:
		envVars = append(envVars, mapToEnvVar(envKeyPrefix+common.OPENSTACK_APPLICATION_CREDENTIALS, "/var/"+volumePrefix+"etcd-backup"))
	case druidutils.OCS:
		envVars = append(envVars, mapToEnvVar(envKeyPrefix+common.OPENSHIFT_APPLICATION_CREDENTIALS, "/var/"+volumePrefix+"etcd-backup"))
	case druidutils.OSS:
		envVars = append(envVars, mapToEnvVar(envKeyPrefix+common.ALICLOUD_APPLICATION_CREDENTIALS, "/var/"+volumePrefix+"etcd-backup"))
	}
	return envVars
}

// createJobArgumentFromStore generates a slice of command-line arguments for a EtcdCopyBackups job based on the given StoreSpec,
// provider, and prefix. The prefix is used to differentiate between source and target command-line arguments.
// This function is used to create the necessary command-line arguments for
// various storage providers and configurations. The generated arguments include storage provider,
// store prefix, and store container information.
func createJobArgumentFromStore(store *druidv1alpha1.StoreSpec, provider, prefix string) (arguments []string) {
	if store == nil || len(provider) == 0 {
		return
	}
	argPrefix := "--" + prefix
	arguments = append(arguments, argPrefix+"storage-provider="+provider)

	if len(store.Prefix) > 0 {
		arguments = append(arguments, argPrefix+"store-prefix="+store.Prefix)
	}

	if store.Container != nil && len(*store.Container) > 0 {
		arguments = append(arguments, argPrefix+"store-container="+*store.Container)
	}
	return
}
