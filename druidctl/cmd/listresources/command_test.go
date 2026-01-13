// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package listresources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	fake "github.com/gardener/etcd-druid/druidctl/internal/client/fake"

	"k8s.io/cli-runtime/pkg/genericiooptions"
)

// extractJSON extracts the JSON portion from output that may contain INFO messages
func extractJSON(output string) string {
	// Find the JSON object (starts with { and ends with })
	start := strings.Index(output, "{")
	if start == -1 {
		return output
	}
	// Find matching closing brace
	depth := 0
	for i := start; i < len(output); i++ {
		switch output[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return output[start : i+1]
			}
		}
	}
	return output[start:]
}

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

	// Use JSON output for programmatic validation
	if err := cmd.Flags().Set("output", "json"); err != nil {
		t.Fatalf("Failed to set output flag: %v", err)
	}

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Parse JSON output
	var result Result
	jsonStr := extractJSON(buf.String())
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, buf.String())
	}

	// Assertions
	if len(result.Etcds) != 1 {
		t.Errorf("Expected 1 etcd in result, got %d", len(result.Etcds))
	}

	if result.Etcds[0].Etcd.Name != "test-etcd" {
		t.Errorf("Expected etcd name 'test-etcd', got '%s'", result.Etcds[0].Etcd.Name)
	}

	if result.Etcds[0].Etcd.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", result.Etcds[0].Etcd.Namespace)
	}

	// Verify expected resources exist (from SingleEtcdWithResources: 3 pods, 2 services)
	podCount := 0
	serviceCount := 0
	for _, item := range result.Etcds[0].Items {
		if item.Key.Kind == "Pod" {
			podCount = len(item.Resources)
		}
		if item.Key.Kind == "Service" {
			serviceCount = len(item.Resources)
		}
	}

	if podCount != 3 {
		t.Errorf("Expected 3 pods, got %d", podCount)
	}
	if serviceCount != 2 {
		t.Errorf("Expected 2 services, got %d", serviceCount)
	}

	if len(errBuf.String()) > 0 {
		t.Logf("Command error output: %s", errBuf.String())
	}

	t.Log("Successfully verified list-resources returns expected resources")
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

	etcds, err := etcdClient.ListEtcds(context.TODO(), "", "")
	if err != nil {
		t.Fatalf("Failed to list etcds: %v", err)
	}

	expectedCount := len(etcds.Items)
	if expectedCount != helper.EtcdObjectCount() {
		t.Errorf("Expected %d etcd resources from MultipleEtcdsScenario, got %d", helper.EtcdObjectCount(), expectedCount)
	}

	// Create the command
	cmd := NewListResourcesCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Use JSON output for validation
	if err := cmd.Flags().Set("output", "json"); err != nil {
		t.Fatalf("Failed to set output flag: %v", err)
	}

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

	// Execute the command
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Parse JSON output
	var result Result
	jsonStr := extractJSON(buf.String())
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, buf.String())
	}

	// MultipleEtcdsScenario has 3 etcds across 2 namespaces
	if len(result.Etcds) != 3 {
		t.Errorf("Expected 3 etcds with -A, got %d", len(result.Etcds))
	}

	// Verify namespaces are different (cross-namespace listing)
	namespaces := make(map[string]bool)
	for _, etcdResult := range result.Etcds {
		namespaces[etcdResult.Etcd.Namespace] = true
	}
	if len(namespaces) < 2 {
		t.Errorf("Expected resources from multiple namespaces, got namespaces: %v", namespaces)
	}

	t.Logf("Successfully verified list-resources -A returns %d etcds from %d namespaces", len(result.Etcds), len(namespaces))
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

	// Set filter flag to specific resource types like pods
	if err := cmd.Flags().Set("filter", "pods"); err != nil {
		t.Fatalf("Failed to set filter flag: %v", err)
	}

	// Use JSON output for validation
	if err := cmd.Flags().Set("output", "json"); err != nil {
		t.Fatalf("Failed to set output flag: %v", err)
	}

	// Complete and validate options
	if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command
	if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Parse JSON output
	var result Result
	jsonStr := extractJSON(buf.String())
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, buf.String())
	}

	// With filter=pods, should only have Pod resources, not Services
	for _, item := range result.Etcds[0].Items {
		if item.Key.Kind == "Service" {
			t.Errorf("Expected filter to exclude Services, but found Service resources")
		}
	}

	// Should have pods
	hasPods := false
	for _, item := range result.Etcds[0].Items {
		if item.Key.Kind == "Pod" {
			hasPods = true
			break
		}
	}
	if !hasPods {
		t.Error("Expected Pod resources with filter=pods")
	}

	if len(errBuf.String()) > 0 {
		t.Logf("Command error output: %s", errBuf.String())
	}

	t.Log("Successfully verified list-resources filter works correctly")
}

func TestListResourcesOutputFormats(t *testing.T) {
	// Test different output formats
	helper := fake.NewTestHelper().WithTestScenario(fake.SingleEtcdWithResources())

	tests := []struct {
		name           string
		outputFormat   string
		expectContains string
	}{
		{"json_output", "json", `"kind": "EtcdResourceList"`},
		{"yaml_output", "yaml", "kind: EtcdResourceList"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalOpts := helper.CreateTestOptions()

			// Create test IO streams to capture output
			streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
			globalOpts.IOStreams = streams

			// Create the command
			cmd := NewListResourcesCommand(globalOpts)
			cmd.SetOut(buf)
			cmd.SetErr(errBuf)

			// Set output format
			if err := cmd.Flags().Set("output", tt.outputFormat); err != nil {
				t.Fatalf("Failed to set output flag: %v", err)
			}

			// Complete and validate options
			if err := globalOpts.Complete(cmd, []string{"test-etcd"}); err != nil {
				t.Fatalf("Failed to complete options: %v", err)
			}

			if err := globalOpts.ValidateResourceSelection(); err != nil {
				t.Fatalf("Failed to validate options: %v", err)
			}

			// Execute the command
			if err := cmd.RunE(cmd, []string{"test-etcd"}); err != nil {
				t.Fatalf("Command failed: %v", err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.expectContains) {
				t.Errorf("Expected output to contain '%s', got: %s", tt.expectContains, output)
			}

			t.Logf("Successfully tested %s output format", tt.outputFormat)
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

	etcds, err := etcdClient.ListEtcds(context.TODO(), "", "")
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

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Command should fail for non-existent etcd
	if err := cmd.RunE(cmd, []string{"non-existent-etcd"}); err == nil {
		t.Error("Expected command to fail for non-existent etcd, but it succeeded")
	} else {
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("Expected 'not found' error, got: %v", err)
		}
		t.Logf("Command correctly failed with: %v", err)
	}

	t.Log("Successfully verified error handling for non-existent etcd")
}

func TestListResourcesNamespaceFlag(t *testing.T) {
	// Test -n namespace flag with resource name (kubectl pattern)
	helper := fake.NewTestHelper().WithTestScenario(fake.MultipleEtcdsScenario())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	// Create the command
	cmd := NewListResourcesCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Set namespace to shoot-ns1
	shootNs1 := "shoot-ns1"
	globalOpts.ConfigFlags.Namespace = &shootNs1

	// Use JSON output
	if err := cmd.Flags().Set("output", "json"); err != nil {
		t.Fatalf("Failed to set output flag: %v", err)
	}

	// Complete and validate options - request etcd-main in shoot-ns1
	if err := globalOpts.Complete(cmd, []string{"etcd-main"}); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command
	if err := cmd.RunE(cmd, []string{"etcd-main"}); err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Parse JSON output
	var result Result
	jsonStr := extractJSON(buf.String())
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	// Should get exactly 1 etcd from shoot-ns1
	if len(result.Etcds) != 1 {
		t.Errorf("Expected 1 etcd, got %d", len(result.Etcds))
	}

	if result.Etcds[0].Etcd.Namespace != "shoot-ns1" {
		t.Errorf("Expected namespace 'shoot-ns1', got '%s'", result.Etcds[0].Etcd.Namespace)
	}

	if result.Etcds[0].Etcd.Name != "etcd-main" {
		t.Errorf("Expected name 'etcd-main', got '%s'", result.Etcds[0].Etcd.Name)
	}

	t.Log("Successfully verified -n namespace flag with resource name")
}

func TestListResourcesCrossNamespace(t *testing.T) {
	// Test cross-namespace selection: ns1/etcd1 ns2/etcd2
	helper := fake.NewTestHelper().WithTestScenario(fake.MultipleEtcdsScenario())
	globalOpts := helper.CreateTestOptions()

	// Create test IO streams to capture output
	streams, _, buf, errBuf := genericiooptions.NewTestIOStreams()
	globalOpts.IOStreams = streams

	// Create the command
	cmd := NewListResourcesCommand(globalOpts)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)

	// Use JSON output
	if err := cmd.Flags().Set("output", "json"); err != nil {
		t.Fatalf("Failed to set output flag: %v", err)
	}

	// IMPORTANT: Clear namespace flag for cross-namespace references
	emptyNs := ""
	globalOpts.ConfigFlags.Namespace = &emptyNs

	// Request resources from different namespaces using ns/name format
	args := []string{"shoot-ns1/etcd-main", "shoot-ns2/etcd-main"}
	if err := globalOpts.Complete(cmd, args); err != nil {
		t.Fatalf("Failed to complete options: %v", err)
	}

	if err := globalOpts.ValidateResourceSelection(); err != nil {
		t.Fatalf("Failed to validate options: %v", err)
	}

	// Execute the command
	if err := cmd.RunE(cmd, args); err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Parse JSON output
	var result Result
	jsonStr := extractJSON(buf.String())
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	// Should get 2 etcds from different namespaces
	if len(result.Etcds) != 2 {
		t.Errorf("Expected 2 etcds with cross-namespace selection, got %d", len(result.Etcds))
	}

	// Verify both namespaces are present
	namespaces := make(map[string]bool)
	for _, etcdResult := range result.Etcds {
		namespaces[etcdResult.Etcd.Namespace] = true
	}
	if !namespaces["shoot-ns1"] || !namespaces["shoot-ns2"] {
		t.Errorf("Expected both shoot-ns1 and shoot-ns2 in results, got: %v", namespaces)
	}

	t.Log("Successfully verified cross-namespace selection (ns/name format)")
}
