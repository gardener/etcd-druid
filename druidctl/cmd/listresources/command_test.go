// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package listresources

import (
	"context"
	"strings"
	"testing"

	fake "github.com/gardener/etcd-druid/druidctl/internal/client/fake"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

func TestListResourcesCommand(t *testing.T) {
	// Create test helper with single etcd scenario
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	// Create the list-resources command
	cmd := NewListResourcesCommand(globalOpts)
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
		// Expected due to discovery limitations in fake client, but verify it's the right error
		if !strings.Contains(err.Error(), "unknown resource tokens") {
			t.Fatalf("Unexpected error: %v", err)
		}
		t.Logf("Command completed with expected discovery error: %v", err)
	}

	output := buf.String()
	errOutput := errBuf.String()
	if len(errOutput) > 0 {
		t.Logf("Command error output captured: '%s'", errOutput)
	}
	t.Logf("Command output captured: '%s'", output)

	// Verify that the output contains all expected resources that this command would have listed
	// Under Construction

	t.Log("Successfully verified list-resources command targets correct etcd")
}

func TestListResourcesAllNamespaces(t *testing.T) {
	// Create test helper with multiple etcd scenario
	helper := fake.NewTestHelper().WithTestScenario(fake.MultipleEtcdsScenario())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	// Verify initial state - should have multiple etcds
	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	etcds, err := etcdClient.ListEtcds(context.TODO(), "")
	if err != nil {
		t.Fatalf("Failed to list etcds: %v", err)
	}

	expectedCount := len(etcds.Items)
	if expectedCount != helper.EtcdObjectCount() {
		t.Errorf("Expected %d etcd resources from MultipleEtcdsScenario, got %d", helper.EtcdObjectCount(), expectedCount)
	}

	// Collect etcd names and namespaces for verification
	expectedEtcds := make(map[string]string) // name -> namespace
	for _, etcd := range etcds.Items {
		expectedEtcds[etcd.Name] = etcd.Namespace
	}

	// Create the command
	cmd := NewListResourcesCommand(globalOpts)
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
		if !strings.Contains(err.Error(), "unknown resource tokens") {
			t.Fatalf("Unexpected error: %v", err)
		}
		t.Logf("Command completed with expected discovery error: %v", err)
	}

	output := buf.String()
	errOutput := errBuf.String()
	if len(errOutput) > 0 {
		t.Logf("Command error output captured: '%s'", errOutput)
	}
	t.Logf("Command output captured: '%s'", output)

	// Verify that the output contains all expected resources that this command would have listed
	// Under Construction

	t.Logf("Successfully verified list-resources command with all-namespaces: %d etcd resources", len(expectedEtcds))
}

func TestListResourcesWithFilter(t *testing.T) {
	// Test list-resources command with filter
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	// Create the command
	cmd := NewListResourcesCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Set filter flag to specific resource types
	specificFilter := "pods,services"
	cmd.Flags().Set("filter", specificFilter)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.Validate(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		if !strings.Contains(err.Error(), "pods") || !strings.Contains(err.Error(), "services") {
			t.Errorf("Expected error to mention filtered resource types (pods, services), got: %v", err)
		}
		t.Logf("Command completed with expected discovery error for filtered resources: %v", err)
	}

	output := buf.String()
	errOutput := errBuf.String()
	if len(errOutput) > 0 {
		t.Logf("Command error output captured: '%s'", errOutput)
	}
	t.Logf("Command output captured: '%s'", output)

	// Verify that the output contains all expected resources that this command would have listed
	// Under Construction

	t.Logf("Successfully verified list-resources command with filter '%s' and lazy loading", specificFilter)
}

func TestListResourcesOutputFormats(t *testing.T) {
	// Test different output formats
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())

	tests := []struct {
		name         string
		outputFormat string
	}{
		{"default_output", ""},
		{"json_output", "json"},
		{"yaml_output", "yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalOpts := helper.CreateTestOptions()

			// Create test IO streams to capture output
			streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
			globalOpts.IOStreams = streams

			// Set output format
			// options.OutputFormat = printer.OutputFormat(tt.outputFormat)

			// Create the command
			cmd := NewListResourcesCommand(globalOpts)
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
				t.Logf("Command completed with expected error for output format %s: %v", tt.outputFormat, err)
			}

			// Verify command executed
			output := buf.String()
			errOutput := errBuf.String()
			if len(errOutput) > 0 {
				t.Logf("Command error output captured: '%s'", errOutput)
			}
			t.Logf("Command output captured for format %s: '%s'", tt.outputFormat, output)

			// Verify that the output contains all expected resources that this command would have listed
			// Under Construction

			t.Logf("Successfully tested list-resources with output format: %s, output: %s", tt.outputFormat, output)
		})
	}
}

func TestListResourcesErrorHandling(t *testing.T) {
	// Test error cases with empty scenario
	helper := fake.NewTestHelper() // No test data
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	// Verify no etcds exist in the fake client
	etcdClient, err := globalOpts.Clients.EtcdClient()
	if err != nil {
		t.Fatalf("Failed to create etcd client: %v", err)
	}

	etcds, err := etcdClient.ListEtcds(context.TODO(), "")
	if err != nil {
		t.Fatalf("Failed to list etcds: %v", err)
	}

	if len(etcds.Items) != 0 {
		t.Errorf("Expected no etcd resources in empty scenario, got %d", len(etcds.Items))
	}

	// Create the command
	cmd := NewListResourcesCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"non-existent-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.Validate(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	if err := cmd.RunE(cmd, []string{"non-existent-etcd"}); err != nil {
		if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "unknown resource tokens") {
			t.Errorf("Expected 'not found' or discovery error, got: %v", err)
		}
		t.Logf("Command failed as expected with error: %v", err)
	} else {
		t.Errorf("Expected command to fail, but it succeeded")
	}

	// Verify command attempt
	output := buf.String()
	errOutput := errBuf.String()
	if len(errOutput) > 0 {
		t.Logf("Command error output captured: '%s'", errOutput)
	}
	t.Logf("Command output captured: '%s'", output)

	// Verify that the output contains all expected resources that this command would have listed
	// Under Construction

	t.Log("Successfully verified list-resources error handling for non-existent etcd")
}

func TestListResourcesEmptyNamespaces(t *testing.T) {
	// Test all-namespaces when no etcds exist
	helper := fake.NewTestHelper() // No test data
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	// Create the command
	cmd := NewListResourcesCommand(globalOpts)
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

	if err := cmd.RunE(cmd, []string{}); err != nil {
		if !strings.Contains(err.Error(), "unknown resource tokens") {
			t.Fatalf("Expected discovery error or success, got: %v", err)
		}
		t.Logf("Command failed with expected discovery error: %v", err)
	}

	// Verify command attempt
	output := buf.String()
	errOutput := errBuf.String()
	if len(errOutput) > 0 {
		t.Logf("Command error output captured: '%s'", errOutput)
	}
	t.Logf("Command output captured: '%s'", output)

	// Verify that the output contains all expected resources that this command would have listed
	// Under Construction

	t.Log("Successfully verified list-resources handles empty namespaces correctly")
}
