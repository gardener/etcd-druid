// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import "sort"

const (
	// LabelSucceeded is a metric label indicating whether associated metric
	// series is for success or failure.
	LabelSucceeded = "succeeded"
	// ValueSucceededTrue is value True for metric label succeeded.
	ValueSucceededTrue = "true"
	// ValueSucceededFalse is value False for metric label succeeded.
	ValueSucceededFalse = "false"

	// LabelFailureReason is a metric label indicating the reason for job failure.
	LabelFailureReason = "failureReason"
	// ValueFailureReasonPreempted is value Preempted for metric label failureReason.
	ValueFailureReasonPreempted = "preempted"
	// ValueFailureReasonEvicted is value Evicted for metric label failureReason.
	ValueFailureReasonEvicted = "evicted"
	// ValueFailureReasonDeadlineExceeded is value DeadlineExceeded for metric label failureReason.
	ValueFailureReasonDeadlineExceeded = "deadlineExceeded"
	// ValueFailureReasonProcessFailure is value ProcessFailure for metric label failureReason.
	ValueFailureReasonProcessFailure = "processFailure"
	// ValueFailureReasonUnknown is value Unknown for metric label failureReason.
	ValueFailureReasonUnknown = "unknown"
	// ValueFailureReasonNone is value None for metric label failureReason.
	ValueFailureReasonNone = "none"

	// LabelEtcdNamespace is the label for prometheus metrics to indicate etcd namespace
	LabelEtcdNamespace = "etcd_namespace"
)

var (
	// DruidLabels are the labels for prometheus metrics
	DruidLabels = map[string][]string{
		LabelSucceeded: {
			ValueSucceededFalse,
			ValueSucceededTrue,
		},
		LabelFailureReason: {
			ValueFailureReasonPreempted,
			ValueFailureReasonEvicted,
			ValueFailureReasonDeadlineExceeded,
			ValueFailureReasonProcessFailure,
			ValueFailureReasonUnknown,
		},
	}
)

// GenerateLabelCombinations generates combinations of label values for metrics
func GenerateLabelCombinations(labelValues map[string][]string) []map[string]string {
	labels := make([]string, len(labelValues))
	valuesList := make([][]string, len(labelValues))
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
	}
	combinations := getCombinations(valuesList)

	output := make([]map[string]string, len(combinations))
	for i, combination := range combinations {
		labelVals := make(map[string]string, len(labels))
		for j := range labels {
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
	for i := range a {
		for j := range b {
			arr := make([]string, len(a[i])+len(b[j]))
			ctr := 0
			for ii := range a[i] {
				arr[ctr] = a[i][ii]
				ctr++
			}
			for jj := range b[j] {
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
	for i := range output {
		elem := make([]string, 1)
		elem[0] = s[i]
		output[i] = elem
	}
	return output
}
