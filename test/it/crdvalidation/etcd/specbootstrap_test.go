// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Tests validation on the etcd.spec.etcd.bootstrapWithExistingCluster field.
// Two aspects are covered here:
//   - Create-time immutability: the field cannot be added on an update.
//   - In-flight immutability: while the BootstrappedWithExistingCluster
//     condition is False (bootstrap in progress), .members and
//     .clientEndpoints cannot be modified.
package etcd

import (
	"context"
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

// TestValidateUpdateSpecBootstrapWithExistingClusterCreateOnly verifies that
// bootstrapWithExistingCluster cannot be added on an update to an existing
// Etcd. Enforced by a CEL transition rule on EtcdConfig.
func TestValidateUpdateSpecBootstrapWithExistingClusterCreateOnly(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	ctx := context.Background()
	cl := itTestEnv.GetClient()

	// Create an Etcd without the field.
	etcd := utils.EtcdBuilderWithoutDefaults("etcd-add-on-update", testNs).WithReplicas(3).Build()
	g.Expect(cl.Create(ctx, etcd)).To(Succeed())

	// Attempt to add bootstrapWithExistingCluster on update — must be rejected.
	etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
		Members: []druidv1alpha1.BootstrapExistingMember{
			{Name: "src-0", PeerURLs: []string{"http://10.0.0.1:2380"}},
		},
		ClientEndpoints: []string{"http://10.0.0.1:2379"},
	}
	validateEtcdUpdate(g, etcd, true, ctx, cl)
}

// TestValidateUpdateSpecBootstrapWithExistingClusterFreezeWhileInProgress
// verifies the top-level CEL rules on Etcd that freeze .members and
// .clientEndpoints while the BootstrappedWithExistingCluster condition is
// False. Edits are permitted when no condition is present yet or when it is
// True; edits are rejected only during the in-progress window.
func TestValidateUpdateSpecBootstrapWithExistingClusterFreezeWhileInProgress(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	ctx := context.Background()
	cl := itTestEnv.GetClient()

	// baseSpec returns a fresh, minimal spec.etcd.bootstrapWithExistingCluster
	// pointing at a single source member. Individual tests copy it and then
	// mutate the copy to trigger — or not trigger — the freeze rules.
	baseSpec := func() *druidv1alpha1.BootstrapWithExistingCluster {
		return &druidv1alpha1.BootstrapWithExistingCluster{
			Members: []druidv1alpha1.BootstrapExistingMember{
				{Name: "src-0", PeerURLs: []string{"http://10.0.0.1:2380"}},
			},
			ClientEndpoints: []string{"http://10.0.0.1:2379"},
		}
	}

	// setCondition writes .status.conditions on the given Etcd through the
	// status subresource. Used by tests to simulate the reconciler having
	// recorded a specific bootstrap state before the operator's spec update.
	setCondition := func(etcd *druidv1alpha1.Etcd, status druidv1alpha1.ConditionStatus) {
		etcd.Status.Conditions = []druidv1alpha1.Condition{
			{
				Type:               druidv1alpha1.ConditionTypeBootstrappedWithExistingCluster,
				Status:             status,
				LastTransitionTime: metav1.Now(),
				LastUpdateTime:     metav1.Now(),
				Reason:             "TestReason",
				Message:            "test message",
			},
		}
		g.Expect(cl.Status().Update(ctx, etcd)).To(Succeed())
	}

	tests := []struct {
		name string
		// conditionStatus, when set, is written to status.conditions before
		// the spec update. An empty string leaves status untouched (no
		// condition at all).
		conditionStatus druidv1alpha1.ConditionStatus
		// mutate is applied to the etcd's spec.etcd.bootstrapWithExistingCluster
		// before the update. If nil, the field is left equal to the initial
		// value (the "no-op update" case).
		mutate    func(b *druidv1alpha1.BootstrapWithExistingCluster)
		expectErr bool
	}{
		{
			name:            "Valid #1: No condition yet — members change is allowed",
			conditionStatus: "",
			mutate: func(b *druidv1alpha1.BootstrapWithExistingCluster) {
				b.Members = append(b.Members, druidv1alpha1.BootstrapExistingMember{
					Name:     "src-1",
					PeerURLs: []string{"http://10.0.0.2:2380"},
				})
			},
			expectErr: false,
		},
		{
			name:            "Valid #2: Condition True — members change is allowed",
			conditionStatus: druidv1alpha1.ConditionTrue,
			mutate: func(b *druidv1alpha1.BootstrapWithExistingCluster) {
				b.Members = append(b.Members, druidv1alpha1.BootstrapExistingMember{
					Name:     "src-1",
					PeerURLs: []string{"http://10.0.0.2:2380"},
				})
			},
			expectErr: false,
		},
		{
			name:            "Valid #3: Condition False, no-op spec update is allowed",
			conditionStatus: druidv1alpha1.ConditionFalse,
			mutate:          nil,
			expectErr:       false,
		},
		{
			name:            "Invalid #1: Condition False, members change is rejected",
			conditionStatus: druidv1alpha1.ConditionFalse,
			mutate: func(b *druidv1alpha1.BootstrapWithExistingCluster) {
				b.Members = append(b.Members, druidv1alpha1.BootstrapExistingMember{
					Name:     "src-1",
					PeerURLs: []string{"http://10.0.0.2:2380"},
				})
			},
			expectErr: true,
		},
		{
			name:            "Invalid #2: Condition False, members[*].peerUrls change is rejected",
			conditionStatus: druidv1alpha1.ConditionFalse,
			mutate: func(b *druidv1alpha1.BootstrapWithExistingCluster) {
				b.Members[0].PeerURLs = append(b.Members[0].PeerURLs, "http://10.0.0.99:2380")
			},
			expectErr: true,
		},
		{
			name:            "Invalid #3: Condition False, clientEndpoints change is rejected",
			conditionStatus: druidv1alpha1.ConditionFalse,
			mutate: func(b *druidv1alpha1.BootstrapWithExistingCluster) {
				b.ClientEndpoints = append(b.ClientEndpoints, "http://10.0.0.2:2379")
			},
			expectErr: true,
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcdName := fmt.Sprintf("etcd-bootstrap-freeze-%d", i)
			etcd := utils.EtcdBuilderWithoutDefaults(etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Etcd.BootstrapWithExistingCluster = baseSpec()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			if test.conditionStatus != "" {
				setCondition(etcd, test.conditionStatus)
			}

			if test.mutate != nil {
				test.mutate(etcd.Spec.Etcd.BootstrapWithExistingCluster)
			}
			validateEtcdUpdate(g, etcd, test.expectErr, ctx, cl)
		})
	}
}

// TestValidateCreateSpecBootstrapWithExistingClusterUniqueMemberNames verifies
// that bootstrapWithExistingCluster.members[*].name is unique across the list.
// Enforced by a top-level CEL rule on Etcd.
func TestValidateCreateSpecBootstrapWithExistingClusterUniqueMemberNames(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	ctx := context.Background()
	cl := itTestEnv.GetClient()

	tests := []struct {
		name      string
		etcdName  string
		members   []druidv1alpha1.BootstrapExistingMember
		expectErr bool
	}{
		{
			name:     "Valid: all names unique",
			etcdName: "etcd-unique-1",
			members: []druidv1alpha1.BootstrapExistingMember{
				{Name: "src-0", PeerURLs: []string{"http://10.0.0.1:2380"}},
				{Name: "src-1", PeerURLs: []string{"http://10.0.0.2:2380"}},
				{Name: "src-2", PeerURLs: []string{"http://10.0.0.3:2380"}},
			},
			expectErr: false,
		},
		{
			name:     "Invalid: duplicate name",
			etcdName: "etcd-unique-2",
			members: []druidv1alpha1.BootstrapExistingMember{
				{Name: "src-0", PeerURLs: []string{"http://10.0.0.1:2380"}},
				{Name: "src-0", PeerURLs: []string{"http://10.0.0.2:2380"}},
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
				Members:         test.members,
				ClientEndpoints: []string{"http://10.0.0.1:2379"},
			}
			if test.expectErr {
				g.Expect(cl.Create(ctx, etcd)).NotTo(Succeed())
			} else {
				g.Expect(cl.Create(ctx, etcd)).To(Succeed())
			}
		})
	}
}

// TestValidateCreateSpecBootstrapWithExistingClusterNoTargetCollision verifies
// that no bootstrapWithExistingCluster.members[*].name may collide with a
// target member name (either <etcd.Name>-<i> or <memberNamePrefix>-<etcd.Name>-<i>).
// Enforced by a top-level CEL rule on Etcd.
func TestValidateCreateSpecBootstrapWithExistingClusterNoTargetCollision(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	ctx := context.Background()
	cl := itTestEnv.GetClient()

	tests := []struct {
		name             string
		etcdName         string
		memberNamePrefix *string
		bootstrapName    string
		expectErr        bool
	}{
		{
			name:          "Valid: bootstrap name unrelated to target prefix",
			etcdName:      "etcd-collision-1",
			bootstrapName: "src-0",
			expectErr:     false,
		},
		{
			name:          "Invalid: bootstrap name starts with target's Etcd name",
			etcdName:      "etcd-collision-2",
			bootstrapName: "etcd-collision-2-0",
			expectErr:     true,
		},
		{
			name:             "Valid: with memberNamePrefix, bootstrap name that would only collide without the prefix is allowed",
			etcdName:         "etcd-collision-3",
			memberNamePrefix: ptr.To("main"),
			// Would have collided with "etcd-collision-3-0" but the actual target prefix is "main-etcd-collision-3-".
			bootstrapName: "etcd-collision-3-0",
			expectErr:     false,
		},
		{
			name:             "Invalid: with memberNamePrefix, bootstrap name starts with prefix-etcdname-",
			etcdName:         "etcd-collision-4",
			memberNamePrefix: ptr.To("main"),
			bootstrapName:    "main-etcd-collision-4-0",
			expectErr:        true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			if test.memberNamePrefix != nil {
				etcd.Spec.MemberNamePrefix = test.memberNamePrefix
			}
			etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
				Members: []druidv1alpha1.BootstrapExistingMember{
					{Name: test.bootstrapName, PeerURLs: []string{"http://10.0.0.1:2380"}},
				},
				ClientEndpoints: []string{"http://10.0.0.1:2379"},
			}
			if test.expectErr {
				g.Expect(cl.Create(ctx, etcd)).NotTo(Succeed())
			} else {
				g.Expect(cl.Create(ctx, etcd)).To(Succeed())
			}
		})
	}
}

// TestValidateCreateSpecBootstrapWithExistingClusterURLUniqueness verifies that
// bootstrapWithExistingCluster.members[*].peerUrls and .clientEndpoints reject
// duplicate URL entries. Enforced by the +listType=set marker on each field.
func TestValidateCreateSpecBootstrapWithExistingClusterURLUniqueness(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	ctx := context.Background()
	cl := itTestEnv.GetClient()

	tests := []struct {
		name            string
		etcdName        string
		peerURLs        []string
		clientEndpoints []string
		expectErr       bool
	}{
		{
			name:            "Valid: unique peerUrls and unique clientEndpoints",
			etcdName:        "etcd-url-unique-1",
			peerURLs:        []string{"http://10.0.0.1:2380", "http://10.0.0.2:2380"},
			clientEndpoints: []string{"http://10.0.0.1:2379"},
			expectErr:       false,
		},
		{
			name:            "Invalid: duplicate peerUrl within a member",
			etcdName:        "etcd-url-unique-2",
			peerURLs:        []string{"http://10.0.0.1:2380", "http://10.0.0.1:2380"},
			clientEndpoints: []string{"http://10.0.0.1:2379"},
			expectErr:       true,
		},
		{
			name:            "Invalid: duplicate clientEndpoint",
			etcdName:        "etcd-url-unique-3",
			peerURLs:        []string{"http://10.0.0.1:2380"},
			clientEndpoints: []string{"http://10.0.0.1:2379", "http://10.0.0.1:2379"},
			expectErr:       true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
				Members: []druidv1alpha1.BootstrapExistingMember{
					{Name: "src-0", PeerURLs: test.peerURLs},
				},
				ClientEndpoints: test.clientEndpoints,
			}
			if test.expectErr {
				g.Expect(cl.Create(ctx, etcd)).NotTo(Succeed())
			} else {
				g.Expect(cl.Create(ctx, etcd)).To(Succeed())
			}
		})
	}
}
