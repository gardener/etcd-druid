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
	druidmetrics "github.com/gardener/etcd-druid/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	namespaceEtcdDruid  = "etcddruid"
	subsystemCompaction = "compaction"
)

var (
	// metricJobsTotal is the metric used to count the total number of compaction jobs initiated by compaction controller.
	metricJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "jobs_total",
			Help:      "Total number of compaction jobs initiated by compaction controller.",
		},
		[]string{druidmetrics.LabelSucceeded},
	)

	// metricJobsCurrent is the metric used to expose currently running compaction job. This metric is important to get a sense of total number of compaction job running in a seed cluster.
	metricJobsCurrent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "jobs_current",
			Help:      "Number of currently running comapction job.",
		},
		[]string{druidmetrics.EtcdNamespace},
	)

	// metricJobDurationSeconds is the metric used to expose the time taken to finish a compaction job.
	metricJobDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "job_duration_seconds",
			Help:      "Total time taken in seconds to finish a running compaction job.",
		},
		[]string{druidmetrics.LabelSucceeded},
	)

	// metricNumDeltaEvents is the metric used to expose the total number of etcd events to be compacted by a compaction job.
	metricNumDeltaEvents = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "num_delta_events",
			Help:      "Total number of etcd events to be compacted by a compaction job.",
		},
		[]string{},
	)
)

func init() {
	// metricJobsTotalValues
	metricJobsTotalValues := map[string][]string{
		druidmetrics.LabelSucceeded: druidmetrics.DruidLabels[druidmetrics.LabelSucceeded],
	}
	metricJobsTotalCombinations := druidmetrics.GenerateLabelCombinations(metricJobsTotalValues)
	for _, combination := range metricJobsTotalCombinations {
		metricJobsTotal.With(prometheus.Labels(combination))
	}

	// metricJobDurationSecondsValues
	metricJobDurationSecondsValues := map[string][]string{
		druidmetrics.LabelSucceeded: druidmetrics.DruidLabels[druidmetrics.LabelSucceeded],
	}
	compactionJobDurationSecondsCombinations := druidmetrics.GenerateLabelCombinations(metricJobDurationSecondsValues)
	for _, combination := range compactionJobDurationSecondsCombinations {
		metricJobDurationSeconds.With(prometheus.Labels(combination))
	}

	// Metrics have to be registered to be exposed:
	metrics.Registry.MustRegister(metricJobsTotal)
	metrics.Registry.MustRegister(metricJobDurationSeconds)

	metrics.Registry.MustRegister(metricJobsCurrent)

	metrics.Registry.MustRegister(metricNumDeltaEvents)
}
