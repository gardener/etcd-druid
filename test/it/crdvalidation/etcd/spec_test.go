// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations of etcd.spec fields.

package etcd

import (
	"testing"

	"github.com/gardener/etcd-druid/test/utils"
)

// TestSpecExternallyManagedMemberAddresses tests the validation of the Spec.ExternallyManagedMemberAddresses field.
func TestSpecExternallyManagedMemberAddresses(t *testing.T) {
	tests := []struct {
		name                           string
		etcdName                       string
		replicas                       int32
		externallyManagedMemberAddress []string
		expectErr                      bool
	}{
		{
			name:                           "Valid externallyManagedMemberAddresses #1: no addresses with non-zero replicas",
			etcdName:                       "etcd-valid-1",
			replicas:                       3,
			externallyManagedMemberAddress: []string{},
			expectErr:                      false,
		},
		{
			name:                           "Valid externallyManagedMemberAddresses #2: valid addresses",
			etcdName:                       "etcd-valid-2",
			replicas:                       3,
			externallyManagedMemberAddress: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"},
			expectErr:                      false,
		},
		{
			name:                           "Invalid externallyManagedMemberAddresses #1: mismatched number of addresses and replicas",
			etcdName:                       "etcd-invalid-1",
			replicas:                       3,
			externallyManagedMemberAddress: []string{"1.1.1.1", "1.1.1.2"},
			expectErr:                      true,
		},
		{
			name:                           "Invalid externallyManagedMemberAddresses #2: invalid address format",
			etcdName:                       "etcd-invalid-2",
			replicas:                       3,
			externallyManagedMemberAddress: []string{"http://1.1.1.1", "http://1.1.1.2"},
			expectErr:                      true,
		},
		{
			name:                           "Invalid externallyManagedMemberAddresses #3: non-unique addresses",
			etcdName:                       "etcd-invalid-3",
			replicas:                       3,
			externallyManagedMemberAddress: []string{"1.1.1.1", "1.1.1.1", "1.1.1.1"},
			expectErr:                      true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(test.replicas).Build()
			etcd.Spec.ExternallyManagedMemberAddresses = test.externallyManagedMemberAddress

			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}
