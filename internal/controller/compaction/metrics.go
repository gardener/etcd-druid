// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"time"

	druidmetrics "github.com/gardener/etcd-druid/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
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
