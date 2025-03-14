// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations of etcd.spec.etcd fields.

package etcd

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"
)

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
			validateEtcdCreation(g, etcd, test.expectErr)
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
			validateEtcdCreation(g, etcd, test.expectErr)
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
			validateEtcdCreation(g, etcd, test.expectErr)
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
			validateEtcdCreation(g, etcd, test.expectErr)

		})
	}
}
