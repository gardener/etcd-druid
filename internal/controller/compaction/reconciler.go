// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"fmt"
	"strconv"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/features"
	"github.com/gardener/etcd-druid/internal/images"
	druidmetrics "github.com/gardener/etcd-druid/internal/metrics"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/gardener/gardener/pkg/utils/imagevector"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
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
	imageVector, err := images.CreateImageVector()
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
	r.logger.Info("Compaction job reconciliation started")
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

	if !etcd.DeletionTimestamp.IsZero() || etcd.Spec.Backup.Store == nil {
		// Delete compaction job if exists
		return r.delete(ctx, r.logger, etcd)
	}

	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String())

	return r.reconcileJob(ctx, logger, etcd)
}

func (r *Reconciler) reconcileJob(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	// Update metrics for currently running compaction job, if any
	job := &batchv1.Job{}
	compactionJobName := druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta)
	if err := r.Get(ctx, types.NamespacedName{Name: compactionJobName, Namespace: etcd.Namespace}, job); err != nil {
		if !errors.IsNotFound(err) {
			// Error reading the object - requeue the request.
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, fmt.Errorf("error while fetching compaction job with the name %v, in the namespace %v: %v", compactionJobName, etcd.Namespace, err)
		}
		logger.Info("No compaction job currently running", "namespace", etcd.Namespace)
	}

	if job != nil && job.Name != "" {
		if !job.DeletionTimestamp.IsZero() {
			logger.Info("Job is already in deletion. A new job will be created only if the previous one has been deleted.", "namespace: ", job.Namespace, "name: ", job.Name)
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, nil
		}

		// Check if there is one active job or not
		if job.Status.Active > 0 {
			metricJobsCurrent.With(prometheus.Labels{druidmetrics.EtcdNamespace: etcd.Namespace}).Set(1)
			// Don't need to requeue if the job is currently running
			return ctrl.Result{}, nil
		}

		// Delete job if the job succeeded
		if job.Status.Succeeded > 0 {
			metricJobsCurrent.With(prometheus.Labels{druidmetrics.EtcdNamespace: etcd.Namespace}).Set(0)
			if job.Status.CompletionTime != nil {
				metricJobDurationSeconds.With(prometheus.Labels{druidmetrics.LabelSucceeded: druidmetrics.ValueSucceededTrue, druidmetrics.EtcdNamespace: etcd.Namespace}).Observe(job.Status.CompletionTime.Time.Sub(job.Status.StartTime.Time).Seconds())
			}
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
				logger.Error(err, "Couldn't delete the successful job", "namespace", etcd.Namespace, "name", compactionJobName)
				return ctrl.Result{
					RequeueAfter: 10 * time.Second,
				}, fmt.Errorf("error while deleting successful compaction job: %v", err)
			}
			metricJobsTotal.With(prometheus.Labels{druidmetrics.LabelSucceeded: druidmetrics.ValueSucceededTrue, druidmetrics.EtcdNamespace: etcd.Namespace}).Inc()
		}

		// Delete job and requeue if the job failed
		if job.Status.Failed > 0 {
			metricJobsCurrent.With(prometheus.Labels{druidmetrics.EtcdNamespace: etcd.Namespace}).Set(0)
			if job.Status.StartTime != nil {
				metricJobDurationSeconds.With(prometheus.Labels{druidmetrics.LabelSucceeded: druidmetrics.ValueSucceededFalse, druidmetrics.EtcdNamespace: etcd.Namespace}).Observe(time.Since(job.Status.StartTime.Time).Seconds())
			}
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
				return ctrl.Result{
					RequeueAfter: 10 * time.Second,
				}, fmt.Errorf("error while deleting failed compaction job: %v", err)
			}
			metricJobsTotal.With(prometheus.Labels{druidmetrics.LabelSucceeded: druidmetrics.ValueSucceededFalse, druidmetrics.EtcdNamespace: etcd.Namespace}).Inc()
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, nil
		}
	}

	// Get full and delta snapshot lease to check the HolderIdentity value to take decision on compaction job
	fullLease := &coordinationv1.Lease{}
	fullSnapshotLeaseName := druidv1alpha1.GetFullSnapshotLeaseName(etcd.ObjectMeta)
	if err := r.Get(ctx, kutil.Key(etcd.Namespace, fullSnapshotLeaseName), fullLease); err != nil {
		logger.Error(err, "Couldn't fetch full snap lease", "namespace", etcd.Namespace, "name", fullSnapshotLeaseName)
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, err
	}

	deltaLease := &coordinationv1.Lease{}
	deltaSnapshotLeaseName := druidv1alpha1.GetDeltaSnapshotLeaseName(etcd.ObjectMeta)
	if err := r.Get(ctx, kutil.Key(etcd.Namespace, deltaSnapshotLeaseName), deltaLease); err != nil {
		logger.Error(err, "Couldn't fetch delta snap lease", "namespace", etcd.Namespace, "name", deltaSnapshotLeaseName)
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
		logger.Error(err, "Can't convert holder identity of full snap lease to integer",
			"namespace", fullLease.Namespace, "leaseName", fullLease.Name, "holderIdentity", fullLease.Spec.HolderIdentity)
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, err
	}

	delta, err := strconv.ParseInt(*deltaLease.Spec.HolderIdentity, 10, 64)
	if err != nil {
		logger.Error(err, "Can't convert holder identity of delta snap lease to integer",
			"namespace", deltaLease.Namespace, "leaseName", deltaLease.Name, "holderIdentity", deltaLease.Spec.HolderIdentity)
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, err
	}

	diff := delta - full
	metricNumDeltaEvents.With(prometheus.Labels{druidmetrics.EtcdNamespace: etcd.Namespace}).Set(float64(diff))

	// Reconcile job only when number of accumulated revisions over the last full snapshot is more than the configured threshold value via 'events-threshold' flag
	if diff >= r.config.EventsThreshold {
		logger.Info("Creating etcd compaction job", "namespace", etcd.Namespace, "name", compactionJobName)
		job, err = r.createCompactionJob(ctx, logger, etcd)
		if err != nil {
			metricJobsTotal.With(prometheus.Labels{druidmetrics.LabelSucceeded: druidmetrics.ValueSucceededFalse, druidmetrics.EtcdNamespace: etcd.Namespace}).Inc()
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, fmt.Errorf("error during compaction job creation: %v", err)
		}
		metricJobsCurrent.With(prometheus.Labels{druidmetrics.EtcdNamespace: etcd.Namespace}).Set(1)
	}

	if job.Name != "" {
		logger.Info("Current compaction job status",
			"namespace", job.Namespace, "name", job.Name, "succeeded", job.Status.Succeeded)
	}

	return ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) delete(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta), Namespace: etcd.Namespace}, job)
	if err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, fmt.Errorf("error while fetching compaction job: %v", err)
		}
		return ctrl.Result{Requeue: false}, nil
	}

	if job.DeletionTimestamp == nil {
		logger.Info("Deleting job", "namespace", job.Namespace, "name", job.Name)
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

	_, etcdBackupImage, _, err := utils.GetEtcdImages(etcd, r.imageVector, r.config.FeatureGates[features.UseEtcdWrapper])
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch etcd backup image: %v", err)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta),
			Namespace: etcd.Namespace,
			Labels:    getLabels(etcd),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         druidv1alpha1.GroupVersion.String(),
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
					ServiceAccountName:    druidv1alpha1.GetServiceAccountName(etcd.ObjectMeta),
					RestartPolicy:         v1.RestartPolicyNever,
					Containers: []v1.Container{{
						Name:            "compact-backup",
						Image:           etcdBackupImage,
						ImagePullPolicy: v1.PullIfNotPresent,
						Args:            getCompactionJobArgs(etcd, r.config.MetricsScrapeWaitDuration.String()),
					}},
				},
			},
		},
	}

	if vms, err := getCompactionJobVolumeMounts(etcd, r.config.FeatureGates); err != nil {
		return nil, fmt.Errorf("error while creating compaction job in %v for %v : %v",
			etcd.Namespace,
			etcd.Name,
			err)
	} else {
		job.Spec.Template.Spec.Containers[0].VolumeMounts = vms
	}

	if env, err := utils.GetBackupRestoreContainerEnvVars(etcd.Spec.Backup.Store); err != nil {
		return nil, fmt.Errorf("error while creating compaction job in %v for %v : %v",
			etcd.Namespace,
			etcd.Name,
			err)
	} else {
		job.Spec.Template.Spec.Containers[0].Env = env
	}

	if vm, err := getCompactionJobVolumes(ctx, r.Client, r.logger, etcd); err != nil {
		return nil, fmt.Errorf("error creating compaction job in %v for %v : %v",
			etcd.Namespace,
			etcd.Name,
			err)
	} else {
		job.Spec.Template.Spec.Volumes = vm
	}

	if etcd.Spec.Backup.CompactionResources != nil {
		job.Spec.Template.Spec.Containers[0].Resources = *etcd.Spec.Backup.CompactionResources
	}

	logger.Info("Creating job", "namespace", job.Namespace, "name", job.Name)
	err = r.Create(ctx, job)
	if err != nil {
		return nil, err
	}

	//TODO (abdasgupta): Evaluate necessity of claiming object here after creation
	return job, nil
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	jobLabels := map[string]string{
		druidv1alpha1.LabelAppNameKey:                   druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta),
		druidv1alpha1.LabelComponentKey:                 common.ComponentNameCompactionJob,
		"networking.gardener.cloud/to-dns":              "allowed",
		"networking.gardener.cloud/to-private-networks": "allowed",
		"networking.gardener.cloud/to-public-networks":  "allowed",
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), jobLabels)
}

func getCompactionJobVolumeMounts(etcd *druidv1alpha1.Etcd, featureMap map[featuregate.Feature]bool) ([]v1.VolumeMount, error) {
	vms := []v1.VolumeMount{
		{
			Name:      "etcd-workspace-dir",
			MountPath: common.VolumeMountPathEtcdData,
		},
	}

	provider, err := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
	if err != nil {
		return vms, fmt.Errorf("storage provider is not recognized while fetching volume mounts")
	}
	switch provider {
	case utils.Local:
		if featureMap[features.UseEtcdWrapper] {
			vms = append(vms, v1.VolumeMount{
				Name:      "host-storage",
				MountPath: "/home/nonroot/" + pointer.StringDeref(etcd.Spec.Backup.Store.Container, ""),
			})
		} else {
			vms = append(vms, v1.VolumeMount{
				Name:      "host-storage",
				MountPath: pointer.StringDeref(etcd.Spec.Backup.Store.Container, ""),
			})
		}
	case utils.GCS:
		vms = append(vms, v1.VolumeMount{
			Name:      common.VolumeNameProviderBackupSecret,
			MountPath: common.VolumeMountPathGCSBackupSecret,
		})
	case utils.S3, utils.ABS, utils.OSS, utils.Swift, utils.OCS:
		vms = append(vms, v1.VolumeMount{
			Name:      common.VolumeNameProviderBackupSecret,
			MountPath: common.VolumeMountPathNonGCSProviderBackupSecret,
		})
	}

	return vms, nil
}

func getCompactionJobVolumes(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd) ([]v1.Volume, error) {
	vs := []v1.Volume{
		{
			Name: "etcd-workspace-dir",
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
	}

	storeValues := etcd.Spec.Backup.Store
	provider, err := utils.StorageProviderFromInfraProvider(storeValues.Provider)
	if err != nil {
		return vs, fmt.Errorf("could not recognize storage provider while fetching volumes")
	}
	switch provider {
	case "Local":
		hostPath, err := utils.GetHostMountPathFromSecretRef(ctx, cl, logger, storeValues, etcd.Namespace)
		if err != nil {
			return vs, fmt.Errorf("could not determine host mount path for local provider")
		}

		hpt := v1.HostPathDirectory
		vs = append(vs, v1.Volume{
			Name: "host-storage",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: hostPath + "/" + pointer.StringDeref(storeValues.Container, ""),
					Type: &hpt,
				},
			},
		})
	case utils.GCS, utils.S3, utils.OSS, utils.ABS, utils.Swift, utils.OCS:
		if storeValues.SecretRef == nil {
			return vs, fmt.Errorf("could not configure secretRef for backup store %v", provider)
		}

		vs = append(vs, v1.Volume{
			Name: common.VolumeNameProviderBackupSecret,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: storeValues.SecretRef.Name,
				},
			},
		})
	}

	return vs, nil
}

func getCompactionJobArgs(etcd *druidv1alpha1.Etcd, metricsScrapeWaitDuration string) []string {
	command := []string{"compact"}
	command = append(command, "--data-dir=/var/etcd/data/compaction.etcd")
	command = append(command, "--restoration-temp-snapshots-dir=/var/etcd/data/compaction.restoration.temp")
	command = append(command, "--snapstore-temp-directory=/var/etcd/data/tmp")
	command = append(command, "--metrics-scrape-wait-duration="+metricsScrapeWaitDuration)
	command = append(command, "--enable-snapshot-lease-renewal=true")
	command = append(command, "--full-snapshot-lease-name="+druidv1alpha1.GetFullSnapshotLeaseName(etcd.ObjectMeta))
	command = append(command, "--delta-snapshot-lease-name="+druidv1alpha1.GetDeltaSnapshotLeaseName(etcd.ObjectMeta))

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
