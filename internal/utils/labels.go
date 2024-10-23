// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ContainsAllDesiredLabels checks if the actual map contains all the desired labels.
func ContainsAllDesiredLabels(actual, desired map[string]string) bool {
	for key, desiredValue := range desired {
		actualValue, ok := actual[key]
		if !ok || actualValue != desiredValue {
			return false
		}
	}
	return true
}

// ContainsLabel checks if the actual map contains the specified key-value pair.
func ContainsLabel(actual map[string]string, key, value string) bool {
	actualValue, ok := actual[key]
	return ok && actualValue == value
}

// DoesLabelSelectorMatchLabels checks if the given label selector matches the given labels.
func DoesLabelSelectorMatchLabels(labelSelector *metav1.LabelSelector, resourceLabels map[string]string) (bool, error) {
	if labelSelector == nil {
		return true, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return false, err
	}
	return selector.Matches(labels.Set(resourceLabels)), nil
}
