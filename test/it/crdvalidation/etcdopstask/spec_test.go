// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// TestValidateEtcdOpsTaskSpecConfig tests the validation of the config field
func TestValidateEtcdOpsTaskSpecConfig(t *testing.T) {
	tests := []struct {
		name      string
		taskName  string
		config    *druidv1alpha1.EtcdOpsTaskConfig
		expectErr bool
	}{
		{
			name:     "Valid config with OnDemandSnapshot",
			taskName: "task-valid-config",
			config: &druidv1alpha1.EtcdOpsTaskConfig{
				OnDemandSnapshot: &druidv1alpha1.OnDemandSnapshotConfig{
					Type: druidv1alpha1.OnDemandSnapshotTypeFull,
				},
			},
			expectErr: false,
		},
		{
			name:      "Invalid config - empty config",
			taskName:  "task-invalid-empty",
			config:    &druidv1alpha1.EtcdOpsTaskConfig{},
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			task := testutils.EtcdOpsTaskBuilderWithDefaults(test.taskName, testNs).WithEtcdName("test-etcd").Build()
			task.Spec.Config = *test.config
			validateEtcdOpsTaskCreation(g, task, test.expectErr)
		})
	}
}

// TestValidateEtcdOpsTaskSpecTTLSecondsAfterFinished tests TTL field validation
func TestValidateEtcdOpsTaskSpecTTLSecondsAfterFinished(t *testing.T) {
	tests := []struct {
		name      string
		taskName  string
		ttl       *int32
		expectErr bool
	}{
		{
			name:      "Valid TTL - nil (uses default)",
			taskName:  "task-ttl-nil",
			ttl:       nil,
			expectErr: false,
		},
		{
			name:      "Valid TTL - positive value",
			taskName:  "task-ttl-positive",
			ttl:       ptr.To(int32(1800)),
			expectErr: false,
		},
		{
			name:      "Invalid TTL - zero",
			taskName:  "task-ttl-zero",
			ttl:       ptr.To(int32(0)),
			expectErr: true,
		},
		{
			name:      "Invalid TTL - negative",
			taskName:  "task-ttl-negative",
			ttl:       ptr.To(int32(-100)),
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			task := testutils.EtcdOpsTaskBuilderWithoutDefaults(test.taskName, testNs).WithEtcdName("test-etcd").WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{Type: druidv1alpha1.OnDemandSnapshotTypeFull}).Build()
			task.Spec.TTLSecondsAfterFinished = test.ttl
			validateEtcdOpsTaskCreation(g, task, test.expectErr)
		})
	}
}

// TestValidateEtcdOpsTaskSpecConfigOndemandSnapshot tests OnDemandSnapshot config validation
func TestValidateEtcdOpsTaskSpecOndemandSnapshotConfig(t *testing.T) {
	tests := []struct {
		name      string
		taskName  string
		config    *druidv1alpha1.OnDemandSnapshotConfig
		expectErr bool
	}{
		{
			name:     "Valid OnDemandSnapshot - full type",
			taskName: "task-valid-full",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type: druidv1alpha1.OnDemandSnapshotTypeFull,
			},
			expectErr: false,
		},
		{
			name:     "Valid OnDemandSnapshot - full with isFinal true",
			taskName: "task-full-isfinal-true",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type:    druidv1alpha1.OnDemandSnapshotTypeFull,
				IsFinal: ptr.To(true),
			},
			expectErr: false,
		},
		{
			name:     "Valid OnDemandSnapshot - delta type",
			taskName: "task-valid-delta",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type: druidv1alpha1.OnDemandSnapshotTypeDelta,
			},
			expectErr: false,
		},
		{
			name:     "Valid OnDemandSnapshot - full with isFinal false",
			taskName: "task-full-isfinal-false",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type:    druidv1alpha1.OnDemandSnapshotTypeFull,
				IsFinal: ptr.To(false),
			},
			expectErr: false,
		},
		{
			name:     "Valid OnDemandSnapshot - positive timeout",
			taskName: "task-positive-timeout",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(120)),
			},
			expectErr: false,
		},
		{
			name:     "Invalid OnDemandSnapshot - delta with isFinal true",
			taskName: "task-delta-isfinal-true",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type:    druidv1alpha1.OnDemandSnapshotTypeDelta,
				IsFinal: ptr.To(true),
			},
			expectErr: true,
		},
		{
			name:     "Invalid OnDemandSnapshot - zero timeout",
			taskName: "task-zero-timeout",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type:                druidv1alpha1.OnDemandSnapshotTypeDelta,
				TimeoutSecondsDelta: ptr.To(int32(0)),
			},
			expectErr: true,
		},
		{
			name:     "Invalid OnDemandSnapshot - full snapshot timeout less than minimum",
			taskName: "task-low-timeout-full",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(60)),
			},
			expectErr: true,
		},
		{
			name:     "Invalid OnDemandSnapshot - delta snapshot timeout less than minimum",
			taskName: "task-low-timeout-delta",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(9)),
			},
			expectErr: true,
		},
		{
			name:     "Invalid OnDemandSnapshot - negative timeout",
			taskName: "task-negative-timeout",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(-30)),
			},
			expectErr: true,
		},
		{
			name:     "Valid OnDemandSnapshot - enum value 'full'",
			taskName: "task-enum-full",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type: druidv1alpha1.OnDemandSnapshotTypeFull,
			},
			expectErr: false,
		},
		{
			name:     "Valid OnDemandSnapshot - enum value 'delta'",
			taskName: "task-enum-delta",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type: druidv1alpha1.OnDemandSnapshotTypeDelta,
			},
			expectErr: false,
		},
		{
			name:     "Invalid OnDemandSnapshot - invalid enum value",
			taskName: "task-enum-invalid-1",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type: druidv1alpha1.OnDemandSnapshotType("invalid"),
			},
			expectErr: true,
		},
		{
			name:     "Invalid OnDemandSnapshot - invalid enum value 2",
			taskName: "task-enum-invalid-2",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type: druidv1alpha1.OnDemandSnapshotType("incremental"),
			},
			expectErr: true,
		},
		{
			name:     "Invalid OnDemandSnapshot - empty enum value",
			taskName: "task-enum-invalid-3",
			config: &druidv1alpha1.OnDemandSnapshotConfig{
				Type: druidv1alpha1.OnDemandSnapshotType(""),
			},
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			task := testutils.EtcdOpsTaskBuilderWithoutDefaults(test.taskName, testNs).WithEtcdName("test-etcd").WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{Type: druidv1alpha1.OnDemandSnapshotTypeFull}).Build()
			task.Spec.Config.OnDemandSnapshot = test.config
			validateEtcdOpsTaskCreation(g, task, test.expectErr)
		})
	}
}

// TestValidateEtcdOpsTaskSpecDefaults tests that default values are properly applied
func TestValidateEtcdOpsTaskSpecDefaults(t *testing.T) {
	tests := []struct {
		name                        string
		taskName                    string
		expectedTTL                 int32
		expectedTimeoutSecondsFull  int32
		expectedTimeoutSecondsDelta int32
	}{
		{
			name:                        "Default values set for spec.TTLSecondsAfterFinished and spec.config.onDemandSnapshot.timeoutSeconds",
			taskName:                    "task-defaults",
			expectedTTL:                 3600,
			expectedTimeoutSecondsFull:  900,
			expectedTimeoutSecondsDelta: 60,
		},
	}

	testNs, g := setupTestEnvironment(t)
	cl := itTestEnv.GetClient()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			task := testutils.EtcdOpsTaskBuilderWithoutDefaults(test.taskName, testNs).WithEtcdName("test-etcd").WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{Type: druidv1alpha1.OnDemandSnapshotTypeFull}).Build()
			validateEtcdOpsTaskCreation(g, task, false)

			fetchedTask := &druidv1alpha1.EtcdOpsTask{}
			g.Expect(cl.Get(context.Background(), client.ObjectKey{Name: test.taskName, Namespace: testNs}, fetchedTask)).To(Succeed())

			g.Expect(fetchedTask.Spec.TTLSecondsAfterFinished).ToNot(BeNil())
			g.Expect(*fetchedTask.Spec.TTLSecondsAfterFinished).To(Equal(test.expectedTTL))

			g.Expect(fetchedTask.Spec.Config.OnDemandSnapshot.TimeoutSecondsFull).ToNot(BeNil())
			g.Expect(*fetchedTask.Spec.Config.OnDemandSnapshot.TimeoutSecondsFull).To(Equal(test.expectedTimeoutSecondsFull))

			g.Expect(fetchedTask.Spec.Config.OnDemandSnapshot.TimeoutSecondsDelta).ToNot(BeNil())
			g.Expect(*fetchedTask.Spec.Config.OnDemandSnapshot.TimeoutSecondsDelta).To(Equal(test.expectedTimeoutSecondsDelta))
		})
	}
}
