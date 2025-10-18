// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// assertEtcdOpsTaskStateAndErrorCode waits for the EtcdOpsTask to reach the specified state with the expected error code (or nil for success) in the given phase.
func assertEtcdOpsTaskStateAndErrorCode(ctx context.Context, g *WithT, t *testing.T, cl client.Client, etcdOpsTask *druidv1alpha1.EtcdOpsTask, expectedTaskState druidv1alpha1.TaskState, expectedOperationState druidv1alpha1.LastOperationState, expectedPhase druidv1alpha1.OperationPhase, expectedErrorCode *druidv1alpha1.ErrorCode) {
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask); err != nil {
			return false
		}

		// Check if task state matches
		if etcdOpsTask.Status.State == nil || *etcdOpsTask.Status.State != expectedTaskState {
			return false
		}

		// Check if LastOperation exists and matches expected state
		if etcdOpsTask.Status.LastOperation == nil || etcdOpsTask.Status.LastOperation.State != expectedOperationState {
			return false
		}

		// Check if Phase exists and matches expected phase
		if etcdOpsTask.Status.Phase == nil || *etcdOpsTask.Status.Phase != expectedPhase {
			return false
		}

		// All basic checks passed, now check for error conditions
		if expectedErrorCode != nil {
			// We expect an error code, so check LastErrors
			if len(etcdOpsTask.Status.LastErrors) > 0 {
				for _, lastError := range etcdOpsTask.Status.LastErrors {
					if lastError.Code == *expectedErrorCode {
						t.Logf("task correctly reached state %s in phase %s with operation state %s and error code %s: %s", expectedTaskState, expectedPhase, expectedOperationState, *expectedErrorCode, lastError.Description)
						return true
					}
				}
			}
			return false
		} else {
			// No error expected, success case
			t.Logf("task correctly reached state %s in phase %s with operation state %s: %s", expectedTaskState, expectedPhase, expectedOperationState, etcdOpsTask.Status.LastOperation.Description)
			return true
		}
	}, 15*time.Second, 1*time.Second).Should(BeTrue(), func() string {
		if expectedErrorCode != nil {
			return fmt.Sprintf("task should reach state %s with error code %s", expectedTaskState, *expectedErrorCode)
		}
		return fmt.Sprintf("task should reach state %s successfully", expectedTaskState)
	}())
}

// assertEtcdOpsTaskDeletedAfterTTL waits for the EtcdOpsTask to be deleted after TTL expires for tasks that reached the specified phase.
func assertEtcdOpsTaskDeletedAfterTTL(ctx context.Context, g *WithT, t *testing.T, cl client.Client, etcdOpsTask *druidv1alpha1.EtcdOpsTask, expectedFinalPhase druidv1alpha1.OperationPhase) {
	t.Logf("waiting for task to reach final phase: %s", expectedFinalPhase)
	err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask)
	g.Expect(err).NotTo(HaveOccurred())

	// Calculate timeout based on TTL plus some buffer time for processing
	ttlSeconds := int32(10)
	if etcdOpsTask.Spec.TTLSecondsAfterFinished != nil {
		ttlSeconds = *etcdOpsTask.Spec.TTLSecondsAfterFinished
	}

	// Add buffer time for processing delays
	timeoutDuration := time.Duration(ttlSeconds+10) * time.Second

	g.Eventually(func() bool {
		err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				t.Logf("task %s/%s was successfully deleted after TTL expiry", etcdOpsTask.Namespace, etcdOpsTask.Name)
				return true
			}
			t.Logf("error checking task deletion: %v", err)
			return false
		}
		return false
	}, timeoutDuration, 2*time.Second).Should(BeTrue(), "task should be deleted after TTL expires")
}
