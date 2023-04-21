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
	// CompactionJobCounterTotal is the metric used to count the total number of compaction jobs initiated by compaction controller.
	CompactionJobCounterTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "jobs_total",
			Help:      "Total number of compaction jobs initiated by compaction controller.",
		},
		[]string{druidmetrics.LabelSucceeded},
	)

	// CompactionJobDurationSeconds is the metric used to expose the time taken to finish running a compaction job.
	CompactionJobDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "job_duration_seconds",
			Help:      "Total time taken in seconds to finish running a compaction job.",
		},
		[]string{druidmetrics.LabelSucceeded},
	)

	// NumDeltaEvents is the metric used to expose the total number of etcd events to be compacted by a compaction job.
	NumDeltaEvents = prometheus.NewGaugeVec(
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
	// compactionJobCountTotal
	compactionJobCountTotalValues := map[string][]string{
		druidmetrics.LabelSucceeded: druidmetrics.DruidLabels[druidmetrics.LabelSucceeded],
	}
	compactionJobCounterCombinations := druidmetrics.GenerateLabelCombinations(compactionJobCountTotalValues)
	for _, combination := range compactionJobCounterCombinations {
		CompactionJobCounterTotal.With(prometheus.Labels(combination))
	}

	// compactionJobDurationSeconds
	compactionJobDurationSecondsValues := map[string][]string{
		druidmetrics.LabelSucceeded: druidmetrics.DruidLabels[druidmetrics.LabelSucceeded],
	}
	compactionJobDurationSecondsCombinations := druidmetrics.GenerateLabelCombinations(compactionJobDurationSecondsValues)
	for _, combination := range compactionJobDurationSecondsCombinations {
		CompactionJobDurationSeconds.With(prometheus.Labels(combination))
	}

	// Metrics have to be registered to be exposed:
	metrics.Registry.MustRegister(CompactionJobCounterTotal)
	metrics.Registry.MustRegister(CompactionJobDurationSeconds)

	metrics.Registry.MustRegister(NumDeltaEvents)
}
