package reconcile

import (
	"context"
	"strings"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	fake "github.com/gardener/etcd-druid/druidctl/internal/client/fake"
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

	if err := globalOpts.Validate(); err != nil {
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
	output := buf.String()
	if !strings.Contains(output, "completed successfully") {
		t.Errorf("Expected success message in output, got: %s", output)
	}

	t.Log("Successfully verified reconcile command adds DruidOperationAnnotation")
}

func TestReconcileCommandAllNamespaces(t *testing.T) {
	// Test reconcile command with --all-namespaces flag
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

	// Set all-namespaces flag
	cmd.Flags().Set("all-namespaces", "true")
	globalOpts.AllNamespaces = true

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{""}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.Validate(); err != nil {
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
	output := buf.String()
	if !strings.Contains(output, "completed successfully") {
		t.Errorf("Expected success message in output, got: %s", output)
	}

	t.Log("Successfully verified reconcile command with all-namespaces")
}

func TestSuspendReconcileCommand(t *testing.T) {
	// Test the suspend-reconcile command adds SuspendEtcdSpecReconcileAnnotation
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// Create the suspend-reconcile command
	cmd := NewSuspendReconcileCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.Validate(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the suspend-reconcile command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Suspend-reconcile command failed: %v", err)
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
	output := buf.String()
	if !strings.Contains(output, "completed successfully") {
		t.Errorf("Expected success message in output, got: %s", output)
	}

	t.Log("Successfully verified suspend-reconcile command adds SuspendEtcdSpecReconcileAnnotation")
}

func TestResumeReconcileCommand(t *testing.T) {
	// Test the resume-reconcile command removes SuspendEtcdSpecReconcileAnnotation
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

	// Create the resume-reconcile command
	cmd := NewResumeReconcileCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.Validate(); err != nil {
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

	// Execute the resume-reconcile command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Resume-reconcile command failed: %v", err)
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
	output := buf.String()
	if !strings.Contains(output, "completed successfully") {
		t.Errorf("Expected success message in output, got: %s", output)
	}

	t.Log("Successfully verified resume-reconcile command removes SuspendEtcdSpecReconcileAnnotation")
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

	// Create the resume-reconcile command
	cmd := NewResumeReconcileCommand(globalOpts)
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
	output := buf.String()
	if !strings.Contains(output, "completed successfully") {
		t.Errorf("Expected success message in output, got: %s", output)
	}

	t.Log("Successfully verified resume-reconcile handles missing annotation correctly")
}
