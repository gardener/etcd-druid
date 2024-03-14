// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	"golang.org/x/exp/constraints"
)

// ShouldBeOneOfAllowedValues checks if value is amongst the allowedValues. If it is not then an error is returned else nil is returned.
// Type is constrained by comparable forcing the consumers to only use concrete types that can be compared using the == or != operators.
func ShouldBeOneOfAllowedValues[E comparable](key string, allowedValues []E, value E) error {
	for _, av := range allowedValues {
		if av == value {
			return nil
		}
	}
	return fmt.Errorf("unsupported value %v provided for %s. allowed values are: %v", value, key, allowedValues)
}

// MustBeGreaterThan checks if the value is greater than the lowerBound. If it is not then an error is returned else nil is returned.
// Type is constrained by constraints.Ordered which enforces the consumers to use concrete types that can be compared using >, >=, <, <= operators.
func MustBeGreaterThan[E constraints.Ordered](key string, lowerBound, value E) error {
	if value <= lowerBound {
		return fmt.Errorf("%s should have a value greater than %v. value provided is %v", key, lowerBound, value)
	}
	return nil
}

// MustBeGreaterThanOrEqualTo checks if the value is greater than or equal to the lowerBound. If it is not then an error is returned else nil is returned.
// Type is constrained by constraints.Ordered which enforces the consumers to use concrete types that can be compared using >, >=, <, <= operators.
func MustBeGreaterThanOrEqualTo[E constraints.Ordered](key string, lowerBound, value E) error {
	if value < lowerBound {
		return fmt.Errorf("%s should have a value greater or equal to %v. value provided is %v", key, lowerBound, value)
	}
	return nil
}
