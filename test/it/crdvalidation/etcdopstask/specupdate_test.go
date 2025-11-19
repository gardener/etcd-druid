// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations on spec updates for etcdopstask.spec fields.
package etcdopstask

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

// skipCELTestsForOlderK8sVersions skips tests that require CEL validation (k8s >= 1.29)
func skipCELTestsForOlderK8sVersions(t *testing.T) {
	if !k8sVersionAbove129 {
		t.Skip("Skipping CEL validation tests for k8s versions below 1.29")
	}
}

// TestValidateUpdateSpecConfig tests immutability of etcdopstask.spec.config
func TestValidateUpdateSpecConfig(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)

	tests := []struct {
		name        string
		taskName    string
		initialType druidv1alpha1.OnDemandSnapshotType
		updatedType druidv1alpha1.OnDemandSnapshotType
		expectErr   bool
	}{
		{
			name:        "Valid #1: Unchanged config type",
			taskName:    "task-valid-config-1",
			initialType: druidv1alpha1.OnDemandSnapshotTypeFull,
			updatedType: druidv1alpha1.OnDemandSnapshotTypeFull,
			expectErr:   false,
		},
		{
			name:        "Invalid #1: Changed config type from full to delta",
			taskName:    "task-invalid-config-1",
			initialType: druidv1alpha1.OnDemandSnapshotTypeFull,
			updatedType: druidv1alpha1.OnDemandSnapshotTypeDelta,
			expectErr:   true,
		},
		{
			name:        "Invalid #2: Changed config type from delta to full",
			taskName:    "task-invalid-config-2",
			initialType: druidv1alpha1.OnDemandSnapshotTypeDelta,
			updatedType: druidv1alpha1.OnDemandSnapshotTypeFull,
			expectErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			task := createBasicEtcdOpsTask(test.taskName, testNs)
			task.Spec.Config.OnDemandSnapshot.Type = test.initialType

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, task)).To(Succeed())

			task.Spec.Config.OnDemandSnapshot.Type = test.updatedType
			validateEtcdOpsTaskUpdate(g, task, test.expectErr, ctx, cl)
		})
	}
}

// TestValidateUpdateSpecTTLSecondsAfterFinished tests immutability of etcdopstask.spec.ttlSecondsAfterFinished
func TestValidateUpdateSpecTTLSecondsAfterFinished(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)

	tests := []struct {
		name       string
		taskName   string
		initialTTL *int32
		updatedTTL *int32
		expectErr  bool
	}{
		{
			name:       "Valid #1: Unchanged TTL",
			taskName:   "task-valid-ttl-1",
			initialTTL: ptr.To(int32(3600)),
			updatedTTL: ptr.To(int32(3600)),
			expectErr:  false,
		},
		{
			name:       "Valid #2: Both nil TTL",
			taskName:   "task-valid-ttl-2",
			initialTTL: nil,
			updatedTTL: nil,
			expectErr:  false,
		},
		{
			name:       "Invalid #1: Updated TTL value",
			taskName:   "task-invalid-ttl-1",
			initialTTL: ptr.To(int32(3600)),
			updatedTTL: ptr.To(int32(7200)),
			expectErr:  true,
		},
		{
			name:       "Invalid #2: Set unset TTL",
			taskName:   "task-invalid-ttl-2",
			initialTTL: nil,
			updatedTTL: ptr.To(int32(1800)),
			expectErr:  true,
		},
		{
			name:       "Invalid #3: Unset set TTL",
			taskName:   "task-invalid-ttl-3",
			initialTTL: ptr.To(int32(1800)),
			updatedTTL: nil,
			expectErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			task := createBasicEtcdOpsTask(test.taskName, testNs)
			task.Spec.TTLSecondsAfterFinished = test.initialTTL

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, task)).To(Succeed())

			// Try to update TTL
			task.Spec.TTLSecondsAfterFinished = test.updatedTTL
			validateEtcdOpsTaskUpdate(g, task, test.expectErr, ctx, cl)
		})
	}
}

// TestValidateUpdateSpecEtcdRef tests immutability of etcdopstask.spec.etcdRef
func TestValidateUpdateSpecEtcdRef(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)

	tests := []struct {
		name            string
		taskName        string
		initialEtcdName string
		updatedEtcdName string
		expectErr       bool
	}{
		{
			name:            "Valid #1: Unchanged etcdRef name",
			taskName:        "task-valid-etcdref-1",
			initialEtcdName: "test-etcd",
			updatedEtcdName: "test-etcd",
			expectErr:       false,
		},
		{
			name:            "Invalid #1: Updated etcdRef name",
			taskName:        "task-invalid-etcdref-1",
			initialEtcdName: "test-etcd",
			updatedEtcdName: "different-etcd",
			expectErr:       true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			task := createBasicEtcdOpsTask(test.taskName, testNs)
			task.Spec.EtcdName = ptr.To(test.initialEtcdName)

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, task)).To(Succeed())

			task.Spec.EtcdName = ptr.To(test.updatedEtcdName)
			validateEtcdOpsTaskUpdate(g, task, test.expectErr, ctx, cl)
		})
	}
}
