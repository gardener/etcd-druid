// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestValidateResourceSelection(t *testing.T) {
	tests := []struct {
		name          string
		resourceArgs  []string
		namespace     *string
		allNamespaces bool
		expectErr     bool
	}{
		{
			name:          "no flags, no args is valid",
			resourceArgs:  []string{},
			namespace:     nil,
			allNamespaces: false,
			expectErr:     false,
		},
		{
			name:          "simple args without -n is valid",
			resourceArgs:  []string{"etcd-main"},
			namespace:     nil,
			allNamespaces: false,
			expectErr:     false,
		},
		{
			name:          "simple args with -n is valid",
			resourceArgs:  []string{"etcd-main", "etcd-events"},
			namespace:     strPtr("shoot-ns1"),
			allNamespaces: false,
			expectErr:     false,
		},
		{
			name:          "cross-namespace args without -n is valid",
			resourceArgs:  []string{"shoot-ns1/etcd-main", "shoot-ns2/etcd-events"},
			namespace:     nil,
			allNamespaces: false,
			expectErr:     false,
		},
		{
			name:          "-A alone is valid",
			resourceArgs:  []string{},
			namespace:     nil,
			allNamespaces: true,
			expectErr:     false,
		},
		{
			name:          "-A with resource args is invalid",
			resourceArgs:  []string{"etcd-main"},
			namespace:     nil,
			allNamespaces: true,
			expectErr:     true,
		},
		{
			name:          "-A with cross-namespace args is invalid",
			resourceArgs:  []string{"shoot-ns1/etcd-main"},
			namespace:     nil,
			allNamespaces: true,
			expectErr:     true,
		},
		{
			name:          "-A with -n is invalid",
			resourceArgs:  []string{},
			namespace:     strPtr("shoot-ns1"),
			allNamespaces: true,
			expectErr:     true,
		},
		{
			name:          "cross-namespace args with -n is invalid",
			resourceArgs:  []string{"shoot-ns2/etcd-events"},
			namespace:     strPtr("shoot-ns1"),
			allNamespaces: false,
			expectErr:     true,
		},
		{
			name:          "mixed args (simple + cross-namespace) with -n is invalid",
			resourceArgs:  []string{"etcd-main", "shoot-ns2/etcd-events"},
			namespace:     strPtr("shoot-ns1"),
			allNamespaces: false,
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &GlobalOptions{
				ResourceArgs:  tt.resourceArgs,
				AllNamespaces: tt.allNamespaces,
				ConfigFlags:   &genericclioptions.ConfigFlags{Namespace: tt.namespace},
			}

			err := opts.ValidateResourceSelection()

			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestBuildEtcdRefList(t *testing.T) {
	tests := []struct {
		name         string
		resourceArgs []string
		namespace    *string // nil means not set, empty string means explicitly empty
		expected     []types.NamespacedName
	}{
		{
			name:         "empty resource args returns nil",
			resourceArgs: []string{},
			namespace:    nil,
			expected:     nil,
		},
		{
			name:         "simple name uses default namespace when -n not set",
			resourceArgs: []string{"etcd-main"},
			namespace:    nil,
			expected: []types.NamespacedName{
				{Namespace: "default", Name: "etcd-main"},
			},
		},
		{
			name:         "simple name uses -n namespace when set",
			resourceArgs: []string{"etcd-main"},
			namespace:    strPtr("shoot-ns1"),
			expected: []types.NamespacedName{
				{Namespace: "shoot-ns1", Name: "etcd-main"},
			},
		},
		{
			name:         "cross-namespace format ns/name uses explicit namespace",
			resourceArgs: []string{"shoot-ns2/etcd-events"},
			namespace:    nil,
			expected: []types.NamespacedName{
				{Namespace: "shoot-ns2", Name: "etcd-events"},
			},
		},
		{
			name:         "multiple simple names use same namespace",
			resourceArgs: []string{"etcd-main", "etcd-events"},
			namespace:    strPtr("shoot-ns1"),
			expected: []types.NamespacedName{
				{Namespace: "shoot-ns1", Name: "etcd-main"},
				{Namespace: "shoot-ns1", Name: "etcd-events"},
			},
		},
		{
			name:         "multiple cross-namespace args",
			resourceArgs: []string{"shoot-ns1/etcd-main", "shoot-ns2/etcd-events"},
			namespace:    nil,
			expected: []types.NamespacedName{
				{Namespace: "shoot-ns1", Name: "etcd-main"},
				{Namespace: "shoot-ns2", Name: "etcd-events"},
			},
		},
		{
			name:         "whitespace-only args are skipped",
			resourceArgs: []string{"etcd-main", "   ", "etcd-events"},
			namespace:    strPtr("shoot-ns1"),
			expected: []types.NamespacedName{
				{Namespace: "shoot-ns1", Name: "etcd-main"},
				{Namespace: "shoot-ns1", Name: "etcd-events"},
			},
		},
		{
			name:         "args with leading/trailing whitespace are trimmed",
			resourceArgs: []string{"  etcd-main  ", "  shoot-ns2/etcd-events  "},
			namespace:    nil,
			expected: []types.NamespacedName{
				{Namespace: "default", Name: "etcd-main"},
				{Namespace: "shoot-ns2", Name: "etcd-events"},
			},
		},
		{
			name:         "empty namespace flag uses default",
			resourceArgs: []string{"etcd-main"},
			namespace:    strPtr(""),
			expected: []types.NamespacedName{
				{Namespace: "default", Name: "etcd-main"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &GlobalOptions{
				ResourceArgs: tt.resourceArgs,
				ConfigFlags:  &genericclioptions.ConfigFlags{Namespace: tt.namespace},
			}

			result := opts.BuildEtcdRefList()

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d refs, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("ref[%d]: expected %v, got %v", i, expected, result[i])
				}
			}
		})
	}
}

// strPtr is a helper to create a pointer to a string
func strPtr(s string) *string {
	return &s
}
