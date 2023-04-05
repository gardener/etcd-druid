// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package compaction

import (
	"context"
	"fmt"
	"strconv"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	ctrlutils "github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// DefaultETCDQuota is the default etcd quota.
	DefaultETCDQuota = 8 * 1024 * 1024 * 1024 // 8Gi
)

// Reconciler reconciles compaction jobs for Etcd resources.
type Reconciler struct {
	client.Client
	config      *Config
	imageVector imagevector.ImageVector
	logger      logr.Logger
}

// NewReconciler creates a new reconciler for Compaction
func NewReconciler(mgr manager.Manager, config *Config) (*Reconciler, error) {
	imageVector, err := ctrlutils.CreateImageVector()
	if err != nil {
		return nil, err
	}
	return NewReconcilerWithImageVector(mgr, config, imageVector), nil
}

// NewReconcilerWithImageVector creates a new reconciler for Compaction with an ImageVector.
// This constructor will mostly be used by tests.
func NewReconcilerWithImageVector(mgr manager.Manager, config *Config, imageVector imagevector.ImageVector) *Reconciler {
	return &Reconciler{
		Client:      mgr.GetClient(),
		config:      config,
		imageVector: imageVector,
		logger:      log.Log.WithName("compaction-lease-controller"),
	}
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;create;list;watch;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=list;watch;delete;get

// Reconcile reconciles the compaction job.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger.Info("Lease controller reconciliation started")
	etcd := &druidv1alpha1.Etcd{}
	if err := r.Get(ctx, req.NamespacedName, etcd); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, err
	}

	if !etcd.DeletionTimestamp.IsZero() {
		// Delete compaction job if exists
		return r.delete(ctx, r.logger, etcd)
	}

	if etcd.Spec.Backup.Store == nil {
		return ctrl.Result{}, nil
	}

	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String())

	// Get full and delta snapshot lease to check the HolderIdentity value to take decision on compaction job
	fullLease := &coordinationv1.Lease{}

	if err := r.Get(ctx, kutil.Key(etcd.Namespace, etcd.GetFullSnapshotLeaseName()), fullLease); err != nil {
		logger.Info("Couldn't fetch full snap lease because: " + err.Error())

		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, err
	}

	deltaLease := &coordinationv1.Lease{}
	if err := r.Get(ctx, kutil.Key(etcd.Namespace, etcd.GetDeltaSnapshotLeaseName()), deltaLease); err != nil {
		logger.Info("Couldn't fetch delta snap lease because: " + err.Error())

		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, err
	}

	// Revisions have not been set yet by etcd-back-restore container.
	// Skip further processing as we cannot calculate a revision delta.
	if fullLease.Spec.HolderIdentity == nil || deltaLease.Spec.HolderIdentity == nil {
		return ctrl.Result{}, nil
	}

	full, err := strconv.ParseInt(*fullLease.Spec.HolderIdentity, 10, 64)
	if err != nil {
		logger.Error(err, "Can't convert holder identity of full snap lease to integer")
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, err
	}

	delta, err := strconv.ParseInt(*deltaLease.Spec.HolderIdentity, 10, 64)
	if err != nil {
		logger.Error(err, "Can't convert holder identity of delta snap lease to integer")
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, err
	}

	diff := delta - full

	// Reconcile job only when number of accumulated revisions over the last full snapshot is more than the configured threshold value via 'events-threshold' flag
	if diff >= r.config.EventsThreshold {
		return r.reconcileJob(ctx, logger, etcd)
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileJob(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	logger.Info("Reconcile etcd compaction job")

	// First check if a job is already running
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: etcd.GetCompactionJobName(), Namespace: etcd.Namespace}, job)

	if err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, fmt.Errorf("error while fetching compaction job: %v", err)
		}

		if r.config.EnableBackupCompaction {
			// Required job doesn't exist. Create new
			job, err = r.createCompactionJob(ctx, logger, etcd)
			logger.Info("Job Creation")
			if err != nil {
				return ctrl.Result{
					RequeueAfter: 10 * time.Second,
				}, fmt.Errorf("error during compaction job creation: %v", err)
			}
		}
	}

	if !job.DeletionTimestamp.IsZero() {
		logger.Info(fmt.Sprintf("Job %s/%s is already in deletion. A new job will only be created if the previous one has been deleted.", job.Namespace, job.Name))
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, nil
	}

	if job.Name != "" {
		logger.Info(fmt.Sprintf("Current compaction job is %s/%s , status: %d", job.Namespace, job.Name, job.Status.Succeeded))
	}

	// Delete job and requeue if the job failed
	if job.Status.Failed > 0 {
		err = r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil {
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, fmt.Errorf("error while deleting failed compaction job: %v", err)
		}
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, nil
	}

	// Delete job and return if the job succeeded
	if job.Status.Succeeded > 0 {
		return r.delete(ctx, logger, etcd)
	}

	return ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) delete(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: etcd.GetCompactionJobName(), Namespace: etcd.Namespace}, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, fmt.Errorf("error while fetching compaction job: %v", err)
		}
		return ctrl.Result{Requeue: false}, nil
	}

	if job.DeletionTimestamp == nil {
		logger.Info("Deleting job", "job", kutil.ObjectName(job))
		if err := client.IgnoreNotFound(r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground))); err != nil {
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, fmt.Errorf("error while deleting compaction job: %v", err)
		}
	}

	logger.Info("No compaction job is running")
	return ctrl.Result{
		Requeue: false,
	}, nil
}

func (r *Reconciler) createCompactionJob(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (*batchv1.Job, error) {
	activeDeadlineSeconds := r.config.ActiveDeadlineDuration.Seconds()

	_, etcdBackupImage, err := utils.GetEtcdImages(etcd, r.imageVector)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch etcd backup image: %v", err)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.GetCompactionJobName(),
			Namespace: etcd.Namespace,
			Labels:    getLabels(etcd),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "druid.gardener.cloud/v1alpha1",
					BlockOwnerDeletion: pointer.Bool(true),
					Controller:         pointer.Bool(true),
					Kind:               "Etcd",
					Name:               etcd.Name,
					UID:                etcd.UID,
				},
			},
		},

		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: pointer.Int64(int64(activeDeadlineSeconds)),
			Completions:           pointer.Int32(1),
			BackoffLimit:          pointer.Int32(0),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: etcd.Spec.Annotations,
					Labels:      getLabels(etcd),
				},
				Spec: v1.PodSpec{
					ActiveDeadlineSeconds: pointer.Int64(int64(activeDeadlineSeconds)),
					ServiceAccountName:    etcd.GetServiceAccountName(),
					RestartPolicy:         v1.RestartPolicyNever,
					Containers: []v1.Container{{
						Name:            "compact-backup",
						Image:           *etcdBackupImage,
						ImagePullPolicy: v1.PullIfNotPresent,
						Command:         getCompactionJobCommands(etcd),
						VolumeMounts:    getCompactionJobVolumeMounts(etcd, logger),
						Env:             getCompactionJobEnvVar(etcd, logger),
					}},
					Volumes: getCompactionJobVolumes(etcd, logger),
				},
			},
		},
	}

	if etcd.Spec.Backup.CompactionResources != nil {
		job.Spec.Template.Spec.Containers[0].Resources = *etcd.Spec.Backup.CompactionResources
	}

	logger.Info("Creating job", "job", kutil.Key(job.Namespace, job.Name).String())
	err = r.Create(ctx, job)
	if err != nil {
		return nil, err
	}

	//TODO (abdasgupta): Evaluate necessity of claiming object here after creation
	return job, nil
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	return map[string]string{
		"name":                             "etcd-backup-compaction",
		"instance":                         etcd.Name,
		"gardener.cloud/role":              "controlplane",
		"networking.gardener.cloud/to-dns": "allowed",
		"networking.gardener.cloud/to-private-networks": "allowed",
		"networking.gardener.cloud/to-public-networks":  "allowed",
	}
}
func getCompactionJobVolumeMounts(etcd *druidv1alpha1.Etcd, logger logr.Logger) []v1.VolumeMount {
	vms := []v1.VolumeMount{
		{
			Name:      "etcd-workspace-dir",
			MountPath: "/var/etcd/data",
		},
	}

	if etcd.Spec.Backup.Store == nil {
		return vms
	}

	provider, err := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
	if err != nil {
		logger.Info("Storage provider is not recognized. Compaction job will not mount any volume with provider specific credentials.")
		return vms
	}

	if provider == utils.GCS {
		vms = append(vms, v1.VolumeMount{
			Name:      "etcd-backup",
			MountPath: "/root/.gcp/",
		})
	} else if provider == utils.S3 || provider == utils.ABS || provider == utils.OSS || provider == utils.Swift || provider == utils.OCS {
		vms = append(vms, v1.VolumeMount{
			Name:      "etcd-backup",
			MountPath: "/root/etcd-backup/",
		})
	}

	return vms
}

func getCompactionJobVolumes(etcd *druidv1alpha1.Etcd, logger logr.Logger) []v1.Volume {
	vs := []v1.Volume{
		{
			Name: "etcd-workspace-dir",
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
	}

	if etcd.Spec.Backup.Store == nil {
		return vs
	}

	storeValues := etcd.Spec.Backup.Store
	provider, err := utils.StorageProviderFromInfraProvider(storeValues.Provider)
	if err != nil {
		logger.Info("Storage provider is not recognized. Compaction job will fail as no storage could be configured.")
		return vs
	}

	if provider == utils.GCS || provider == utils.S3 || provider == utils.OSS || provider == utils.ABS || provider == utils.Swift || provider == utils.OCS {
		if storeValues.SecretRef == nil {
			logger.Info("No secretRef is configured for backup store. Compaction job will fail as no storage could be configured.")
			return vs
		}

		vs = append(vs, v1.Volume{
			Name: "etcd-backup",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: storeValues.SecretRef.Name,
				},
			},
		})
	}

	return vs
}

func getCompactionJobEnvVar(etcd *druidv1alpha1.Etcd, logger logr.Logger) []v1.EnvVar {
	var env []v1.EnvVar
	if etcd.Spec.Backup.Store == nil {
		return env
	}

	storeValues := etcd.Spec.Backup.Store

	env = append(env, getEnvVarFromValues("STORAGE_CONTAINER", *storeValues.Container))
	env = append(env, getEnvVarFromFields("POD_NAMESPACE", "metadata.namespace"))

	provider, err := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
	if err != nil {
		logger.Info("Storage provider is not recognized. Compaction job will likely fail as there is no provider specific credentials.")
		return env
	}

	switch provider {
	case utils.S3:
		env = append(env, getEnvVarFromValues("AWS_APPLICATION_CREDENTIALS", "/root/etcd-backup"))
	case utils.ABS:
		env = append(env, getEnvVarFromValues("AZURE_APPLICATION_CREDENTIALS", "/root/etcd-backup"))
	case utils.GCS:
		env = append(env, getEnvVarFromValues("GOOGLE_APPLICATION_CREDENTIALS", "/root/.gcp/serviceaccount.json"))
	case utils.Swift:
		env = append(env, getEnvVarFromValues("OPENSTACK_APPLICATION_CREDENTIALS", "/root/etcd-backup"))
	case utils.OSS:
		env = append(env, getEnvVarFromValues("ALICLOUD_APPLICATION_CREDENTIALS", "/root/etcd-backup"))
	case utils.ECS:
		if storeValues.SecretRef == nil {
			logger.Info("No secretRef is configured for backup store. Compaction job will fail as no storage could be configured.")
			return env
		}

		env = append(env, getEnvVarFromSecrets("ECS_ENDPOINT", storeValues.SecretRef.Name, "endpoint"))
		env = append(env, getEnvVarFromSecrets("ECS_ACCESS_KEY_ID", storeValues.SecretRef.Name, "accessKeyID"))
		env = append(env, getEnvVarFromSecrets("ECS_SECRET_ACCESS_KEY", storeValues.SecretRef.Name, "secretAccessKey"))
	case utils.OCS:
		env = append(env, getEnvVarFromValues("OPENSHIFT_APPLICATION_CREDENTIALS", "/root/etcd-backup"))
	}

	return env
}

func getEnvVarFromValues(name, value string) v1.EnvVar {
	return v1.EnvVar{
		Name:  name,
		Value: value,
	}
}

func getEnvVarFromFields(name, fieldPath string) v1.EnvVar {
	return v1.EnvVar{
		Name: name,
		ValueFrom: &v1.EnvVarSource{
			FieldRef: &v1.ObjectFieldSelector{
				FieldPath: fieldPath,
			},
		},
	}
}

func getEnvVarFromSecrets(name, secretName, secretKey string) v1.EnvVar {
	return v1.EnvVar{
		Name: name,
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{
					Name: secretName,
				},
				Key: secretKey,
			},
		},
	}
}

func getCompactionJobCommands(etcd *druidv1alpha1.Etcd) []string {
	command := []string{"" + "etcdbrctl"}
	command = append(command, "compact")
	command = append(command, "--data-dir=/var/etcd/data/compaction.etcd")
	command = append(command, "--restoration-temp-snapshots-dir=/var/etcd/compaction.restoration.temp")
	command = append(command, "--snapstore-temp-directory=/var/etcd/data/tmp")
	command = append(command, "--enable-snapshot-lease-renewal=true")
	command = append(command, "--full-snapshot-lease-name="+etcd.GetFullSnapshotLeaseName())
	command = append(command, "--delta-snapshot-lease-name="+etcd.GetDeltaSnapshotLeaseName())

	var quota int64 = DefaultETCDQuota
	if etcd.Spec.Etcd.Quota != nil {
		quota = etcd.Spec.Etcd.Quota.Value()
	}
	command = append(command, "--embedded-etcd-quota-bytes="+fmt.Sprint(quota))

	if etcd.Spec.Etcd.EtcdDefragTimeout != nil {
		command = append(command, "--etcd-defrag-timeout="+etcd.Spec.Etcd.EtcdDefragTimeout.Duration.String())
	}

	backupValues := etcd.Spec.Backup
	if backupValues.EtcdSnapshotTimeout != nil {
		command = append(command, "--etcd-snapshot-timeout="+backupValues.EtcdSnapshotTimeout.Duration.String())
	}
	storeValues := etcd.Spec.Backup.Store
	if storeValues != nil {
		provider, err := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
		if err == nil {
			command = append(command, "--storage-provider="+provider)
		}

		if storeValues.Prefix != "" {
			command = append(command, "--store-prefix="+storeValues.Prefix)
		}

		if storeValues.Container != nil {
			command = append(command, "--store-container="+*(storeValues.Container))
		}
	}

	return command
}
