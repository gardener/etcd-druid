// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations of etcd.spec fields.

package etcd

import (
	"fmt"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSpecExternallyManagedMemberAddresses tests validation of the new and deprecated member address fields.
func TestSpecExternallyManagedMemberAddresses(t *testing.T) {
	tests := []struct {
		name                           string
		etcdName                       string
		replicas                       int32
		externallyManagedMemberAddress []string
		expectErr                      bool
	}{
		{
			name:                           "Valid externallyManagedMemberAddresses #1: druid-managed setup - no addresses with non-zero replicas",
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
			name:                           "Invalid externallyManagedMemberAddresses #2: non-unique addresses",
			etcdName:                       "etcd-invalid-2",
			replicas:                       3,
			externallyManagedMemberAddress: []string{"1.1.1.1", "1.1.1.1", "1.1.1.1"},
			expectErr:                      true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, deprecated := range []bool{false, true} {
				t.Run(fmt.Sprintf("deprecated=%t", deprecated), func(t *testing.T) {
					etcd := utils.EtcdBuilderWithoutDefaults(fmt.Sprintf("%s-%t", test.etcdName, deprecated), testNs).
						WithReplicas(test.replicas).
						WithExternallyManagedMembers(test.externallyManagedMemberAddress).
						Build()
					if deprecated {
						etcd.Spec.Etcd.ExternallyManagedMembers = nil
						etcd.Spec.ExternallyManagedMemberAddresses = test.externallyManagedMemberAddress
					}
					validateEtcdCreation(g, etcd, test.expectErr)
				})
			}
		})
	}
}

func TestSpecExternallyManagedMembersEndpointsDiscovery(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	tests := []struct {
		name      string
		config    druidv1alpha1.EndpointsDiscoveryConfig
		expectErr bool
	}{
		{name: "omitted"},
		{name: "refresh-disabled", config: druidv1alpha1.EndpointsDiscoveryConfig{EndpointsFilePath: "/var/lib/etcd/endpoints"}},
		{name: "refresh-enabled", config: druidv1alpha1.EndpointsDiscoveryConfig{EndpointsFilePath: "/var/lib/etcd/endpoints", RefreshEnabled: true, RefreshInterval: &metav1.Duration{Duration: 10 * time.Second}}},
		{name: "default-interval", config: druidv1alpha1.EndpointsDiscoveryConfig{EndpointsFilePath: "/var/lib/etcd/endpoints", RefreshEnabled: true}},
		{name: "zero-interval", config: druidv1alpha1.EndpointsDiscoveryConfig{EndpointsFilePath: "/var/lib/etcd/endpoints", RefreshEnabled: true, RefreshInterval: &metav1.Duration{}}, expectErr: true},
		{name: "missing-file", config: druidv1alpha1.EndpointsDiscoveryConfig{RefreshEnabled: true, RefreshInterval: &metav1.Duration{Duration: 10 * time.Second}}, expectErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults("etcd-discovery-"+test.name, testNs).
				WithReplicas(1).
				WithExternallyManagedMembers([]string{"1.1.1.1"}).
				WithEndpointsDiscovery(test.config).Build()
			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}
