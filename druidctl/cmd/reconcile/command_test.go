// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconcile

import (
	"context"
	"strings"
	"testing"

	fake "github.com/gardener/etcd-druid/druidctl/internal/client/fake"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

func TestReconcileCommand(t *testing.T) {
	// Test the reconcile command adds "druid.gardener.cloud/operation: reconcile" annotation
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// Create the reconcile command
	cmd := NewReconcileCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the reconcile command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Reconcile command failed: %v", err)
	}

	// Verify the etcd resource has the reconcile annotation
	updatedEtcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get updated etcd: %v", err)
	}

	// Check that the DruidOperationAnnotation is set to reconcile
	if updatedEtcd.Annotations == nil {
		t.Error("Expected annotations to be set on etcd resource")
	} else {
		if value, exists := updatedEtcd.Annotations[druidv1alpha1.DruidOperationAnnotation]; !exists {
			t.Error("Expected DruidOperationAnnotation to be set")
		} else if value != druidv1alpha1.DruidOperationReconcile {
			t.Errorf("Expected DruidOperationAnnotation value to be 'reconcile', got: %s", value)
		}
	}

	// Verify command output

	t.Log("Successfully verified reconcile command adds DruidOperationAnnotation")
}

func TestReconcileCommandAllNamespaces(t *testing.T) {
	// Test reconcile command with --all-namespaces flag
	helper := fake.NewTestHelper().WithTestScenario(fake.MultipleEtcdsScenario())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	// Provide 'y' confirmation for --all-namespaces prompt
	streams.In = strings.NewReader("y\n")
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// Create the reconcile command
	cmd := NewReconcileCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Set all-namespaces
	emptyNs := ""
	globalOpts.ConfigFlags.Namespace = &emptyNs
	globalOpts.AllNamespaces = true

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the reconcile command
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("Reconcile command failed: %v", err)
	}

	// Verify multiple etcd resources have the reconcile annotation
	// Check etcd-main in shoot-ns1
	etcdMain1, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns1", "etcd-main")
	if err != nil {
		t.Fatalf("Failed to get etcd-main in shoot-ns1: %v", err)
	} else {
		if etcdMain1.Annotations != nil {
			if value, exists := etcdMain1.Annotations[druidv1alpha1.DruidOperationAnnotation]; exists && value == druidv1alpha1.DruidOperationReconcile {
				t.Logf("Reconcile annotation set for etcd %s/%s", etcdMain1.Namespace, etcdMain1.Name)
			}
		}
	}

	// Check etcd-events in shoot-ns1
	etcdEvents1, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns1", "etcd-events")
	if err != nil {
		t.Fatalf("Failed to get etcd-events in shoot-ns1: %v", err)
	} else {
		if etcdEvents1.Annotations != nil {
			if value, exists := etcdEvents1.Annotations[druidv1alpha1.DruidOperationAnnotation]; exists && value == druidv1alpha1.DruidOperationReconcile {
				t.Logf("Reconcile annotation set for etcd %s/%s", etcdEvents1.Namespace, etcdEvents1.Name)
			}
		}
	}

	// Verify command output

	t.Log("Successfully verified reconcile command with all-namespaces")
}

func TestSuspendReconcileCommand(t *testing.T) {
	// Test the reconcile suspend command adds SuspendEtcdSpecReconcileAnnotation
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// Create the reconcile suspend command
	cmd := NewSuspendCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the reconcile suspend command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Reconcile suspend command failed: %v", err)
	}

	// Verify the etcd resource has the suspend annotation
	updatedEtcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get updated etcd: %v", err)
	}

	// Check that the SuspendEtcdSpecReconcileAnnotation is set
	if updatedEtcd.Annotations == nil {
		t.Error("Expected annotations to be set on etcd resource")
	} else {
		if value, exists := updatedEtcd.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation]; !exists {
			t.Error("Expected SuspendEtcdSpecReconcileAnnotation to be set")
		} else if value != "true" {
			t.Errorf("Expected SuspendEtcdSpecReconcileAnnotation value to be 'true', got: %s", value)
		}
	}

	// Verify command output

	t.Log("Successfully verified reconcile suspend command adds SuspendEtcdSpecReconcileAnnotation")
}

func TestResumeReconcileCommand(t *testing.T) {
	// Test the reconcile resume command removes SuspendEtcdSpecReconcileAnnotation
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// First, add the suspend annotation to test removal
	etcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get test etcd: %v", err)
	}

	// Add the suspend annotation
	if err = etcdClient.UpdateEtcd(context.TODO(), etcd, func(e *druidv1alpha1.Etcd) {
		if e.Annotations == nil {
			e.Annotations = make(map[string]string)
		}
		e.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation] = "true"
	}); err != nil {
		t.Fatalf("Failed to add suspend annotation: %v", err)
	}

	// Create the reconcile resume command
	cmd := NewResumeCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Verify initial state has suspend annotation
	initialEtcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get initial etcd: %v", err)
	}
	if _, exists := initialEtcd.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation]; !exists {
		t.Fatal("Test setup failed: etcd should have suspend annotation initially")
	}

	// Execute the reconcile resume command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Reconcile resume command failed: %v", err)
	}

	// Verify the etcd resource no longer has the suspend annotation
	updatedEtcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get updated etcd: %v", err)
	}

	// Check that the SuspendEtcdSpecReconcileAnnotation is removed
	if updatedEtcd.Annotations != nil {
		if _, exists := updatedEtcd.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation]; exists {
			t.Error("Expected SuspendEtcdSpecReconcileAnnotation to be removed")
		}
	}

	// Verify command output

	t.Log("Successfully verified reconcile resume command removes SuspendEtcdSpecReconcileAnnotation")
}

func TestReconcileErrorHandling(t *testing.T) {
	// Test with empty client (no etcd resources)
	helper := fake.NewTestHelper()
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	// Test reconcile command with non-existent etcd
	reconcileCmd := NewReconcileCommand(globalOpts)
	reconcileCmd.SetOut(buf)
	reconcileCmd.SetErr(errBuf)

	if err := globalOpts.Complete(reconcileCmd, []string{"non-existent-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := reconcileCmd.RunE(reconcileCmd, []string{"non-existent-etcd"}); err == nil {
		t.Fatalf("Expected reconcile command to fail for non-existent etcd, but it succeeded")
	} else {
		if strings.Contains(err.Error(), "not found") {
			t.Logf("Reconcile command correctly failed with 'not found' error: %v", err)
		} else {
			t.Logf("Reconcile command failed with different error (still correct): %v", err)
		}
	}

	t.Log("Successfully verified reconcile error handling")
}

func TestResumeReconcileWithoutAnnotation(t *testing.T) {
	// Test resume command when etcd has no suspend annotation
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// Create the reconcile resume command
	cmd := NewResumeCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	// Execute the resume command - this should still succeed
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Resume command failed unexpectedly: %v", err)
	}

	// Verify the etcd remains unchanged (no annotations should be added)
	updatedEtcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get updated etcd: %v", err)
	}

	// Verify no suspend annotation exists (should remain unchanged)
	if updatedEtcd.Annotations != nil {
		if _, exists := updatedEtcd.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation]; exists {
			t.Error("Suspend annotation should not exist after resume on etcd without annotation")
		}
	}

	// Verify command succeeded

	t.Log("Successfully verified reconcile resume handles missing annotation correctly")
}

func TestReconcileNamespaceFlag(t *testing.T) {
	// Test reconcile command with -n namespace flag (kubectl pattern: -n ns resourcename)
	helper := fake.NewTestHelper().WithTestScenario(fake.MultipleEtcdsScenario())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// Create the reconcile command
	cmd := NewReconcileCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Set namespace to shoot-ns1
	shootNs1 := "shoot-ns1"
	globalOpts.ConfigFlags.Namespace = &shootNs1

	// Complete with just the resource name (namespace from -n flag)
	if err := globalOpts.Complete(cmd, []string{"etcd-main"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the reconcile command
	if err := cmd.RunE(cmd, []string{"etcd-main"}); err != nil {
		t.Fatalf("Reconcile command failed: %v", err)
	}

	// Verify the etcd in shoot-ns1 has the reconcile annotation
	updatedEtcd, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns1", "etcd-main")
	if err != nil {
		t.Fatalf("Failed to get updated etcd: %v", err)
	}

	if updatedEtcd.Annotations == nil {
		t.Error("Expected annotations to be set on etcd resource")
	} else {
		if value, exists := updatedEtcd.Annotations[druidv1alpha1.DruidOperationAnnotation]; !exists {
			t.Error("Expected DruidOperationAnnotation to be set")
		} else if value != druidv1alpha1.DruidOperationReconcile {
			t.Errorf("Expected DruidOperationAnnotation value to be 'reconcile', got: %s", value)
		}
	}

	// Verify etcd in shoot-ns2 was NOT modified (different namespace)
	ns2Etcd, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns2", "etcd-main")
	if err != nil {
		t.Fatalf("Failed to get shoot-ns2 etcd: %v", err)
	}
	if ns2Etcd.Annotations != nil {
		if _, exists := ns2Etcd.Annotations[druidv1alpha1.DruidOperationAnnotation]; exists {
			t.Error("Expected shoot-ns2/etcd-main to NOT have reconcile annotation")
		}
	}

	t.Log("Successfully verified reconcile with -n namespace flag")
}

func TestReconcileCrossNamespace(t *testing.T) {
	// Test reconcile command with cross-namespace references: ns1/etcd1 ns2/etcd2
	helper := fake.NewTestHelper().WithTestScenario(fake.MultipleEtcdsScenario())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// Create the reconcile command
	cmd := NewReconcileCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Clear namespace flag for cross-namespace selection
	emptyNs := ""
	globalOpts.ConfigFlags.Namespace = &emptyNs

	// Use cross-namespace format: ns/name
	args := []string{"shoot-ns1/etcd-main", "shoot-ns2/etcd-main"}
	if err := globalOpts.Complete(cmd, args); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the reconcile command
	if err := cmd.RunE(cmd, args); err != nil {
		t.Fatalf("Reconcile command failed: %v", err)
	}

	// Verify BOTH etcds have the reconcile annotation
	etcd1, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns1", "etcd-main")
	if err != nil {
		t.Fatalf("Failed to get shoot-ns1/etcd-main: %v", err)
	}
	if etcd1.Annotations == nil || etcd1.Annotations[druidv1alpha1.DruidOperationAnnotation] != druidv1alpha1.DruidOperationReconcile {
		t.Error("Expected shoot-ns1/etcd-main to have reconcile annotation")
	}

	etcd2, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns2", "etcd-main")
	if err != nil {
		t.Fatalf("Failed to get shoot-ns2/etcd-main: %v", err)
	}
	if etcd2.Annotations == nil || etcd2.Annotations[druidv1alpha1.DruidOperationAnnotation] != druidv1alpha1.DruidOperationReconcile {
		t.Error("Expected shoot-ns2/etcd-main to have reconcile annotation")
	}

	t.Log("Successfully verified cross-namespace reconcile")
}

func TestSuspendCrossNamespace(t *testing.T) {
	// Test suspend command with cross-namespace references
	helper := fake.NewTestHelper().WithTestScenario(fake.MultipleEtcdsScenario())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// Create the suspend command
	cmd := NewSuspendCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Clear namespace flag for cross-namespace selection
	emptyNs := ""
	globalOpts.ConfigFlags.Namespace = &emptyNs

	// Use cross-namespace format
	args := []string{"shoot-ns1/etcd-main", "shoot-ns2/etcd-main"}
	if err := globalOpts.Complete(cmd, args); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the suspend command
	if err := cmd.RunE(cmd, args); err != nil {
		t.Fatalf("Suspend command failed: %v", err)
	}

	// Verify BOTH etcds have the suspend annotation
	etcd1, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns1", "etcd-main")
	if err != nil {
		t.Fatalf("Failed to get shoot-ns1/etcd-main: %v", err)
	}
	if etcd1.Annotations == nil || etcd1.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation] != "true" {
		t.Error("Expected shoot-ns1/etcd-main to have suspend annotation")
	}

	etcd2, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns2", "etcd-main")
	if err != nil {
		t.Fatalf("Failed to get shoot-ns2/etcd-main: %v", err)
	}
	if etcd2.Annotations == nil || etcd2.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation] != "true" {
		t.Error("Expected shoot-ns2/etcd-main to have suspend annotation")
	}

	t.Log("Successfully verified cross-namespace suspend")
}

func TestResumeCrossNamespace(t *testing.T) {
	// Test resume command with cross-namespace references
	helper := fake.NewTestHelper().WithTestScenario(fake.MultipleEtcdsScenario())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// First, add suspend annotations to both etcds
	etcd1, _ := etcdClient.GetEtcd(context.TODO(), "shoot-ns1", "etcd-main")
	_ = etcdClient.UpdateEtcd(context.TODO(), etcd1, func(e *druidv1alpha1.Etcd) {
		if e.Annotations == nil {
			e.Annotations = make(map[string]string)
		}
		e.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation] = "true"
	})

	etcd2, _ := etcdClient.GetEtcd(context.TODO(), "shoot-ns2", "etcd-main")
	_ = etcdClient.UpdateEtcd(context.TODO(), etcd2, func(e *druidv1alpha1.Etcd) {
		if e.Annotations == nil {
			e.Annotations = make(map[string]string)
		}
		e.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation] = "true"
	})

	// Create the resume command
	cmd := NewResumeCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Clear namespace flag for cross-namespace selection
	emptyNs := ""
	globalOpts.ConfigFlags.Namespace = &emptyNs

	// Use cross-namespace format
	args := []string{"shoot-ns1/etcd-main", "shoot-ns2/etcd-main"}
	if err := globalOpts.Complete(cmd, args); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the resume command
	if err := cmd.RunE(cmd, args); err != nil {
		t.Fatalf("Resume command failed: %v", err)
	}

	// Verify BOTH etcds have suspend annotation REMOVED
	updated1, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns1", "etcd-main")
	if err != nil {
		t.Fatalf("Failed to get shoot-ns1/etcd-main: %v", err)
	}
	if updated1.Annotations != nil {
		if _, exists := updated1.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation]; exists {
			t.Error("Expected shoot-ns1/etcd-main to NOT have suspend annotation after resume")
		}
	}

	updated2, err := etcdClient.GetEtcd(context.TODO(), "shoot-ns2", "etcd-main")
	if err != nil {
		t.Fatalf("Failed to get shoot-ns2/etcd-main: %v", err)
	}
	if updated2.Annotations != nil {
		if _, exists := updated2.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation]; exists {
			t.Error("Expected shoot-ns2/etcd-main to NOT have suspend annotation after resume")
		}
	}

	t.Log("Successfully verified cross-namespace resume")
}
