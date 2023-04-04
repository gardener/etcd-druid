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

func init() {
	// CompactionJobCountTotal
	CompactionJobCountTotalValues := map[string][]string{
		druidmetrics.LabelSucceeded: druidmetrics.DruidLabels[druidmetrics.LabelSucceeded],
	}
	CompactionJobCounterCombinations := druidmetrics.GenerateLabelCombinations(CompactionJobCountTotalValues)
	for _, combination := range CompactionJobCounterCombinations {
		druidmetrics.CompactionJobCounterTotal.With(prometheus.Labels(combination))
	}

	// CompactionJobDurationSeconds
	CompactionJobDurationSecondsValues := map[string][]string{
		druidmetrics.LabelSucceeded: druidmetrics.DruidLabels[druidmetrics.LabelSucceeded],
	}
	CompactionJobDurationSecondsCombinations := druidmetrics.GenerateLabelCombinations(CompactionJobDurationSecondsValues)
	for _, combination := range CompactionJobDurationSecondsCombinations {
		druidmetrics.CompactionJobDurationSeconds.With(prometheus.Labels(combination))
	}

	// Metrics have to be registered to be exposed:
	metrics.Registry.MustRegister(druidmetrics.CompactionJobCounterTotal)
	metrics.Registry.MustRegister(druidmetrics.CompactionJobDurationSeconds)

	metrics.Registry.MustRegister(druidmetrics.TotalNumberOfEvents)
}
