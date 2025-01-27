// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
)

func TestContainsAllDesiredLabels(t *testing.T) {
	testCases := []struct {
		name     string
		actual   map[string]string
		desired  map[string]string
		expected bool
	}{
		{
			name: "actual map is missing a key",
			actual: map[string]string{
				"key1": "value1",
			},
			desired: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: false,
		},
		{
			name: "actual map has a key with a different value",
			actual: map[string]string{
				"key1": "value1",
				"key2": "value3",
			},
			desired: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: false,
		},
		{
			name: "actual map has all desired labels",
			actual: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			desired: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: true,
		},
		{
			name: "actual map has all desired labels and extra labels",
			actual: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
			desired: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: true,
		},
		{
			name:   "actual map is nil",
			actual: nil,
			desired: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: false,
		},
		{
			name: "desired map is nil",
			actual: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			desired:  nil,
			expected: true,
		},
		{
			name:     "both maps are nil",
			actual:   nil,
			desired:  nil,
			expected: true,
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g.Expect(ContainsAllDesiredLabels(tc.actual, tc.desired)).To(Equal(tc.expected))
		})
	}
}

func TestContainsLabel(t *testing.T) {
	testCases := []struct {
		name     string
		labels   map[string]string
		key      string
		value    string
		expected bool
	}{
		{
			name: "labels map is missing the key",
			labels: map[string]string{
				"key1": "value1",
			},
			key:      "key2",
			value:    "value2",
			expected: false,
		},
		{
			name: "labels map has the key with a different value",
			labels: map[string]string{
				"key1": "value1",
				"key2": "value3",
			},
			key:      "key2",
			value:    "value2",
			expected: false,
		},
		{
			name: "labels map has the key with the desired value",
			labels: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			key:      "key2",
			value:    "value2",
			expected: true,
		},
		{
			name:     "labels map is nil",
			labels:   nil,
			key:      "key1",
			value:    "value1",
			expected: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g.Expect(ContainsLabel(tc.labels, tc.key, tc.value)).To(Equal(tc.expected))
		})
	}
}

func TestDoesLabelSelectorMatchLabels(t *testing.T) {
	testCases := []struct {
		name           string
		labelSelector  *metav1.LabelSelector
		resourceLabels map[string]string
		expectedResult bool
		expectedError  bool
	}{
		{
			name: "label selector matches resource labels",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			resourceLabels: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "label selector does not match resource labels",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			resourceLabels: map[string]string{
				"key1": "value1",
				"key2": "value3",
			},
			expectedResult: false,
			expectedError:  false,
		},
		{
			name: "label selector matches resource labels, where there are more resource labels than label selector MatchLabels",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			resourceLabels: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name:          "label selector is nil",
			labelSelector: nil,
			resourceLabels: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "resource labels are nil",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			resourceLabels: nil,
			expectedResult: false,
			expectedError:  false,
		},
		{
			name:          "label selector is empty",
			labelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{}},
			resourceLabels: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name: "resource labels are nil",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			resourceLabels: map[string]string{},
			expectedResult: false,
			expectedError:  false,
		},
		{
			name:           "both label selector and resource labels are nil",
			labelSelector:  nil,
			resourceLabels: nil,
			expectedResult: true,
			expectedError:  false,
		},
		{
			name:           "both label selector and resource labels are empty",
			labelSelector:  &metav1.LabelSelector{MatchLabels: map[string]string{}},
			resourceLabels: map[string]string{},
			expectedResult: true,
			expectedError:  false,
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := DoesLabelSelectorMatchLabels(tc.labelSelector, tc.resourceLabels)
			if tc.expectedError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(result).To(Equal(tc.expectedResult))
		})
	}
}
