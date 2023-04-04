// Copyright 2019 The etcd Authors
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

package metrics

import (
	"sort"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// LabelSucceeded is a metric label indicating whether associated metric
	// series is for success or failure.
	LabelSucceeded = "succeeded"
	// ValueSucceededTrue is value True for metric label succeeded.
	ValueSucceededTrue = "true"
	// ValueSucceededFalse is value False for metric label failed.
	ValueSucceededFalse = "false"
	// CompactionJobSubmitted is value for metric of total number of compaction jobs initiated by compaction controller.
	CompactionJobSubmitted = "compaction_jobs"
	// ValueRestoreSingleNode is value for metric of single node restoration.
	ValueRestoreSingleNode = "single_node"
	// LabelError is a metric error to indicate error occured.
	LabelError = "error"

	namespaceEtcdDruid  = "etcd_druid"
	subsystemCompaction = "compaction"
)

var (
	// DruidLabels are the labels for prometheus metrics
	DruidLabels = map[string][]string{
		LabelSucceeded: {
			ValueSucceededFalse,
			ValueSucceededTrue,
		},
	}

	// CompactionJobCounterTotal is metric to count the total number of compaction jobs initiated by compaction controller.
	CompactionJobCounterTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "compaction_job_total",
			Help:      "Total number of compaction jobs initiated.",
		},
		[]string{LabelSucceeded},
	)

	// CompactionJobDurationSeconds is metric to expose duration to complete last compaction job.
	CompactionJobDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "compaction_job_duration_seconds",
			Help:      "Total time taken in seconds to complete last compaction job.",
		},
		[]string{LabelSucceeded},
	)

	// TotalNumberOfEvents is metric to expose the total number of events compacted by last compaction job.
	TotalNumberOfEvents = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespaceEtcdDruid,
			Subsystem: subsystemCompaction,
			Name:      "total_number_of_events",
			Help:      "Total number of events compacted by last compaction job.",
		},
		[]string{},
	)
)

// GenerateLabelCombinations generates combinations of label values for metrics
func GenerateLabelCombinations(labelValues map[string][]string) []map[string]string {
	labels := make([]string, len(labelValues))
	valuesList := make([][]string, len(labelValues))
	valueCounts := make([]int, len(labelValues))
	i := 0
	for label := range labelValues {
		labels[i] = label
		i++
	}
	sort.Strings(labels)
	for i, label := range labels {
		values := make([]string, len(labelValues[label]))
		copy(values, labelValues[label])
		valuesList[i] = values
		valueCounts[i] = len(values)
	}
	combinations := getCombinations(valuesList)

	output := make([]map[string]string, len(combinations))
	for i, combination := range combinations {
		labelVals := make(map[string]string, len(labels))
		for j := 0; j < len(labels); j++ {
			labelVals[labels[j]] = combination[j]
		}
		output[i] = labelVals
	}
	return output
}

// getCombinations returns combinations of slice of string slices
func getCombinations(valuesList [][]string) [][]string {
	if len(valuesList) == 0 {
		return [][]string{}
	} else if len(valuesList) == 1 {
		return wrapInSlice(valuesList[0])
	}

	return cartesianProduct(wrapInSlice(valuesList[0]), getCombinations(valuesList[1:]))
}

// cartesianProduct combines two slices of slice of strings while also
// combining the sub-slices of strings into a single string
// Ex:
// a => [[p,q],[r,s]]
// b => [[1,2],[3,4]]
// Output => [[p,q,1,2],[p,q,3,4],[r,s,1,2],[r,s,3,4]]
func cartesianProduct(a [][]string, b [][]string) [][]string {
	output := make([][]string, len(a)*len(b))
	for i := 0; i < len(a); i++ {
		for j := 0; j < len(b); j++ {
			arr := make([]string, len(a[i])+len(b[j]))
			ctr := 0
			for ii := 0; ii < len(a[i]); ii++ {
				arr[ctr] = a[i][ii]
				ctr++
			}
			for jj := 0; jj < len(b[j]); jj++ {
				arr[ctr] = b[j][jj]
				ctr++
			}
			output[(i*len(b))+j] = arr
		}
	}
	return output
}

// wrapInSlice is a helper function to wrap a slice of strings within
// a slice of slices of strings
// Ex: [p,q,r] -> [[p],[q],[r]]
func wrapInSlice(s []string) [][]string {
	output := make([][]string, len(s))
	for i := 0; i < len(output); i++ {
		elem := make([]string, 1)
		elem[0] = s[i]
		output[i] = elem
	}
	return output
}
