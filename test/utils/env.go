// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"os"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/store"
	"k8s.io/utils/ptr"
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

// GetEmulatorEndpoint returns the endpoint override for emulators based on environment variables.
// Returns nil if no emulator endpoint is configured for the given provider.
func GetEmulatorEndpoint(provider druidv1alpha1.StorageProvider) *string {
	switch provider {
	case store.S3, "aws":
		if host := GetEnvOrDefault("LOCALSTACK_HOST", ""); host != "" {
			return ptr.To("http://" + host)
		}
	case store.ABS, "azure":
		if host := GetEnvOrDefault("AZURITE_DOMAIN", ""); host != "" {
			return ptr.To("http://" + host + "/devstoreaccount1/")
		}
	case store.GCS, "gcp":
		if host := GetEnvOrDefault("FAKEGCS_HOST", ""); host != "" {
			return ptr.To("http://" + host + "/storage/v1/b")
		}
	}
	return nil
}
