// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strconv"
	"time"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/images"
	druidmetrics "github.com/gardener/etcd-druid/internal/metrics"
	druidstore "github.com/gardener/etcd-druid/internal/store"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/internal/utils/imagevector"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// DefaultETCDQuota is the default etcd quota.
	DefaultETCDQuota = 8 * 1024 * 1024 * 1024 // 8Gi

	// SafeToEvictKey - annotation that ignores constraints to evict a pod like not being replicated, being on
	// kube-system namespace or having a local storage if set to "false".
	SafeToEvictKey = "cluster-autoscaler.kubernetes.io/safe-to-evict"
)

const (
	podFailureReasonPreemptionByScheduler  = v1.PodReasonPreemptionByScheduler
	podFailureReasonDeletionByTaintManager = "DeletionByTaintManager"
	podFailureReasonEvictionByEvictionAPI  = "EvictionByEvictionAPI"
	podFailureReasonTerminationByKubelet   = v1.PodReasonTerminationByKubelet
	podFailureReasonProcessFailure         = "ProcessFailure"
	podFailureReasonUnknown                = "Unknown"
)

const (
	jobSucceeded int = iota
	jobFailed
)

var (
	// defaultCompactionJobCpuRequests defines the default cpu requests for the compaction job
	defaultCompactionJobCpuRequests = resource.MustParse("600m")
	// defaultCompactionJobMemoryRequests defines the default memory requests for the compaction job
	defaultCompactionJobMemoryRequests = resource.MustParse("3Gi")
	// lastJobCompletionReason is used to store the reason of the last completed job.
	lastJobCompletionReason *string
)

// FullSnapshotTriggerFunc defines a function type for triggering a full snapshot.
type FullSnapshotTriggerFunc func(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error

// Reconciler reconciles compaction jobs for Etcd resources.
type Reconciler struct {
	client.Client
	config              druidconfigv1alpha1.CompactionControllerConfiguration
	imageVector         imagevector.ImageVector
	logger              logr.Logger
	FullSnapshotTrigger FullSnapshotTriggerFunc
}

// NewReconciler creates a new reconciler for Compaction
func NewReconciler(mgr manager.Manager, config druidconfigv1alpha1.CompactionControllerConfiguration) (*Reconciler, error) {
	imageVector, err := images.CreateImageVector()
	if err != nil {
		return nil, err
	}
	return NewReconcilerWithImageVector(mgr, config, imageVector), nil
}

// NewReconcilerWithImageVector creates a new reconciler for Compaction with an ImageVector.
// This constructor will mostly be used by tests.
func NewReconcilerWithImageVector(mgr manager.Manager, config druidconfigv1alpha1.CompactionControllerConfiguration, imageVector imagevector.ImageVector) *Reconciler {
	reconciler := &Reconciler{
		Client:      mgr.GetClient(),
		config:      config,
		imageVector: imageVector,
		logger:      log.Log.WithName("compaction-lease-controller"),
	}
	reconciler.FullSnapshotTrigger = func(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
		return reconciler.triggerFullSnapshot(ctx, logger, etcd)
	}
	return reconciler
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

	if !etcd.DeletionTimestamp.IsZero() || !etcd.IsBackupStoreEnabled() {
		// Delete compaction job if exists
		return r.delete(ctx, r.logger, etcd)
	}

	logger := r.logger.WithValues("etcd", client.ObjectKeyFromObject(etcd).String())

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

	if job.Name != "" {
		if !job.DeletionTimestamp.IsZero() {
			logger.Info("Job is already in deletion. A new job will be created only if the previous one has been deleted.", "namespace: ", job.Namespace, "name: ", job.Name)
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, nil
		}

		// Check if there is one active job or not
		if job.Status.Active > 0 {
			metricJobsCurrent.With(prometheus.Labels{druidmetrics.LabelEtcdNamespace: etcd.Namespace}).Set(1)
			// Don't need to requeue if the job is currently running
			return ctrl.Result{}, nil
		}
		// Set the current job count to 0 if the job is not active
		metricJobsCurrent.With(prometheus.Labels{druidmetrics.LabelEtcdNamespace: etcd.Namespace}).Set(0)
		jobCompletionState, jobCompletionReason := getJobCompletionStateAndReason(job)
		if jobCompletionState == jobSucceeded {
			recordSuccessfulJobMetrics(job)
		} else {
			if err := r.recordFailedJobMetrics(ctx, logger, job, etcd, jobCompletionReason); err != nil {
				return ctrl.Result{RequeueAfter: 10 * time.Second}, fmt.Errorf("error while handling failed job: %w", err)
			}
		}
		// Delete the completed compaction job
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			logger.Error(err, "Couldn't delete the completed compaction job", "namespace", etcd.Namespace, "name", compactionJobName)
			return ctrl.Result{
				RequeueAfter: 10 * time.Second}, fmt.Errorf("error while deleting the completed compaction job: %w", err)
		}
		lastJobCompletionReason = &jobCompletionReason
	}
	// This block is added because the terminationGracePeriodSeconds is set to 60 seconds.
	// Even after the job is deleted, the associated pod may remain for some time.
	// During this period, we should not attempt to create a new compaction job to avoid conflicts.
	jobMeta := &metav1.ObjectMeta{
		Name:      compactionJobName,
		Namespace: etcd.Namespace,
	}
	pod, err := getPodForJob(ctx, r.Client, jobMeta)
	if err != nil {
		logger.Error(err, "Couldn't fetch pods for job", "namespace", job.Namespace, "name", job.Name)
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, fmt.Errorf("error while fetching pod for job: %v", err)
	}
	if pod != nil {
		logger.Info("Pod found for job, cannot create a new job until the existing one is completely cleaned", "namespace", pod.Namespace, "name", pod.Name)
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, nil
	}

	// Get full and delta snapshot lease to check the HolderIdentity value to take decision on compaction job
	fullLease := &coordinationv1.Lease{}
	fullSnapshotLeaseName := druidv1alpha1.GetFullSnapshotLeaseName(etcd.ObjectMeta)
	if err := r.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: fullSnapshotLeaseName}, fullLease); err != nil {
		logger.Error(err, "Couldn't fetch full snap lease", "namespace", etcd.Namespace, "name", fullSnapshotLeaseName)
		return ctrl.Result{
			RequeueAfter: 10 * time.Second,
		}, err
	}

	deltaLease := &coordinationv1.Lease{}
	deltaSnapshotLeaseName := druidv1alpha1.GetDeltaSnapshotLeaseName(etcd.ObjectMeta)
	if err := r.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: deltaSnapshotLeaseName}, deltaLease); err != nil {
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
	metricNumDeltaEvents.With(prometheus.Labels{druidmetrics.LabelEtcdNamespace: etcd.Namespace}).Set(float64(diff))
	return r.createCompactionJobOrTriggerFullSnapshot(ctx, logger, etcd, diff, compactionJobName, job)
}

func (r *Reconciler) delete(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	job := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta), Namespace: etcd.Namespace}, job); err != nil {
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

func (r *Reconciler) createCompactionJobOrTriggerFullSnapshot(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, accumulatedEtcdRevisions int64, compactionJobName string, job *batchv1.Job) (ctrl.Result, error) {
	var err error
	eventsThreshold := r.config.EventsThreshold
	triggerFullsnapshotThreshold := r.config.TriggerFullSnapshotThreshold
	if compactionSpec := etcd.Spec.Backup.SnapshotCompaction; compactionSpec != nil {
		if compactionSpec.EventsThreshold != nil {
			eventsThreshold = *compactionSpec.EventsThreshold
		}
		if compactionSpec.TriggerFullSnapshotThreshold != nil {
			triggerFullsnapshotThreshold = *compactionSpec.TriggerFullSnapshotThreshold
		}
	}

	// Trigger full snapshot if the delta revisions over the last full snapshot are more than the configured upper threshold
	// or if the last job completion reason is DeadlineExceeded.
	// This is to ensure that we avoid spinning up compaction jobs even when we know that the probability of it getting succeeded is very low due to the large number of revisions.
	// This avoids unnecessary resource consumption and delays in the compaction process
	if (lastJobCompletionReason != nil && *lastJobCompletionReason == batchv1.JobReasonDeadlineExceeded) || accumulatedEtcdRevisions >= triggerFullsnapshotThreshold {
		var reason string
		if lastJobCompletionReason != nil && *lastJobCompletionReason == batchv1.JobReasonDeadlineExceeded {
			reason = "previous compaction job deadline exceeded"
		} else {
			reason = "delta revisions have crossed the upper threshold"
		}
		logger.Info("Triggering full snapshot",
			"namespace", etcd.Namespace,
			"name", compactionJobName,
			"accumulatedRevisions", accumulatedEtcdRevisions,
			"triggerFullSnapshotThreshold", triggerFullsnapshotThreshold,
			"reason", reason)
		// Trigger full snapshot
		if err = r.FullSnapshotTrigger(ctx, logger, etcd); err != nil {
			recordFullSnapshotsTriggered(druidmetrics.ValueSucceededFalse, etcd.Namespace)
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, fmt.Errorf("error while triggering full snapshot: %v", err)
		}
		recordFullSnapshotsTriggered(druidmetrics.ValueSucceededTrue, etcd.Namespace)
		// Reset the last job completion reason after triggering full snapshot
		lastJobCompletionReason = nil
		return ctrl.Result{Requeue: false}, nil
	}

	// Create compaction job only when number of accumulated revisions over the last full snapshot is more than the configured events threshold.
	if accumulatedEtcdRevisions >= eventsThreshold {
		logger.Info("Creating etcd compaction job", "namespace", etcd.Namespace, "name", compactionJobName)
		job, err = r.createCompactionJob(ctx, logger, etcd)
		if err != nil {
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, fmt.Errorf("error during compaction job creation: %v", err)
		}
		metricJobsCurrent.With(prometheus.Labels{druidmetrics.LabelEtcdNamespace: etcd.Namespace}).Set(1)
	}

	if job.Name != "" {
		logger.Info("Current compaction job status",
			"namespace", job.Namespace, "name", job.Name, "succeeded", job.Status.Succeeded)
	}

	return ctrl.Result{Requeue: false}, nil
}

// triggerFullSnapshot triggers a full snapshot for the given Etcd resource's associated etcd cluster.
func (r *Reconciler) triggerFullSnapshot(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	httpScheme := "http"
	httpTransport := &http.Transport{}

	if tlsConfig := etcd.Spec.Backup.TLS; tlsConfig != nil {
		httpScheme = "https"
		etcdbrCASecret := &v1.Secret{}
		dataKey := ptr.Deref(tlsConfig.TLSCASecretRef.DataKey, "bundle.crt")
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: etcd.Namespace, Name: tlsConfig.TLSCASecretRef.Name}, etcdbrCASecret); err != nil {
			logger.Error(err, "Failed to get etcdbr CA secret", "secretName", tlsConfig.TLSCASecretRef.Name)
			return err
		}
		certData, ok := etcdbrCASecret.Data[dataKey]
		if !ok {
			return fmt.Errorf("CA cert data key %q not found in secret %s/%s", dataKey, etcdbrCASecret.Namespace, etcdbrCASecret.Name)
		}
		caCerts := x509.NewCertPool()
		if !caCerts.AppendCertsFromPEM(certData) {
			return fmt.Errorf("failed to append CA certs from secret %s/%s", etcdbrCASecret.Namespace, etcdbrCASecret.Name)
		}
		httpTransport.TLSClientConfig = &tls.Config{
			RootCAs:    caCerts,
			MinVersion: tls.VersionTLS12,
		}
	}

	httpClient := &http.Client{
		Transport: httpTransport,
	}
	return fullSnapshot(ctx, logger, etcd, httpClient, httpScheme)
}

func (r *Reconciler) createCompactionJob(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (*batchv1.Job, error) {
	activeDeadlineSeconds := r.config.ActiveDeadlineDuration.Seconds()

	_, etcdBackupImage, _, err := utils.GetEtcdImages(etcd, r.imageVector)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch etcd backup image: %v", err)
	}

	var cpuRequests resource.Quantity
	var memoryRequests resource.Quantity
	etcdBackupSpec := etcd.Spec.Backup
	if etcdBackupSpec.SnapshotCompaction != nil && etcdBackupSpec.SnapshotCompaction.Resources != nil {
		cpuRequests = *etcdBackupSpec.SnapshotCompaction.Resources.Requests.Cpu()
		if cpuRequests.IsZero() {
			cpuRequests = defaultCompactionJobCpuRequests
		}
		memoryRequests = *etcdBackupSpec.SnapshotCompaction.Resources.Requests.Memory()
		if memoryRequests.IsZero() {
			memoryRequests = defaultCompactionJobMemoryRequests
		}
	} else {
		cpuRequests = defaultCompactionJobCpuRequests
		memoryRequests = defaultCompactionJobMemoryRequests
	}

	// TerminationGracePeriodSeconds is set to 60 seconds to allow sufficient time for inspecting
	// the pod's status in case of failure. This includes checking statuses such as DisruptionTarget
	// to determine if the pod was subjected to disruptions, or ContainerStatuses to identify if the
	// failure was due to a process issue. The 60-second grace period ensures the pod remains accessible
	// long enough for Druid to fetch this information before it is garbage collected.
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta),
			Namespace: etcd.Namespace,
			Labels:    getLabels(etcd),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         druidv1alpha1.SchemeGroupVersion.String(),
					BlockOwnerDeletion: ptr.To(true),
					Controller:         ptr.To(true),
					Kind:               druidv1alpha1.SchemeGroupVersion.WithKind("Etcd").Kind,
					Name:               etcd.Name,
					UID:                etcd.UID,
				},
			},
		},

		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: ptr.To(int64(activeDeadlineSeconds)),
			Completions:           ptr.To[int32](1),
			BackoffLimit:          ptr.To[int32](0),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: getEtcdCompactionAnnotations(etcd.Spec.Annotations),
					Labels:      getLabels(etcd),
				},
				Spec: v1.PodSpec{
					ActiveDeadlineSeconds:         ptr.To(int64(activeDeadlineSeconds)),
					TerminationGracePeriodSeconds: ptr.To[int64](60),
					ServiceAccountName:            druidv1alpha1.GetServiceAccountName(etcd.ObjectMeta),
					RestartPolicy:                 v1.RestartPolicyNever,
					Containers: []v1.Container{{
						Name:            "compact-backup",
						Image:           etcdBackupImage,
						ImagePullPolicy: v1.PullIfNotPresent,
						Args:            getCompactionJobArgs(etcd, r.config.MetricsScrapeWaitDuration.Duration.String()),
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceCPU:    cpuRequests,
								v1.ResourceMemory: memoryRequests,
							},
						},
						SecurityContext: &v1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
						},
					}},
				},
			},
		},
	}

	if vms, err := getCompactionJobVolumeMounts(etcd); err != nil {
		return nil, fmt.Errorf("error while creating compaction job in %v for %v : %v",
			etcd.Namespace,
			etcd.Name,
			err)
	} else {
		job.Spec.Template.Spec.Containers[0].VolumeMounts = vms
	}

	env, err := utils.GetBackupRestoreContainerEnvVars(etcd.Spec.Backup.Store)
	if err != nil {
		return nil, fmt.Errorf("error while creating compaction job in %v for %v : unable to get backup-restore container environment variables : %v",
			etcd.Namespace,
			etcd.Name,
			err)
	}
	providerEnv, err := druidstore.GetProviderEnvVars(etcd.Spec.Backup.Store)
	if err != nil {
		return nil, fmt.Errorf("error while creating compaction job in %v for %v : unable to get provider-specific environment variables : %v",
			etcd.Namespace,
			etcd.Name,
			err)
	}
	job.Spec.Template.Spec.Containers[0].Env = append(env, providerEnv...)

	if vm, err := getCompactionJobVolumes(ctx, r.Client, r.logger, etcd); err != nil {
		return nil, fmt.Errorf("error creating compaction job in %v for %v : %v",
			etcd.Namespace,
			etcd.Name,
			err)
	} else {
		job.Spec.Template.Spec.Volumes = vm
	}

	logger.Info("Creating job", "namespace", job.Namespace, "name", job.Name)
	err = r.Create(ctx, job)
	if err != nil {
		return nil, err
	}

	// TODO (abdasgupta): Evaluate necessity of claiming object here after creation
	return job, nil
}

func recordSuccessfulJobMetrics(job *batchv1.Job) {
	metricJobsTotal.With(prometheus.Labels{
		druidmetrics.LabelSucceeded:     druidmetrics.ValueSucceededTrue,
		druidmetrics.LabelFailureReason: druidmetrics.ValueFailureReasonNone,
		druidmetrics.LabelEtcdNamespace: job.Namespace,
	}).Inc()
	if job.Status.CompletionTime != nil {
		metricJobDurationSeconds.With(prometheus.Labels{
			druidmetrics.LabelSucceeded:     druidmetrics.ValueSucceededTrue,
			druidmetrics.LabelFailureReason: druidmetrics.ValueFailureReasonNone,
			druidmetrics.LabelEtcdNamespace: job.Namespace,
		}).Observe(job.Status.CompletionTime.Time.Sub(job.Status.StartTime.Time).Seconds())
	}
}

func (r *Reconciler) recordFailedJobMetrics(ctx context.Context, logger logr.Logger, job *batchv1.Job, etcd *druidv1alpha1.Etcd, jobCompletionReason string) error {
	var (
		failureReason   string
		durationSeconds float64
	)

	switch jobCompletionReason {
	case batchv1.JobReasonDeadlineExceeded:
		logger.Info("Job has been completed due to deadline exceeded", "namespace", job.Namespace, "name", job.Name)
		failureReason = druidmetrics.ValueFailureReasonDeadlineExceeded
		durationSeconds = float64(*job.Spec.ActiveDeadlineSeconds)
	case batchv1.JobReasonBackoffLimitExceeded:
		logger.Info("Job has been completed due to backoffLimitExceeded", "namespace", job.Namespace, "name", job.Name)
		podFailureReasonLabelValue, lastTransitionTime, err := getPodFailureValueWithLastTransitionTime(ctx, r, logger, job)
		if err != nil {
			return fmt.Errorf("error while handling job's failure condition type backoffLimitExceeded: %w", err)
		}
		failureReason = podFailureReasonLabelValue
		if !lastTransitionTime.IsZero() {
			durationSeconds = lastTransitionTime.Sub(job.Status.StartTime.Time).Seconds()
		}
	default:
		logger.Info("Job has been completed with unknown reason", "namespace", job.Namespace, "name", job.Name)
		failureReason = druidmetrics.ValueFailureReasonUnknown
		durationSeconds = time.Now().UTC().Sub(job.Status.StartTime.Time).Seconds()
	}

	metricJobsTotal.With(prometheus.Labels{
		druidmetrics.LabelSucceeded:     druidmetrics.ValueSucceededFalse,
		druidmetrics.LabelFailureReason: failureReason,
		druidmetrics.LabelEtcdNamespace: etcd.Namespace,
	}).Inc()

	if durationSeconds > 0 {
		metricJobDurationSeconds.With(prometheus.Labels{
			druidmetrics.LabelSucceeded:     druidmetrics.ValueSucceededFalse,
			druidmetrics.LabelFailureReason: failureReason,
			druidmetrics.LabelEtcdNamespace: etcd.Namespace,
		}).Observe(durationSeconds)
	}

	return nil
}

func getPodFailureValueWithLastTransitionTime(ctx context.Context, r *Reconciler, logger logr.Logger, job *batchv1.Job) (string, time.Time, error) {
	pod, err := getPodForJob(ctx, r.Client, &job.ObjectMeta)
	if err != nil {
		logger.Error(err, "Couldn't fetch pods for job", "namespace", job.Namespace, "name", job.Name)
		return "", time.Time{}, fmt.Errorf("error while fetching pod for job: %w", err)
	}
	if pod == nil {
		logger.Info("Pod not found for job", "namespace", job.Namespace, "name", job.Name)
		return druidmetrics.ValueFailureReasonUnknown, time.Now().UTC(), nil
	}
	podFailureReason, lastTransitionTime := getPodFailureReasonAndLastTransitionTime(pod)
	var podFailureReasonLabelValue string
	switch podFailureReason {
	case podFailureReasonPreemptionByScheduler:
		logger.Info("Pod has been preempted by the scheduler", "namespace", pod.Namespace, "name", pod.Name)
		podFailureReasonLabelValue = druidmetrics.ValueFailureReasonPreempted
	case podFailureReasonDeletionByTaintManager, podFailureReasonEvictionByEvictionAPI, podFailureReasonTerminationByKubelet:
		logger.Info("Pod has been evicted", "namespace", pod.Namespace, "name", pod.Name, "reason", podFailureReason)
		podFailureReasonLabelValue = druidmetrics.ValueFailureReasonEvicted
	case podFailureReasonProcessFailure:
		logger.Info("Pod has failed due to process failure", "namespace", pod.Namespace, "name", pod.Name)
		podFailureReasonLabelValue = druidmetrics.ValueFailureReasonProcessFailure
	default:
		logger.Info("Pod has failed due to unknown reason", "namespace", pod.Namespace, "name", pod.Name)
		podFailureReasonLabelValue = druidmetrics.ValueFailureReasonUnknown
	}
	return podFailureReasonLabelValue, lastTransitionTime, nil
}

// getJobCompletionStateAndReason returns whether the job is successful or not and the reason for the completion.
func getJobCompletionStateAndReason(job *batchv1.Job) (int, string) {
	jobConditions := job.Status.Conditions
	for _, condition := range jobConditions {
		if (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobSuccessCriteriaMet) && condition.Status == v1.ConditionTrue {
			return jobSucceeded, condition.Reason
		}
		if (condition.Type == batchv1.JobFailed || condition.Type == batchv1.JobFailureTarget) && condition.Status == v1.ConditionTrue {
			return jobFailed, condition.Reason
		}
	}
	return jobFailed, "" // the control will never reach here. But since the function signature requires a return value, this is added.
}

// getPodForJob returns the single pod associated with the job.
func getPodForJob(ctx context.Context, cl client.Client, jobMeta *metav1.ObjectMeta) (*v1.Pod, error) {
	labelSelector := client.MatchingLabels{batchv1.JobNameLabel: jobMeta.Name}

	podList := &v1.PodList{}
	if err := cl.List(ctx, podList, &client.ListOptions{
		Namespace: jobMeta.Namespace,
	}, labelSelector); err != nil {
		return nil, fmt.Errorf("error while fetching pods for job: %w", err)
	}

	if len(podList.Items) == 0 {
		return nil, nil
	}

	return &podList.Items[0], nil
}

// getPodFailureReasonAndLastTransitionTime returns the reason for the pod failure.
func getPodFailureReasonAndLastTransitionTime(pod *v1.Pod) (string, time.Time) {
	// Check the pod status DisruptionTarget condition
	podConditions := pod.Status.Conditions
	for _, condition := range podConditions {
		if condition.Type == v1.DisruptionTarget && condition.Status == v1.ConditionTrue {
			return condition.Reason, condition.LastTransitionTime.Time
		}
	}
	// If the DisruptionTarget condition is not present, then check the container status
	if pod.Status.ContainerStatuses != nil {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Terminated != nil {
				return podFailureReasonProcessFailure, containerStatus.State.Terminated.FinishedAt.Time
			}
		}
	}
	return podFailureReasonUnknown, time.Now().UTC()
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	jobLabels := map[string]string{
		druidv1alpha1.LabelAppNameKey:                   druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta),
		druidv1alpha1.LabelComponentKey:                 common.ComponentNameSnapshotCompactionJob,
		"networking.gardener.cloud/to-dns":              "allowed",
		"networking.gardener.cloud/to-private-networks": "allowed",
		"networking.gardener.cloud/to-public-networks":  "allowed",
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), jobLabels)
}

func getCompactionJobVolumeMounts(etcd *druidv1alpha1.Etcd) ([]v1.VolumeMount, error) {
	vms := []v1.VolumeMount{
		{
			Name:      "etcd-workspace-dir",
			MountPath: common.VolumeMountPathEtcdData,
		},
	}

	provider, err := druidstore.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
	if err != nil {
		return vms, fmt.Errorf("storage provider is not recognized while fetching volume mounts")
	}
	switch provider {
	case druidstore.Local:
		vms = append(vms, v1.VolumeMount{
			Name:      "host-storage",
			MountPath: kubernetes.MountPathLocalStore(etcd, &provider),
		})
	case druidstore.GCS:
		vms = append(vms, v1.VolumeMount{
			Name:      common.VolumeNameProviderBackupSecret,
			MountPath: common.VolumeMountPathGCSBackupSecret,
		})
	case druidstore.S3, druidstore.ABS, druidstore.OSS, druidstore.Swift, druidstore.OCS:
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
	provider, err := druidstore.StorageProviderFromInfraProvider(storeValues.Provider)
	if err != nil {
		return vs, fmt.Errorf("could not recognize storage provider while fetching volumes")
	}
	switch provider {
	case "Local":
		hostPath, err := druidstore.GetHostMountPathFromSecretRef(ctx, cl, logger, storeValues, etcd.Namespace)
		if err != nil {
			return vs, fmt.Errorf("could not determine host mount path for local provider")
		}

		hpt := v1.HostPathDirectoryOrCreate
		vs = append(vs, v1.Volume{
			Name: "host-storage",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: hostPath + "/" + ptr.Deref(storeValues.Container, ""),
					Type: &hpt,
				},
			},
		})
	case druidstore.GCS, druidstore.S3, druidstore.OSS, druidstore.ABS, druidstore.Swift, druidstore.OCS:
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
		provider, err := druidstore.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
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

func getEtcdCompactionAnnotations(etcdAnnotations map[string]string) map[string]string {
	etcdCompactionAnnotations := make(map[string]string)

	for key, value := range etcdAnnotations {
		// Do not add annotation: `cluster-autoscaler.kubernetes.io/safe-to-evict: "false"` to compaction job's pod template.
		if key == SafeToEvictKey && value == "false" {
			continue
		}
		etcdCompactionAnnotations[key] = value
	}
	return etcdCompactionAnnotations
}
