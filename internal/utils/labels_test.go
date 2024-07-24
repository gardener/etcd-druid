// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"testing"

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
