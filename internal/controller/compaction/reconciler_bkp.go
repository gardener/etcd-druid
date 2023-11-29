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

//
//import (
//	"context"
//	"time"
//
//	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
//	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
//	druidmetrics "github.com/gardener/etcd-druid/pkg/metrics"
//	"github.com/gardener/gardener/pkg/utils/imagevector"
//	"github.com/go-logr/logr"
//	"github.com/prometheus/client_golang/prometheus"
//	batchv1 "k8s.io/api/batch/v1"
//	coordinationv1 "k8s.io/api/coordination/v1"
//	"k8s.io/apimachinery/pkg/api/errors"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	ctrl "sigs.k8s.io/controller-runtime"
//	"sigs.k8s.io/controller-runtime/pkg/client"
//	"sigs.k8s.io/controller-runtime/pkg/log"
//	"sigs.k8s.io/controller-runtime/pkg/manager"
//)
//
//const (
//	// defaultETCDQuota is the default etcd quota.
//	defaultETCDQuota = 8 * 1024 * 1024 * 1024 // 8Gi
//)
//
//// Reconciler reconciles compaction jobs for Etcd resources.
//type Reconciler struct {
//	client      client.Client
//	config      *Config
//	imageVector imagevector.ImageVector
//	logger      logr.Logger
//}
//
//// NewReconciler creates a new reconciler for Compaction
//func NewReconciler(mgr manager.Manager, config *Config) (*Reconciler, error) {
//	imageVector, err := ctrlutils.CreateImageVector()
//	if err != nil {
//		return nil, err
//	}
//	return NewReconcilerWithImageVector(mgr, config, imageVector), nil
//}
//
//// NewReconcilerWithImageVector creates a new reconciler for Compaction with an ImageVector.
//func NewReconcilerWithImageVector(mgr manager.Manager, config *Config, imageVector imagevector.ImageVector) *Reconciler {
//	return &Reconciler{
//		client:      mgr.GetClient(),
//		config:      config,
//		imageVector: imageVector,
//		logger:      log.Log.WithName(controllerName),
//	}
//}
//
//// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch
//// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
//// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;create;list;watch;update;patch;delete
//// +kubebuilder:rbac:groups="",resources=pods,verbs=list;watch;delete;get
//
//// Reconcile reconciles the compaction job.
//func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//	etcd := &druidv1alpha1.Etcd{}
//	if result := ctrlutils.GetLatestEtcd(ctx, r.client, req.NamespacedName, etcd); ctrlutils.ShortCircuitReconcileFlow(result) {
//		return result.ReconcileResult()
//	}
//	rLog := r.logger.WithValues("etcd", etcd.GetNamespaceName())
//	jobKey := getJobKey(etcd)
//	if etcd.IsMarkedForDeletion() || !etcd.IsBackupEnabled() {
//		return r.triggerJobDeletion(ctx, rLog, jobKey)
//	}
//
//	if result := r.reconcileExistingJob(ctx, rLog, jobKey); ctrlutils.ShortCircuitReconcileFlow(result) {
//		return result.ReconcileResult()
//	}
//
//	return r.reconcileJob(ctx, rLog, etcd)
//	/*
//		Get latest etcd
//		if it is not found then do not requeue
//		if there is an error then requeue with error
//
//		if it's marked for deletion {
//			cleanup any Jobs still out there
//		}
//		Get Existing Job
//			if Job not found then continue
//			If error in getting Job requeue with error
//		If Job exists and is in deletion, requeue
//		If Job is active return with no-requeue
//		If Job succeeded record metrics and delete job and continue reconcile
//		If Job failed record metrics, delete job and continue reconcile
//		Get delta and full leases
//			If error requeue with error
//		compute difference and if difference > eventsThreshold create Job and record metrics
//	*/
//
//}
//
//func getJobKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
//	return client.ObjectKey{
//		Namespace: etcd.Namespace,
//		Name:      etcd.GetCompactionJobName(),
//	}
//}
//
//func (r *Reconciler) reconcileExistingJob(ctx context.Context, logger logr.Logger, jobKey client.ObjectKey) ctrlutils.ReconcileStepResult {
//	rjLog := logger.WithValues("operation", "reconcile-compaction-job", "job", jobKey)
//	job := &batchv1.Job{}
//	if err := r.client.Get(ctx, jobKey, job); !errors.IsNotFound(err) {
//		return ctrlutils.ReconcileWithError(err)
//	}
//
//	type existingJobHandler func(ctx context.Context, logger logr.Logger, job *batchv1.Job) ctrlutils.ReconcileStepResult
//	handlers := []existingJobHandler{
//		r.handleJobDeletionInProgress,
//		r.handleActiveJob,
//		r.handleSuccessfulJobCompletion,
//		r.handleFailedJobCompletion,
//	}
//	for _, handler := range handlers {
//		if result := handler(ctx, rjLog, job); ctrlutils.ShortCircuitReconcileFlow(result) {
//			return result
//		}
//	}
//	return ctrlutils.ContinueReconcile()
//}
//
//func (r *Reconciler) getMatchingJobs(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) {
//	// TODO: @seshachalam-yv need to fix
//	// r.client.List(ctx)
//}
//
//func (r *Reconciler) reconcileJob(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
//	jobObjectKey := client.ObjectKey{Namespace: etcd.Namespace, Name: etcd.GetCompactionJobName()}
//	// Get any existing job that might exist at this time
//	job := &batchv1.Job{}
//	if err := r.client.Get(ctx, jobObjectKey, job); !errors.IsNotFound(err) {
//		return ctrlutils.ReconcileWithError(err).ReconcileResult()
//	}
//
//	// TODO: @seshachalam-yv need to fix
//	// If there is an existing Job then check the Job status and take appropriate action.
//	// if !utils.IsEmptyString(job.Name) {
//	// 	if result := r.handleExistingJob(ctx, rjLog, job); ctrlutils.ShortCircuitReconcileFlow(result) {
//	// 		return result.ReconcileResult()
//	// 	}
//	// }
//
//	// latestDeltaRevision, err := getLatestDeltaRevision(ctx, etcd.GetDeltaSnapshotLeaseName())
//	return ctrlutils.ReconcileWithError(nil).ReconcileResult()
//}
//
//func (r *Reconciler) canScheduleJob(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) {
//	r.getLatestFullSnapshotRevision(ctx, logger, etcd)
//}
//
//func (r *Reconciler) getLatestFullSnapshotRevision(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) (int64, error) {
//	fullLease := &coordinationv1.Lease{}
//	if err := r.client.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: etcd.GetFullSnapshotLeaseName()}, fullLease); err != nil {
//		logger.Error(err, "could not fetch full snapshot lease", "lease-name", etcd.GetFullSnapshotLeaseName())
//		return -1, err
//	}
//	return parseRevision(fullLease)
//}
//
//func parseRevision(lease *coordinationv1.Lease) (int64, error) {
//	// TODO: @seshachalam-yv need to fix
//	// if lease.Spec.HolderIdentity == nil {
//
//	// }
//
//	return int64(0), nil
//}
//
//func (r *Reconciler) handleExistingJob(ctx context.Context, logger logr.Logger, job *batchv1.Job) ctrlutils.ReconcileStepResult {
//	type existingJobHandler func(ctx context.Context, logger logr.Logger, job *batchv1.Job) ctrlutils.ReconcileStepResult
//	handlers := []existingJobHandler{
//		r.handleJobDeletionInProgress,
//		r.handleActiveJob,
//		r.handleSuccessfulJobCompletion,
//		r.handleFailedJobCompletion,
//	}
//	for _, handler := range handlers {
//		if result := handler(ctx, logger, job); ctrlutils.ShortCircuitReconcileFlow(result) {
//			return result
//		}
//	}
//	return ctrlutils.ContinueReconcile()
//}
//
//func (r *Reconciler) handleJobDeletionInProgress(ctx context.Context, logger logr.Logger, job *batchv1.Job) ctrlutils.ReconcileStepResult {
//	if !job.DeletionTimestamp.IsZero() {
//		logger.Info("Deletion has been triggered for the job. A new job can only be created once the previously scheduled job has been deleted")
//		return ctrlutils.ReconcileAfter(10*time.Second, "deletion in progress, requeuing job")
//	}
//	return ctrlutils.ContinueReconcile()
//}
//
//func (r *Reconciler) handleActiveJob(_ context.Context, _ logr.Logger, job *batchv1.Job) ctrlutils.ReconcileStepResult {
//	metricJobsCurrent.With(prometheus.Labels{druidmetrics.EtcdNamespace: job.Namespace}).Set(1)
//	return ctrlutils.DoNotRequeue()
//}
//
//func (r *Reconciler) handleFailedJobCompletion(ctx context.Context, logger logr.Logger, job *batchv1.Job) ctrlutils.ReconcileStepResult {
//	if job.Status.Failed > 0 {
//		metricJobsCurrent.With(prometheus.Labels{druidmetrics.EtcdNamespace: job.Namespace}).Set(0)
//		if job.Status.StartTime != nil {
//			metricJobDurationSeconds.With(prometheus.Labels{druidmetrics.LabelSucceeded: druidmetrics.ValueSucceededFalse, druidmetrics.EtcdNamespace: job.Namespace}).Observe(time.Since(job.Status.StartTime.Time).Seconds())
//		}
//	}
//	return ctrlutils.ContinueReconcile()
//}
//
//func (r *Reconciler) handleSuccessfulJobCompletion(ctx context.Context, logger logr.Logger, job *batchv1.Job) ctrlutils.ReconcileStepResult {
//	if job.Status.Succeeded > 0 {
//		metricJobsCurrent.With(prometheus.Labels{druidmetrics.EtcdNamespace: job.Namespace}).Set(0)
//		if job.Status.CompletionTime != nil {
//			metricJobDurationSeconds.With(prometheus.Labels{druidmetrics.LabelSucceeded: druidmetrics.ValueSucceededTrue, druidmetrics.EtcdNamespace: job.Namespace}).Observe(job.Status.CompletionTime.Time.Sub(job.Status.StartTime.Time).Seconds())
//		}
//		if result := r.deleteJob(ctx, logger, job); ctrlutils.ShortCircuitReconcileFlow(result) {
//			return result
//		}
//		metricJobsTotal.With(prometheus.Labels{druidmetrics.LabelSucceeded: druidmetrics.ValueSucceededTrue, druidmetrics.EtcdNamespace: job.Namespace}).Inc()
//	}
//	return ctrlutils.ContinueReconcile()
//}
//
//func (r *Reconciler) triggerJobDeletion(ctx context.Context, logger logr.Logger, jobObjectKey client.ObjectKey) (ctrl.Result, error) {
//	dLog := logger.WithValues("operation", "delete-compaction-job", "job", jobObjectKey)
//	job := &batchv1.Job{}
//	if err := r.client.Get(ctx, jobObjectKey, job); err != nil {
//		if errors.IsNotFound(err) {
//			dLog.Info("No compaction job exists, nothing to clean up")
//			return ctrlutils.DoNotRequeue().ReconcileResult()
//		}
//		return ctrlutils.ReconcileWithError(err).ReconcileResult()
//	}
//
//	if job.DeletionTimestamp != nil {
//		dLog.Info("Deletion has already been triggered for the compaction job. Skipping further action.")
//		return ctrlutils.DoNotRequeue().ReconcileResult()
//	}
//
//	dLog.Info("Triggering delete of compaction job")
//	if result := r.deleteJob(ctx, dLog, job); ctrlutils.ShortCircuitReconcileFlow(result) {
//		return result.ReconcileResult()
//	}
//	return ctrlutils.DoNotRequeue().ReconcileResult()
//}
//
//func (r *Reconciler) deleteJob(ctx context.Context, logger logr.Logger, job *batchv1.Job) ctrlutils.ReconcileStepResult {
//	if err := client.IgnoreNotFound(r.client.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground))); err != nil {
//		logger.Error(err, "error when deleting compaction job")
//		return ctrlutils.ReconcileWithError(err)
//	}
//	return ctrlutils.ContinueReconcile()
//}
//
//func (r *Reconciler) getLatestJob(ctx context.Context, objectKey client.ObjectKey) (*batchv1.Job, error) {
//	job := &batchv1.Job{}
//	if err := r.client.Get(ctx, objectKey, job); err != nil {
//		if errors.IsNotFound(err) {
//			return nil, nil
//		}
//		return nil, err
//	}
//	return job, nil
//}
