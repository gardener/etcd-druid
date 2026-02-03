// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"os"
)

// GetEnvOrError gets the environment variable by key and returns an error if it is not set.
func GetEnvOrError(key string) (string, error) {
	if value, ok := os.LookupEnv(key); ok {
		return value, nil
	}

	return "", fmt.Errorf("environment variable not found: %s", key)
}

// GetEnvOrDefault gets the environment variable by key or returns the default value if it is not set.
func GetEnvOrDefault(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}
