// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"
	"testing"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// assertEtcdOpsTaskStateAndErrorCode waits for the EtcdOpsTask to reach the specified state with the expected error code (or nil for success) in the given phase.
func AssertEtcdOpsTaskStateAndErrorCode(ctx context.Context, g *WithT, t *testing.T, cl client.Client, etcdOpsTask *druidv1alpha1.EtcdOpsTask, expectedTaskState druidv1alpha1.TaskState, expectedPhase druidapicommon.LastOperationType, expectedOperationState druidapicommon.LastOperationState, expectedErrorCode *druidapicommon.ErrorCode) {
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask); err != nil {
			return false
		}

		if etcdOpsTask.Status.State == nil || *etcdOpsTask.Status.State != expectedTaskState {
			return false
		}

		if etcdOpsTask.Status.LastOperation == nil || etcdOpsTask.Status.LastOperation.State != expectedOperationState || etcdOpsTask.Status.LastOperation.Type != expectedPhase {
			return false
		}

		if expectedErrorCode != nil {
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
			t.Logf("task correctly reached state %s in phase %s with operation state %s: %s", expectedTaskState, expectedPhase, expectedOperationState, etcdOpsTask.Status.LastOperation.Description)
			return true
		}
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), func() string {
		if expectedErrorCode != nil {
			return fmt.Sprintf("task should reach state %s with error code %s", expectedTaskState, *expectedErrorCode)
		}
		return fmt.Sprintf("task should reach state %s successfully", expectedTaskState)
	}())
}

// assertEtcdOpsTaskDeletedAfterTTL waits for the EtcdOpsTask to be deleted after TTL expires for tasks that reached the specified phase.
func AssertEtcdOpsTaskDeletedAfterTTL(ctx context.Context, g *WithT, t *testing.T, cl client.Client, etcdOpsTask *druidv1alpha1.EtcdOpsTask, expectedFinalPhase druidapicommon.LastOperationType) {
	t.Logf("waiting for task to reach final phase: %s", expectedFinalPhase)
	err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask)
	g.Expect(err).NotTo(HaveOccurred())

	timeoutDuration := time.Duration(*etcdOpsTask.Spec.TTLSecondsAfterFinished+5) * time.Second

	g.Eventually(func() bool {
		err = cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				t.Logf("task %s/%s was successfully deleted after TTL expiry", etcdOpsTask.Namespace, etcdOpsTask.Name)
				return true
			}
			t.Logf("error checking task deletion: %v", err)
			return false
		}
		return false
	}, timeoutDuration, 500*time.Millisecond).Should(BeTrue(), "task should be deleted after TTL expires")
}
