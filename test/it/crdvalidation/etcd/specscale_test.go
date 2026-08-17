// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing DEP-08 scale coordination CEL rules on etcd.spec updates. These rules
// read the ScaleOperationComplete status condition to reject conflicting
// opposite-direction membership changes while an operation is in flight. The
// condition uses positive polarity, so an in-flight operation is status == False
// with the reason naming it.
package etcd

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
)

// TestValidateUpdateScaleOperationCoordination exercises the object-level CEL
// rules that gate replica-direction changes on the ScaleOperationComplete
// condition.
func TestValidateUpdateScaleOperationCoordination(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	ctx := context.Background()
	cl := itTestEnv.GetClient()

	// setScaleCondition writes a ScaleOperationComplete condition with the given
	// status and reason through the status subresource, simulating the reconciler
	// having recorded an in-flight scale operation (status False) or a converged
	// cluster (status True).
	setScaleCondition := func(etcd *druidv1alpha1.Etcd, status druidv1alpha1.ConditionStatus, reason string) {
		etcd.Status.Conditions = []druidv1alpha1.Condition{
			{
				Type:               druidv1alpha1.ConditionTypeScaleOperationComplete,
				Status:             status,
				LastTransitionTime: metav1.Now(),
				LastUpdateTime:     metav1.Now(),
				Reason:             reason,
				Message:            "test message",
			},
		}
		g.Expect(cl.Status().Update(ctx, etcd)).To(Succeed())
	}

	tests := []struct {
		name            string
		initialReplicas int32
		updatedReplicas int32
		// conditionStatus/conditionReason, when set, are written before the update.
		// An empty status leaves status.conditions untouched. An in-flight operation
		// is ConditionFalse with the operation reason; a converged cluster is
		// ConditionTrue/NoScaleOperation.
		conditionStatus druidv1alpha1.ConditionStatus
		conditionReason string
		expectErr       bool
	}{
		{
			name:            "Valid: scale-out with no scale operation in progress",
			initialReplicas: 3,
			updatedReplicas: 5,
			expectErr:       false,
		},
		{
			name:            "Valid: scale-in with no scale operation in progress",
			initialReplicas: 5,
			updatedReplicas: 3,
			expectErr:       false,
		},
		{
			name:            "Valid: scale-out while ScalingOut in progress (same direction)",
			initialReplicas: 3,
			updatedReplicas: 5,
			conditionStatus: druidv1alpha1.ConditionFalse,
			conditionReason: druidv1alpha1.ScaleOperationReasonScalingOut,
			expectErr:       false,
		},
		{
			name:            "Valid: scale-in while ScalingIn in progress (same direction)",
			initialReplicas: 5,
			updatedReplicas: 3,
			conditionStatus: druidv1alpha1.ConditionFalse,
			conditionReason: druidv1alpha1.ScaleOperationReasonScalingIn,
			expectErr:       false,
		},
		{
			name:            "Valid: scale-in to zero is never blocked",
			initialReplicas: 3,
			updatedReplicas: 0,
			conditionStatus: druidv1alpha1.ConditionFalse,
			conditionReason: druidv1alpha1.ScaleOperationReasonScalingOut,
			expectErr:       false,
		},
		{
			name:            "Valid: converged condition (True/NoScaleOperation) does not block scale-in",
			initialReplicas: 5,
			updatedReplicas: 3,
			conditionStatus: druidv1alpha1.ConditionTrue,
			conditionReason: druidv1alpha1.ScaleOperationReasonNoScaleOperation,
			expectErr:       false,
		},
		{
			name:            "Invalid: scale-out while ScalingIn in progress",
			initialReplicas: 3,
			updatedReplicas: 5,
			conditionStatus: druidv1alpha1.ConditionFalse,
			conditionReason: druidv1alpha1.ScaleOperationReasonScalingIn,
			expectErr:       true,
		},
		{
			name:            "Invalid: scale-out while BootstrapMembersRemoval in progress",
			initialReplicas: 3,
			updatedReplicas: 5,
			conditionStatus: druidv1alpha1.ConditionFalse,
			conditionReason: druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
			expectErr:       true,
		},
		{
			name:            "Invalid: scale-in while ScalingOut in progress",
			initialReplicas: 5,
			updatedReplicas: 3,
			conditionStatus: druidv1alpha1.ConditionFalse,
			conditionReason: druidv1alpha1.ScaleOperationReasonScalingOut,
			expectErr:       true,
		},
		{
			name:            "Invalid: scale-in while BootstrapMembersRemoval in progress",
			initialReplicas: 5,
			updatedReplicas: 3,
			conditionStatus: druidv1alpha1.ConditionFalse,
			conditionReason: druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
			expectErr:       true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcdName := utils.GenerateTestNamespaceName(t, "etcd-scale", 8)
			etcd := utils.EtcdBuilderWithoutDefaults(etcdName, testNs).WithReplicas(test.initialReplicas).Build()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			if test.conditionStatus != "" {
				setScaleCondition(etcd, test.conditionStatus, test.conditionReason)
			}

			etcd.Spec.Replicas = test.updatedReplicas
			validateEtcdUpdate(g, etcd, test.expectErr, ctx, cl)
		})
	}
}
