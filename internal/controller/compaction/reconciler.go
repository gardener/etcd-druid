// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"errors"
	"fmt"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
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

	// pollInterval defines the interval between polling attempts for status updates
	pollInterval = 1 * time.Second
	// pollTimeout defines the maximum time to wait for status updates
	pollTimeout = 30 * time.Second
)

const (
	jobSucceeded int = iota
	jobFailed
)

var (
	// defaultCompactionJobCPURequests defines the default cpu requests for the compaction job
	defaultCompactionJobCPURequests = resource.MustParse("600m")
	// defaultCompactionJobMemoryRequests defines the default memory requests for the compaction job
	defaultCompactionJobMemoryRequests = resource.MustParse("3Gi")
)

// Reconciler reconciles compaction jobs for Etcd resources.
type Reconciler struct {
	client.Client
	config           druidconfigv1alpha1.CompactionControllerConfiguration
	imageVector      imagevector.ImageVector
	logger           logr.Logger
	EtcdbrHTTPClient httpClientInterface
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
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	if !etcd.DeletionTimestamp.IsZero() || !etcd.IsBackupStoreEnabled() {
		// Delete compaction job if exists
		return r.delete(ctx, r.logger, etcd)
	}

	logger := r.logger.WithValues("etcdNamespace", etcd.Namespace, "etcdName", etcd.Name)

	return r.doReconcile(ctx, logger, etcd)
}

func (r *Reconciler) doReconcile(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	// Fetch the compaction job for the given Etcd resource.
	compactionJobName := druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta)
	job, err := r.fetchCompactionJob(ctx, compactionJobName, etcd.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error while fetching compaction job: %w", err)
	}

	if isJobPresent(job) {
		// If the job is marked for deletion, we will not proceed with compaction process until the job is deleted.
		if !job.DeletionTimestamp.IsZero() {
			logger.Info("Job is already in deletion. A new job will be created only if the previous one has been deleted.", "jobName", job.Name)
			return ctrl.Result{
				RequeueAfter: 10 * time.Second,
			}, nil
		}

		// Check if there is one active job or not
		if job.Status.Active > 0 {
			logger.Info("Compaction job is currently running", "jobName", job.Name)
			metricJobsCurrent.With(prometheus.Labels{druidmetrics.LabelEtcdNamespace: etcd.Namespace}).Set(1)
			// Don't need to requeue if the job is currently running
			return ctrl.Result{}, nil
		}

		// Set the current job count to 0 as the job is not active
		logger.Info("Compaction job is completed", "jobName", job.Name)
		metricJobsCurrent.With(prometheus.Labels{druidmetrics.LabelEtcdNamespace: etcd.Namespace}).Set(0)

		// Update the metrics and status for the completed job
		logger.Info("Updating metrics and status for the completed job", "jobName", job.Name)
		if err := r.updateMetricsAndStatusForCompletedJob(ctx, logger, job, etcd); err != nil {
			logger.Error(err, "Error while updating metrics and/or status for completed job", "jobName", job.Name)
			return ctrl.Result{}, fmt.Errorf("error while updating metrics and status for completed job: %w", err)
		}

		// Delete the completed compaction job
		logger.Info("Deleting the completed compaction job", "jobName", compactionJobName)
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			logger.Error(err, "Couldn't delete the completed compaction job", "jobName", compactionJobName)
			return ctrl.Result{}, fmt.Errorf("error while deleting the completed compaction job: %w", err)
		}

		// Wait for the deletion marker to be set before proceeding
		logger.Info("Waiting for job deletion marker to be set", "jobName", compactionJobName)
		if err := r.waitForJobDeletionMarker(ctx, logger, compactionJobName, etcd.Namespace); err != nil {
			logger.Error(err, "Error waiting for job deletion marker")
			return ctrl.Result{}, fmt.Errorf("error waiting for job deletion marker: %w", err)
		}

		// Requeue to ensure that the compaction job is deleted and next reconciliation can proceed with no active job.
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger.Info("No compaction job is currently running")

	diff, err := r.getDeltaRevisionsSinceFullSnapshot(ctx, logger, etcd)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error while getting difference between delta and full snapshot revisions: %w", err)
	}

	logger.Info("Delta revisions since last full snapshot", "diff", diff)
	metricNumDeltaEvents.With(prometheus.Labels{druidmetrics.LabelEtcdNamespace: etcd.Namespace}).Set(float64(diff))
	return r.triggerFullSnapshotOrCreateCompactionJob(ctx, logger, etcd, diff)
}

// fetchCompactionJob fetches the compaction job for the given Etcd resource.
func (r *Reconciler) fetchCompactionJob(ctx context.Context, compactionJobName, compactionJobNamespace string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: compactionJobName, Namespace: compactionJobNamespace}, job); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("error while fetching compaction job with the name %v, in the namespace %v: %w", compactionJobName, compactionJobNamespace, err)
		}
		// Job not found, return nil
		return nil, nil
	}
	return job, nil
}

func isJobPresent(job *batchv1.Job) bool {
	return job != nil && job.Name != ""
}

// waitForJobDeletionMarker waits for the job to have a deletion timestamp set or to be completely deleted.
// This ensures that the next reconciliation will not see a stale job without deletion marker due to informer cache staleness.
func (r *Reconciler) waitForJobDeletionMarker(ctx context.Context, logger logr.Logger, jobName, jobNamespace string) error {
	logger.Info("Waiting for job deletion marker to be set", "jobName", jobName)

	err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true,
		func(ctx context.Context) (bool, error) {
			job, err := r.fetchCompactionJob(ctx, jobName, jobNamespace)
			if err != nil {
				logger.Error(err, "Error fetching job while waiting for deletion marker", "jobName", jobName)
				return false, err // This will cause retry
			}

			if job == nil {
				logger.Info("Job completely deleted", "jobName", jobName)
				return true, nil // Job is gone, success
			}

			if !job.DeletionTimestamp.IsZero() {
				logger.Info("Job deletion marker is set", "jobName", jobName)
				return true, nil // Deletion marker found, success
			}

			// Job still exists without deletion marker, continue polling
			logger.V(1).Info("Job still exists without deletion marker, retrying...", "jobName", jobName)
			return false, nil
		})

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.Info("Timeout waiting for job deletion marker, proceeding anyway", "jobName", jobName, "timeout", pollTimeout)
			return nil // Don't fail the reconciliation on timeout
		}
		return fmt.Errorf("error waiting for job deletion marker: %w", err)
	}

	return nil
}

// waitForEtcdStatusConditionUpdate waits for the etcd status condition to reflect the expected state.
// This ensures that the next reconciliation will not see a stale etcd status due to informer cache staleness.
func (r *Reconciler) waitForEtcdStatusConditionUpdate(ctx context.Context, logger logr.Logger, etcdNamespace, etcdName string, expectedCondition druidv1alpha1.Condition) error {
	logger.Info("Waiting for etcd status condition to be updated",
		"expectedType", expectedCondition.Type, "expectedStatus", expectedCondition.Status, "expectedReason", expectedCondition.Reason)

	err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true,
		func(ctx context.Context) (bool, error) {
			latestEtcd := &druidv1alpha1.Etcd{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: etcdNamespace, Name: etcdName}, latestEtcd); err != nil {
				logger.Error(err, "Error fetching etcd while waiting for status condition update")
				return false, err // This will cause retry
			}

			// Find the condition of the expected type and compare Status, and Reason
			for _, condition := range latestEtcd.Status.Conditions {
				if condition.Type == expectedCondition.Type {
					if condition.Status == expectedCondition.Status && condition.Reason == expectedCondition.Reason {
						logger.Info("Etcd status condition updated successfully", "conditionType", condition.Type, "status", condition.Status, "reason", condition.Reason)
						return true, nil // Expected condition found
					}
					// Condition exists but doesn't match expected values
					logger.V(1).Info("Etcd status condition found but doesn't match expected values, retrying...", "conditionType", condition.Type,
						"currentStatus", condition.Status, "expectedStatus", expectedCondition.Status,
						"currentReason", condition.Reason, "expectedReason", expectedCondition.Reason)
					return false, nil
				}
			}

			// Condition of expected type not found
			logger.V(1).Info("Etcd status condition not found, retrying...", "expectedType", expectedCondition.Type)
			return false, nil
		})

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.Info("Timeout waiting for etcd status condition update, proceeding anyway", "expectedType", expectedCondition.Type, "timeout", pollTimeout)
			return nil // Don't fail the reconciliation on timeout
		}
		return fmt.Errorf("error waiting for etcd status condition update: %w", err)
	}

	return nil
}

func (r *Reconciler) delete(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	job := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta), Namespace: etcd.Namespace}, job); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("error while fetching compaction job: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if job.DeletionTimestamp == nil {
		logger.Info("Deleting job", "jobName", job.Name)
		if err := client.IgnoreNotFound(r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground))); err != nil {
			return ctrl.Result{}, fmt.Errorf("error while deleting compaction job: %w", err)
		}
	}

	logger.Info("No compaction job is running")
	return ctrl.Result{}, nil
}

// getDeltaRevisionsSinceFullSnapshot retrieves the difference between the full and delta snapshot revisions for the given Etcd resource.
func (r *Reconciler) getDeltaRevisionsSinceFullSnapshot(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (int64, error) {
	// Get full and delta snapshot lease to check the HolderIdentity value to take decision on compaction job
	fullLease := &coordinationv1.Lease{}
	fullSnapshotLeaseName := druidv1alpha1.GetFullSnapshotLeaseName(etcd.ObjectMeta)
	if err := r.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: fullSnapshotLeaseName}, fullLease); err != nil {
		logger.Error(err, "Couldn't fetch full snap lease", "leaseName", fullSnapshotLeaseName)
		return 0, err
	}

	deltaLease := &coordinationv1.Lease{}
	deltaSnapshotLeaseName := druidv1alpha1.GetDeltaSnapshotLeaseName(etcd.ObjectMeta)
	if err := r.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: deltaSnapshotLeaseName}, deltaLease); err != nil {
		logger.Error(err, "Couldn't fetch delta snap lease", "leaseName", deltaSnapshotLeaseName)
		return 0, err
	}

	// Revisions have not been set yet by etcd-back-restore container.
	// Skip further processing as we cannot calculate a revision delta.
	if fullLease.Spec.HolderIdentity == nil || deltaLease.Spec.HolderIdentity == nil {
		return 0, fmt.Errorf("holder identity is not set for full or delta snapshot lease, cannot calculate revision delta")
	}

	full, err := strconv.ParseInt(*fullLease.Spec.HolderIdentity, 10, 64)
	if err != nil {
		logger.Error(err, "Can't convert holder identity of full snap lease to integer",
			"leaseName", fullLease.Name, "holderIdentity", fullLease.Spec.HolderIdentity)
		return 0, err
	}

	delta, err := strconv.ParseInt(*deltaLease.Spec.HolderIdentity, 10, 64)
	if err != nil {
		logger.Error(err, "Can't convert holder identity of delta snap lease to integer",
			"leaseName", deltaLease.Name, "holderIdentity", deltaLease.Spec.HolderIdentity)
		return 0, err
	}
	return delta - full, nil
}

// triggerFullSnapshotOrCreateCompactionJob triggers a full snapshot or creates a compaction job based on the accumulated revisions and the last compaction job status.
func (r *Reconciler) triggerFullSnapshotOrCreateCompactionJob(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, accumulatedEtcdRevisions int64) (ctrl.Result, error) {
	eventsThreshold := r.config.EventsThreshold
	triggerFullSnapshotThreshold := r.config.TriggerFullSnapshotThreshold
	if compactionSpec := etcd.Spec.Backup.SnapshotCompaction; compactionSpec != nil {
		if compactionSpec.EventsThreshold != nil {
			eventsThreshold = *compactionSpec.EventsThreshold
		}
		if compactionSpec.TriggerFullSnapshotThreshold != nil {
			triggerFullSnapshotThreshold = *compactionSpec.TriggerFullSnapshotThreshold
		}
	}

	logger.Info("Compaction thresholds", "eventsThreshold", eventsThreshold, "triggerFullSnapshotThreshold", triggerFullSnapshotThreshold)

	// Trigger full snapshot if the delta revisions over the last full snapshot are more than the configured upper threshold
	// or if the last job completion reason is DeadlineExceeded or last full snapshot failed.
	// This is to ensure that we avoid spinning up compaction jobs even when we know that the probability of it getting succeeded is very low due to the large number of revisions.
	// This avoids unnecessary resource consumption and delays in the compaction process
	if isLastCompactionConditionDeadlineExceededOrFullSnapshotFailure(etcd) || accumulatedEtcdRevisions >= triggerFullSnapshotThreshold {
		return r.triggerFullSnapshotAndUpdateStatus(ctx, logger, etcd, accumulatedEtcdRevisions, triggerFullSnapshotThreshold)
	}
	return r.checkAndTriggerCompactionJob(ctx, logger, etcd, accumulatedEtcdRevisions, eventsThreshold)
}

// triggerFullSnapshotAndUpdateStatus triggers a full snapshot and updates the etcd status condition LastSnapshotCompactionSucceeded.
func (r *Reconciler) triggerFullSnapshotAndUpdateStatus(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, accumulatedEtcdRevisions, triggerFullSnapshotThreshold int64) (ctrl.Result, error) {
	latestCondition := druidv1alpha1.Condition{
		Type: druidv1alpha1.ConditionTypeLastSnapshotCompactionSucceeded,
	}
	fullSnapErr := r.triggerFullSnapshot(ctx, logger, etcd, accumulatedEtcdRevisions, triggerFullSnapshotThreshold)
	if fullSnapErr != nil {
		latestCondition.Status = druidv1alpha1.ConditionFalse
		latestCondition.Reason = druidv1alpha1.FullSnapshotFailureReason
		latestCondition.Message = fmt.Sprintf("Error while triggering full snapshot for etcd %s/%s: %v ,Compaction will be retried", etcd.Namespace, etcd.Name, fullSnapErr)
	} else {
		latestCondition.Status = druidv1alpha1.ConditionTrue
		latestCondition.Reason = druidv1alpha1.FullSnapshotSuccessReason
		latestCondition.Message = fmt.Sprintf("Full snapshot taken successfully for etcd %s/%s", etcd.Namespace, etcd.Name)
	}
	etcdStatusUpdateErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		logger.Info("Updating etcd status condition for full snapshot compaction",
			"conditionType", latestCondition.Type, "status", latestCondition.Status, "reason", latestCondition.Reason, "message", latestCondition.Message)
		// Fetch the latest etcd resource to avoid conflict errors
		latestEtcd := &druidv1alpha1.Etcd{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: etcd.Namespace, Name: etcd.Name}, latestEtcd); err != nil {
			return fmt.Errorf("error while fetching etcd %s/%s: %w", etcd.Namespace, etcd.Name, err)
		}
		return r.updateCompactionJobEtcdStatusCondition(ctx, latestEtcd, latestCondition)
	})
	// If the etcd status update was successful, we will wait for the condition to be reflected in the cache.
	if etcdStatusUpdateErr == nil {
		if err := r.waitForEtcdStatusConditionUpdate(ctx, logger, etcd.Namespace, etcd.Name, latestCondition); err != nil {
			// Don't fail the reconciliation - this is just a cache consistency check
			logger.Error(err, "Error waiting for etcd status condition update after full snapshot")
		}
	}

	var requeueErrReason error
	if fullSnapErr != nil {
		requeueErrReason = fmt.Errorf("error while triggering compaction-fullSnapshot: %w", fullSnapErr)
	}
	if etcdStatusUpdateErr != nil {
		requeueErrReason = errors.Join(requeueErrReason, fmt.Errorf("error while updating compaction-fullSnapshot etcd status condition: %w", etcdStatusUpdateErr))
	}
	if requeueErrReason != nil {
		logger.Error(requeueErrReason, "Error in triggering compaction-fullSnapshot and/or updating etcd status condition")
		return ctrl.Result{}, requeueErrReason
	}
	logger.Info("Compaction-FullSnapshot triggered and etcd status condition updated successfully")
	return ctrl.Result{}, nil
}

// checkAndTriggerCompactionJob creates compaction job only when number of accumulated revisions over the last full snapshot is more than the configured events threshold.
func (r *Reconciler) checkAndTriggerCompactionJob(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, accumulatedEtcdRevisions, eventsThreshold int64) (ctrl.Result, error) {
	var err error
	job := &batchv1.Job{}
	compactionJobName := druidv1alpha1.GetCompactionJobName(etcd.ObjectMeta)
	if accumulatedEtcdRevisions >= eventsThreshold {
		logger.Info("Creating etcd compaction job", "jobName", compactionJobName)
		job, err = r.createCompactionJob(ctx, logger, etcd)
		if err != nil {
			logger.Error(err, "Error while creating compaction job", "jobName", compactionJobName)
			return ctrl.Result{}, fmt.Errorf("error during compaction job creation: %w", err)
		}
		metricJobsCurrent.With(prometheus.Labels{druidmetrics.LabelEtcdNamespace: etcd.Namespace}).Set(1)
	}

	if isJobPresent(job) {
		logger.Info("Current compaction job status", "jobName", job.Name, "succeeded", job.Status.Succeeded)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) createCompactionJob(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (*batchv1.Job, error) {
	activeDeadlineSeconds := r.config.ActiveDeadlineDuration.Seconds()

	_, etcdBackupImage, _, err := utils.GetEtcdImages(etcd, r.imageVector)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch etcd backup image: %w", err)
	}

	var cpuRequests resource.Quantity
	var memoryRequests resource.Quantity
	etcdBackupSpec := etcd.Spec.Backup
	if etcdBackupSpec.SnapshotCompaction != nil && etcdBackupSpec.SnapshotCompaction.Resources != nil {
		cpuRequests = *etcdBackupSpec.SnapshotCompaction.Resources.Requests.Cpu()
		if cpuRequests.IsZero() {
			cpuRequests = defaultCompactionJobCPURequests
		}
		memoryRequests = *etcdBackupSpec.SnapshotCompaction.Resources.Requests.Memory()
		if memoryRequests.IsZero() {
			memoryRequests = defaultCompactionJobMemoryRequests
		}
	} else {
		cpuRequests = defaultCompactionJobCPURequests
		memoryRequests = defaultCompactionJobMemoryRequests
	}

	// TerminationGracePeriodSeconds is set to 60 seconds to allow sufficient time for inspecting
	// the pod's status in case of disruptions. This includes checking statuses such as DisruptionTarget
	// to determine if the pod was subjected to disruptions, such as preemptions & evictions.
	// The 60-second grace period ensures the pod remains accessible long enough for Druid to fetch this information before the resource is deleted.
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
		return nil, fmt.Errorf("error while creating compaction job in %v for %v : %w",
			etcd.Namespace,
			etcd.Name,
			err)
	} else {
		job.Spec.Template.Spec.Containers[0].VolumeMounts = vms
	}

	env, err := utils.GetBackupRestoreContainerEnvVars(etcd, etcd.Spec.Backup.Store)
	if err != nil {
		return nil, fmt.Errorf("error while creating compaction job in %v for %v : unable to get backup-restore container environment variables : %w",
			etcd.Namespace,
			etcd.Name,
			err)
	}
	providerEnv, err := druidstore.GetProviderEnvVars(etcd.Spec.Backup.Store)
	if err != nil {
		return nil, fmt.Errorf("error while creating compaction job in %v for %v : unable to get provider-specific environment variables : %w",
			etcd.Namespace,
			etcd.Name,
			err)
	}
	job.Spec.Template.Spec.Containers[0].Env = append(env, providerEnv...)

	if vm, err := getCompactionJobVolumes(ctx, r.Client, r.logger, etcd); err != nil {
		return nil, fmt.Errorf("error creating compaction job in %v for %v : %w",
			etcd.Namespace,
			etcd.Name,
			err)
	} else {
		job.Spec.Template.Spec.Volumes = vm
	}

	logger.Info("Creating job", "jobName", job.Name)
	err = r.Create(ctx, job)
	if err != nil {
		return nil, err
	}

	// TODO (abdasgupta): Evaluate necessity of claiming object here after creation
	return job, nil
}

// updateMetricsAndStatusForCompletedJob checks the job completion state and reason to capture the metrics and update the etcd status condition
func (r *Reconciler) updateMetricsAndStatusForCompletedJob(ctx context.Context, logger logr.Logger, job *batchv1.Job, etcd *druidv1alpha1.Etcd) error {
	var (
		// jobFailureReason indicates the specific reason for the job failure.
		jobFailureReason string
		// jobFailureReasonMetricLabelValue is the value for the metric label indicating the reason for job failure.
		jobFailureReasonMetricLabelValue string
		jobDurationSeconds               float64
	)

	jobCompletionState, jobCompletionReason := getJobCompletionStateAndReason(job)
	logger.Info("Job has been completed with reason: " + jobCompletionReason)
	if jobCompletionState == jobFailed {
		var err error
		jobFailureReason, jobFailureReasonMetricLabelValue, jobDurationSeconds, err = r.fetchFailedJobMetrics(ctx, logger, job, jobCompletionReason)
		if err != nil {
			logger.Error(err, "Error while fetching failed job metrics", "jobName", job.Name)
			return fmt.Errorf("error while fetching failed job metrics: %w", err)
		}
		logger.Info("Job has failed with reason: " + jobFailureReason)
	}

	// Construct & Update the etcd status condition for the compaction job
	latestCondition := druidv1alpha1.Condition{
		Type:    druidv1alpha1.ConditionTypeLastSnapshotCompactionSucceeded,
		Status:  computeSnapshotCompactionJobStatus(jobCompletionState),
		Reason:  computeSnapshotCompactionJobReason(jobCompletionState, jobFailureReason),
		Message: fmt.Sprintf("Compaction job %s/%s completed with state %s", job.Namespace, job.Name, jobCompletionReason),
	}
	if latestCondition.Status == druidv1alpha1.ConditionFalse {
		latestCondition.Message += ", Compaction will be retried"
	}

	logger.Info("Updating etcd status condition for compaction job",
		"conditionType", latestCondition.Type, "status", latestCondition.Status,
		"reason", latestCondition.Reason, "message", latestCondition.Message)
	// Fetch the latest etcd resource before updating the status condition to avoid conflict errors
	latestEtcd := &druidv1alpha1.Etcd{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: etcd.Namespace, Name: etcd.Name}, latestEtcd); err != nil {
		return fmt.Errorf("error while fetching etcd %s/%s: %w", etcd.Namespace, etcd.Name, err)
	}
	if err := r.updateCompactionJobEtcdStatusCondition(ctx, latestEtcd, latestCondition); err != nil {
		logger.Error(err, "Error while updating etcd status condition for compaction job", "jobName", job.Name)
		return fmt.Errorf("error while updating etcd status condition for compaction job: %w", err)
	}

	// Wait for the etcd status condition to be reflected in the cache
	if err := r.waitForEtcdStatusConditionUpdate(ctx, logger, etcd.Namespace, etcd.Name, latestCondition); err != nil {
		// Don't fail the reconciliation - this is just a cache consistency check
		logger.Error(err, "Error waiting for etcd status condition update after job completion")
	}

	// Metrics are to be updated after updating the Etcd status condition, as otherwise, any failures in updating the etcd status conditions results
	// in a reconciliation loop, which leads to the metrics being updated multiple times for the same job.
	if jobCompletionState == jobSucceeded {
		recordSuccessfulJobMetrics(job)
	} else {
		recordFailureJobMetrics(jobFailureReasonMetricLabelValue, jobDurationSeconds, job)
	}
	return nil
}

// getPodFailureValueWithLastTransitionTime returns the specific reason for the pod failure, corresponding metric label value, and the last transition time of the pod.
func (r *Reconciler) getPodFailureValueWithLastTransitionTime(ctx context.Context, logger logr.Logger, job *batchv1.Job) (string, string, time.Time, error) {
	pod, err := getPodForJob(ctx, r.Client, &job.ObjectMeta)
	if err != nil {
		logger.Error(err, "Couldn't fetch pods for job", "name", job.Name)
		return "", "", time.Time{}, fmt.Errorf("error while fetching pod for job: %w", err)
	}
	if pod == nil {
		logger.Info("Pod not found for job", "name", job.Name)
		return druidv1alpha1.PodFailureReasonUnknown, druidmetrics.ValueFailureReasonUnknown, time.Now().UTC(), nil
	}
	podFailureReason, lastTransitionTime := getPodFailureReasonAndLastTransitionTime(pod)
	var podFailureReasonMetricLabelValue string
	switch podFailureReason {
	case druidv1alpha1.PodFailureReasonPreemptionByScheduler:
		logger.Info("Pod has been preempted by the scheduler", "name", pod.Name)
		podFailureReasonMetricLabelValue = druidmetrics.ValueFailureReasonPreempted
	case druidv1alpha1.PodFailureReasonDeletionByTaintManager, druidv1alpha1.PodFailureReasonEvictionByEvictionAPI, druidv1alpha1.PodFailureReasonTerminationByKubelet:
		logger.Info("Pod has been evicted", "name", pod.Name, "reason", podFailureReason)
		podFailureReasonMetricLabelValue = druidmetrics.ValueFailureReasonEvicted
	case druidv1alpha1.PodFailureReasonProcessFailure:
		logger.Info("Pod has failed due to process failure", "name", pod.Name)
		podFailureReasonMetricLabelValue = druidmetrics.ValueFailureReasonProcessFailure
	default:
		logger.Info("Pod has failed due to unknown reason", "name", pod.Name)
		podFailureReasonMetricLabelValue = druidmetrics.ValueFailureReasonUnknown
	}
	return podFailureReason, podFailureReasonMetricLabelValue, lastTransitionTime, nil
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
				return druidv1alpha1.PodFailureReasonProcessFailure, containerStatus.State.Terminated.FinishedAt.Time
			}
		}
	}
	return druidv1alpha1.PodFailureReasonUnknown, time.Now().UTC()
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
