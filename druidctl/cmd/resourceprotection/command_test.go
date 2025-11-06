// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package resourceprotection

import (
	"context"
	"strings"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	fake "github.com/gardener/etcd-druid/druidctl/internal/client/fake"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

func TestAddComponentProtectionCommand(t *testing.T) {
	// Test the add-component-protection command
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	etcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get test etcd: %v", err)
	}

	// Add the disable protection annotation
	if err := etcdClient.UpdateEtcd(context.TODO(), etcd, func(e *druidv1alpha1.Etcd) {
		if e.Annotations == nil {
			e.Annotations = make(map[string]string)
		}
		e.Annotations[druidv1alpha1.DisableEtcdComponentProtectionAnnotation] = ""
	}); err != nil {
		t.Fatalf("Failed to setup test etcd with annotation: %v", err)
	}

	// Create the command
	cmd := NewAddProtectionCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.Validate(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Command failed unexpectedly: %v", err)
	}

	// Verify the intended action, i.e. the disable protection annotation should be REMOVED
	updatedEtcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get updated etcd: %v", err)
	}

	// Disable protection annotation should be removed i.e protection enabled
	if updatedEtcd.Annotations != nil {
		if _, exists := updatedEtcd.Annotations[druidv1alpha1.DisableEtcdComponentProtectionAnnotation]; exists {
			t.Errorf("Expected disable protection annotation to be removed, but it still exists")
		}
	}

	// Verify command output
	output := buf.String()
	if !strings.Contains(output, "Component protection added successfully") {
		t.Errorf("Expected success message in output, got: %s", output)
	}

	t.Log("Successfully verified add-component-protection command removes disable annotation")
}

func TestAddProtectionWithoutAnnotation(t *testing.T) {
	// Test add protection when etcd doesn't have the annotation
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	etcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get test etcd: %v", err)
	}

	// Ensure no disable protection annotation exists initially
	if etcd.Annotations != nil {
		if _, exists := etcd.Annotations[druidv1alpha1.DisableEtcdComponentProtectionAnnotation]; exists {
			t.Fatal("Test setup error: etcd should not have disable protection annotation initially")
		}
	}

	// Create the add protection command
	cmd := NewAddProtectionCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.Validate(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Command failed unexpectedly: %v", err)
	}

	// Verify the intended action, i.e. since annotation didn't exist, state should remain unchanged
	updatedEtcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get updated etcd: %v", err)
	}

	// The disable protection annotation should still NOT exist
	if updatedEtcd.Annotations != nil {
		if _, exists := updatedEtcd.Annotations[druidv1alpha1.DisableEtcdComponentProtectionAnnotation]; exists {
			t.Errorf("Expected disable protection annotation to still not exist, but it does")
		}
	}

	// Verify command output
	output := buf.String()
	if !strings.Contains(output, "Component protection added successfully") {
		t.Errorf("Expected success message in output, got: %s", output)
	}

	t.Log("Successfully verified remove protection handles missing annotation correctly")
}

func TestRemoveComponentProtectionCommand(t *testing.T) {
	// Test the remove-component-protection command
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	_, err = etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get test etcd: %v", err)
	}

	// Create the command
	cmd := NewRemoveProtectionCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err = globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err = globalOpts.Validate(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command
	if err = cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Command failed unexpectedly: %v", err)
	}

	// Verify the intended action, i.e. the disable protection annotation should be ADDED
	updatedEtcd, err := etcdClient.GetEtcd(context.TODO(), "default", "test-etcd")
	if err != nil {
		t.Fatalf("Failed to get updated etcd: %v", err)
	}

	// The disable protection annotation should now exist, i.e protection disabled
	if updatedEtcd.Annotations == nil {
		t.Errorf("Expected annotations to exist after removing protection, but annotations is nil")
	} else {
		if _, exists := updatedEtcd.Annotations[druidv1alpha1.DisableEtcdComponentProtectionAnnotation]; !exists {
			t.Errorf("Expected disable protection annotation to be added, but it doesn't exist")
		}
	}

	// Verify command output
	output := buf.String()
	if !strings.Contains(output, "Component protection removed successfully") {
		t.Errorf("Expected success message in output, got: %s", output)
	}

	t.Log("Successfully verified remove-component-protection command adds disable annotation")
}

func TestResourceProtectionAllNamespaces(t *testing.T) {
	// Test resource protection commands with --all-namespaces flag
	helper := fake.NewTestHelper().WithTestScenario(fake.MultipleEtcdsScenario())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	// Get initial list of etcds and add disable protection annotation to all
	etcds, err := etcdClient.ListEtcds(context.TODO(), "")
	if err != nil {
		t.Fatalf("Failed to list etcds: %v", err)
	}

	// Add disable protection annotation to all etcds initially
	for _, etcd := range etcds.Items {
		err = etcdClient.UpdateEtcd(context.TODO(), &etcd, func(e *druidv1alpha1.Etcd) {
			if e.Annotations == nil {
				e.Annotations = make(map[string]string)
			}
			e.Annotations[druidv1alpha1.DisableEtcdComponentProtectionAnnotation] = ""
		})
		if err != nil {
			t.Fatalf("Failed to setup test etcd %s/%s with annotation: %v", etcd.Namespace, etcd.Name, err)
		}
	}

	// Create the add protection command
	cmd := NewAddProtectionCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Set the all-namespaces flag
	cmd.Flags().Set("all-namespaces", "true")
	globalOpts.AllNamespaces = true

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.Validate(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("Command failed unexpectedly: %v", err)
	}

	// Verify the intended action, i.e. disable protection annotation should be removed from all etcds
	updatedEtcds, err := etcdClient.ListEtcds(context.TODO(), "")
	if err != nil {
		t.Fatalf("Failed to list updated etcds: %v", err)
	}

	protectionEnabledCount := 0
	for _, etcd := range updatedEtcds.Items {
		if etcd.Annotations == nil {
			protectionEnabledCount++
		} else {
			if _, exists := etcd.Annotations[druidv1alpha1.DisableEtcdComponentProtectionAnnotation]; !exists {
				protectionEnabledCount++
			}
		}
	}

	if protectionEnabledCount != len(updatedEtcds.Items) {
		t.Errorf("Expected protection to be enabled for all %d etcds, but only %d have protection enabled", len(updatedEtcds.Items), protectionEnabledCount)
	}

	// Verify command output
	output := buf.String()
	if !strings.Contains(output, "Component protection added successfully") {
		t.Errorf("Expected success message in output, got: %s", output)
	}

	t.Logf("Successfully verified resource protection command with all-namespaces: %d etcd resources protected", protectionEnabledCount)
}

func TestResourceProtectionErrorHandling(t *testing.T) {
	// Test error cases with non-existent etcd
	helper := fake.NewTestHelper()
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	// Create the command
	cmd := NewAddProtectionCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"non-existent-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.Validate(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command, this should fail with non-existent resource error
	if err := cmd.RunE(cmd, []string{"non-existent-etcd"}); err == nil {
		t.Fatalf("Expected command to fail for non-existent etcd, but it succeeded")
	} else {
		if strings.Contains(err.Error(), "not found") {
			t.Logf("Command correctly failed with 'not found' error: %v", err)
		} else {
			t.Logf("Command failed with different error (still correct behavior): %v", err)
		}
	}

	t.Log("Successfully verified error handling for non-existent resource")
}
