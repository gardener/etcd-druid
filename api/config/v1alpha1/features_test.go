// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	. "github.com/onsi/gomega"
	"testing"
)

func TestDefaultFeatureGate(t *testing.T) {
	tests := []struct {
		name                    string
		enabledFeatures         map[string]bool
		expectedEnabledFeatures map[string]bool
		expectedError           bool
	}{
		{
			name: "Map contains known features and does not violate the maturity specification",
			enabledFeatures: map[string]bool{
				UseEtcdWrapper: true,
			},
			expectedEnabledFeatures: map[string]bool{
				UseEtcdWrapper: true,
			},
		},
		{
			name: "Map contains unknown features",
			enabledFeatures: map[string]bool{
				"UnknownFeature": true,
			},
			expectedError: true,
		},
		{
			name: "Map contains features that violate the maturity specification",
			enabledFeatures: map[string]bool{
				UseEtcdWrapper: false,
			},
			expectedError: true,
		},
	}

	g := NewWithT(t)

	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := DefaultFeatureGates.SetEnabledFeaturesFromMap(test.enabledFeatures)
			if test.expectedError {
				g.Expect(err).ToNot(BeNil())
			} else {
				g.Expect(err).To(BeNil())
				for feature, expected := range test.expectedEnabledFeatures {
					g.Expect(DefaultFeatureGates.IsEnabled(feature)).To(Equal(expected))
				}
			}
		})
	}
}

func TestSetEnabledFeaturesFromMap(t *testing.T) {
	fg := newFeatureGate()
	fg.knownFeatures = map[string]maturityLevelSpec{
		"feature1": maturityLevelSpecAlpha,
		"feature2": maturityLevelSpecBeta,
		"feature3": maturityLevelSpecGA,
	}
	tests := []struct {
		name            string
		enabledFeatures map[string]bool
		expectedError   bool
	}{
		{
			name: "Map contains valid features that do not violate the maturity specification",
			enabledFeatures: map[string]bool{
				"feature1": true,
				"feature2": false,
				"feature3": true,
			},
			expectedError: false,
		},
		{
			name: "Map contains unknown features",
			enabledFeatures: map[string]bool{
				"feature1": true,
				"feature3": false,
			},
			expectedError: true,
		},
		{
			name: "Map contains features that violate the maturity specification",
			enabledFeatures: map[string]bool{
				"feature1": false,
				"feature2": true,
				"feature3": false,
			},
			expectedError: true,
		},
		{
			name:            "Map is empty",
			enabledFeatures: map[string]bool{},
			expectedError:   false,
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := fg.SetEnabledFeaturesFromMap(test.enabledFeatures)
			if test.expectedError {
				g.Expect(err).ToNot(BeNil())
			} else {
				g.Expect(err).To(BeNil())
			}
		})
	}
}

func TestIsEnabled(t *testing.T) {
	fg := newFeatureGate()
	fg.knownFeatures = map[string]maturityLevelSpec{
		"feature1": maturityLevelSpecAlpha,
		"feature2": maturityLevelSpecBeta,
		"feature3": maturityLevelSpecGA,
	}
	fg.enabledFeatures = map[string]bool{
		"feature1": false,
		"feature2": true,
		"feature3": true,
	}
	tests := []struct {
		name     string
		feature  string
		expected bool
	}{
		{
			name:     "Feature is known and is enabled",
			feature:  "feature2",
			expected: true,
		},
		{
			name:     "Feature is known and is disabled",
			feature:  "feature1",
			expected: false,
		},
		{
			name:     "Feature is unknown",
			feature:  "unknownFeature",
			expected: false,
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			g.Expect(fg.IsEnabled(test.feature)).To(Equal(test.expected))
		})
	}
}
