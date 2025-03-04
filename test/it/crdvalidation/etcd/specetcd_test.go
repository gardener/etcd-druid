// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// tests for etcd.spec.etcd fields

// runs the validation on the etcd.spec.etcd.etcdDefragTimeout field.
func TestValidateSpecEtcdEtcdDefragTimeout(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
				return
			}

			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Etcd.EtcdDefragTimeout = duration
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}

}

// runs the validation on the etcd.spec.etcd.heartbeatDuration field.
func TestValidateSpecEtcdHeartbeatDuration(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
				return
			}

			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Etcd.HeartbeatDuration = duration
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}

}

// runs validation on the etcd.spec.etcd.metrics field where if the field exists, the value should be either "basic" or "extensive"
func TestValidateSpecEtcdMetrics(t *testing.T) {
	tests := []struct {
		name         string
		etcdName     string
		metricsValue string
		expectErr    bool
	}{
		{
			name:         "Valid metrics #1: basic",
			etcdName:     "etcd-valid-1",
			metricsValue: "basic",
			expectErr:    false,
		},
		{
			name:         "Valid metrics #2: extensive",
			etcdName:     "etcd-valid-2",
			metricsValue: "extensive",
			expectErr:    false,
		},
		{
			name:         "Invalid metrics #1: invalid value",
			etcdName:     "etcd-invalid-1",
			metricsValue: "random",
			expectErr:    true,
		},
		{
			name:         "Invalid metrics #2: Empty string",
			etcdName:     "etcd-invalid-2",
			metricsValue: "",
			expectErr:    true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Etcd.Metrics = (*druidv1alpha1.MetricsLevel)(&test.metricsValue)
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}

}

// runs validation on the cron expression passed to the field etcd.spec.etcd.defragmentationSchedule
func TestValidateSpecEtcdDefragmentationSchedule(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range cronFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Etcd.DefragmentationSchedule = &test.value
			validateEtcdCreation(t, g, etcd, test.expectErr)

		})
	}
}

// checks that if the values for etcd.spec.storageCapacity and etcd.spec.etcd.quota are valid, then if backups are enabled, the value of storageCapacity must be > 3x value of quota. If backups are not enabled, value of storageCapacity must be > quota
func TestValidateSpecStorageCapacitySpecEtcdQuotaRelation(t *testing.T) {
	testNs, g := setupTestEnvironment(t)
	tests := []struct {
		name            string
		etcdName        string
		storageCapacity resource.Quantity
		quota           resource.Quantity
		backup          bool
		expectErr       bool
	}{
		{
			name:            "Valid #1: backups enabled",
			etcdName:        "etcd-valid-1",
			storageCapacity: resource.MustParse("27Gi"),
			quota:           resource.MustParse("8Gi"),
			backup:          true,
			expectErr:       false,
		},
		{
			name:            "Valid #2: backups disabled",
			etcdName:        "etcd-valid-2",
			storageCapacity: resource.MustParse("12Gi"),
			quota:           resource.MustParse("8Gi"),
			backup:          false,
			expectErr:       false,
		},
		{
			name:            "Invalid #1: backups enabled",
			etcdName:        "etcd-invalid-1",
			storageCapacity: resource.MustParse("15Gi"),
			quota:           resource.MustParse("8Gi"),
			backup:          true,
			expectErr:       true,
		},
		{
			name:            "Invalid #2: backups disabled",
			etcdName:        "etcd-invalid-2",
			storageCapacity: resource.MustParse("9Gi"),
			quota:           resource.MustParse("10Gi"),
			backup:          false,
			expectErr:       true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.StorageCapacity = &test.storageCapacity
			etcd.Spec.Etcd.Quota = &test.quota

			if test.backup {
				container := "etcd-bucket"
				provider := "Provider"
				etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
					Container: &container,
					Provider:  (*druidv1alpha1.StorageProvider)(&provider),
					Prefix:    "etcd-test",
					SecretRef: &corev1.SecretReference{
						Name: "test-secret",
					},
				}
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})

	}
}
