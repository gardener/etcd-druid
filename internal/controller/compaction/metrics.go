// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"fmt"
	"time"

	druidmetrics "github.com/gardener/etcd-druid/internal/metrics"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	namespaceEtcdDruid  = "etcddruid"
	subsystemCompaction = "compaction"
)

var (
	baseDuration = 60 * time.Second
	// metricJobsTotal is the metric used to count the total number of compaction jobs initiated by compaction controller.
	metricJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "jobs_total",
			Help:      "Total number of compaction jobs initiated by compaction controller.",
		},
		[]string{druidmetrics.LabelSucceeded, druidmetrics.LabelFailureReason, druidmetrics.LabelEtcdNamespace},
	)

	// metricFullSnapshotsTriggered is the metric used to count the total number of full snapshots triggered by compaction controller.
	metricFullSnapshotsTriggered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "full_snapshot_triggered_total",
			Help:      "Total number of full snapshot triggered by compaction controller.",
		},
		[]string{druidmetrics.LabelSucceeded, druidmetrics.LabelEtcdNamespace},
	)

	// metricJobsCurrent is the metric used to expose currently running compaction job. This metric is important to get a sense of total number of compaction job running in a seed cluster.
	metricJobsCurrent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "jobs_current",
			Help:      "Number of currently running comapction job.",
		},
		[]string{druidmetrics.LabelEtcdNamespace},
	)

	// metricJobDurationSeconds is the metric used to expose the time taken to finish a compaction job.
	metricJobDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "job_duration_seconds",
			Help:      "Total time taken in seconds to finish a running compaction job.",
			Buckets: []float64{baseDuration.Seconds(),
				5 * baseDuration.Seconds(),
				10 * baseDuration.Seconds(),
				15 * baseDuration.Seconds(),
				20 * baseDuration.Seconds(),
				30 * baseDuration.Seconds(),
			},
		},
		[]string{druidmetrics.LabelSucceeded, druidmetrics.LabelFailureReason, druidmetrics.LabelEtcdNamespace},
	)

	// metricNumDeltaEvents is the metric used to expose the total number of etcd events to be compacted by a compaction job.
	metricNumDeltaEvents = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "num_delta_events",
			Help:      "Total number of etcd events to be compacted by a compaction job.",
		},
		[]string{druidmetrics.LabelEtcdNamespace},
	)
)

func init() {
	// metricJobsTotalValues
	metricJobsTotalValues := map[string][]string{
		druidmetrics.LabelSucceeded:     druidmetrics.DruidLabels[druidmetrics.LabelSucceeded],
		druidmetrics.LabelFailureReason: druidmetrics.DruidLabels[druidmetrics.LabelFailureReason],
		druidmetrics.LabelEtcdNamespace: druidmetrics.DruidLabels[druidmetrics.LabelEtcdNamespace],
	}
	metricJobsTotalCombinations := druidmetrics.GenerateLabelCombinations(metricJobsTotalValues)
	for _, combination := range metricJobsTotalCombinations {
		metricJobsTotal.With(prometheus.Labels(combination))
	}

	// metricJobDurationSecondsValues
	metricJobDurationSecondsValues := map[string][]string{
		druidmetrics.LabelSucceeded:     druidmetrics.DruidLabels[druidmetrics.LabelSucceeded],
		druidmetrics.LabelFailureReason: druidmetrics.DruidLabels[druidmetrics.LabelFailureReason],
		druidmetrics.LabelEtcdNamespace: druidmetrics.DruidLabels[druidmetrics.LabelEtcdNamespace],
	}
	compactionJobDurationSecondsCombinations := druidmetrics.GenerateLabelCombinations(metricJobDurationSecondsValues)
	for _, combination := range compactionJobDurationSecondsCombinations {
		metricJobDurationSeconds.With(prometheus.Labels(combination))
	}

	// metricFullSnapshotsTriggeredValues
	metricFullSnapshotsTriggeredValues := map[string][]string{
		druidmetrics.LabelSucceeded:     druidmetrics.DruidLabels[druidmetrics.LabelSucceeded],
		druidmetrics.LabelEtcdNamespace: druidmetrics.DruidLabels[druidmetrics.LabelEtcdNamespace],
	}
	metricFullSnapshotsTriggeredCombinations := druidmetrics.GenerateLabelCombinations(metricFullSnapshotsTriggeredValues)
	for _, combination := range metricFullSnapshotsTriggeredCombinations {
		metricFullSnapshotsTriggered.With(prometheus.Labels(combination))
	}

	// Metrics have to be registered to be exposed:
	metrics.Registry.MustRegister(metricJobsTotal)
	metrics.Registry.MustRegister(metricJobDurationSeconds)
	metrics.Registry.MustRegister(metricJobsCurrent)
	metrics.Registry.MustRegister(metricNumDeltaEvents)
	metrics.Registry.MustRegister(metricFullSnapshotsTriggered)
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

func recordFailureJobMetrics(failureReason string, durationSeconds float64, job *batchv1.Job) {
	metricJobsTotal.With(prometheus.Labels{
		druidmetrics.LabelSucceeded:     druidmetrics.ValueSucceededFalse,
		druidmetrics.LabelFailureReason: failureReason,
		druidmetrics.LabelEtcdNamespace: job.Namespace,
	}).Inc()

	if durationSeconds > 0 {
		metricJobDurationSeconds.With(prometheus.Labels{
			druidmetrics.LabelSucceeded:     druidmetrics.ValueSucceededFalse,
			druidmetrics.LabelFailureReason: failureReason,
			druidmetrics.LabelEtcdNamespace: job.Namespace,
		}).Observe(durationSeconds)
	}
}

func (r *Reconciler) fetchFailedJobMetrics(ctx context.Context, logger logr.Logger, job *batchv1.Job, jobCompletionReason string) (string, float64, error) {
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
			return "", 0, fmt.Errorf("error while handling job's failure condition type backoffLimitExceeded: %w", err)
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
	return failureReason, durationSeconds, nil
}

// recordFullSnapshotsTriggered increments the full snapshot triggered metric with the given labels.
func recordFullSnapshotsTriggered(succeeded, namespace string) {
	metricFullSnapshotsTriggered.With(prometheus.Labels{
		druidmetrics.LabelSucceeded:     succeeded,
		druidmetrics.LabelEtcdNamespace: namespace,
	}).Inc()
}
