// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations on spec updates for etcd.spec fields.
package etcd

import (
	"context"
	"testing"

	"github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/gomega"
)

// etcd.spec.storageClass is immutable
func TestValidateUpdateSpecStorageClass(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)

	tests := []struct {
		name                    string
		etcdName                string
		initialStorageClassName string
		updatedStorageClassName string
		expectErr               bool
	}{
		{
			name:                    "Valid #1: Unchanged storageClass",
			etcdName:                "etcd-valid-1",
			initialStorageClassName: "gardener.cloud-fast",
			updatedStorageClassName: "gardener.cloud-fast",
			expectErr:               false,
		},
		{
			name:                    "Invalid #1: Updated storageClass",
			etcdName:                "etcd-invalid-1",
			initialStorageClassName: "gardener.cloud-fast",
			updatedStorageClassName: "default",
			expectErr:               true,
		},
		{
			name:                    "Invalid #2: Set unset storageClass",
			etcdName:                "etcd-invalid-2",
			initialStorageClassName: "",
			updatedStorageClassName: "new-value",
			expectErr:               true,
		},
		{
			name:                    "Invalid #3: Unset set storageClass",
			etcdName:                "etcd-invalid-3",
			initialStorageClassName: "initial",
			updatedStorageClassName: "",
			expectErr:               true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			if test.initialStorageClassName != "" {
				etcd.Spec.StorageClass = &test.initialStorageClassName
			}
			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			if test.updatedStorageClassName != "" {
				etcd.Spec.StorageClass = &test.updatedStorageClassName
			} else {
				etcd.Spec.StorageClass = nil
			}
			validateEtcdUpdate(g, etcd, test.expectErr, ctx, cl)
		})
	}
}

// checks the update on the etcd.spec.replicas field
func TestValidateUpdateSpecReplicas(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	tests := []struct {
		name            string
		etcdName        string
		initialReplicas int
		updatedReplicas int
		expectErr       bool
	}{
		{
			name:            "Valid update to replicas #1",
			etcdName:        "etcd-valid-inc",
			initialReplicas: 3,
			updatedReplicas: 5,
			expectErr:       false,
		},
		{
			name:            "Valid update to replicas #2",
			etcdName:        "etcd-valid-zero",
			initialReplicas: 3,
			updatedReplicas: 0,
			expectErr:       false,
		},
		{
			name:            "Invalid update to replicas #1",
			etcdName:        "etcd-invalid-dec",
			initialReplicas: 5,
			updatedReplicas: 3,
			expectErr:       true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(int32(test.initialReplicas)).Build()
			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			etcd.Spec.Replicas = int32(test.updatedReplicas)
			validateEtcdUpdate(g, etcd, test.expectErr, ctx, cl)
		})
	}
}

// check the immutability of the etcd.spec.VolumeClaimTemplate field
func TestValidateUpdateSpecVolumeClaimTemplate(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	tests := []struct {
		name                string
		etcdName            string
		initalVolClaimTemp  string
		updatedVolClaimTemp string
		expectErr           bool
	}{
		{
			name:                "Valid #1: Unchanged volumeClaimTemplate",
			etcdName:            "etcd-valid-1-volclaim",
			initalVolClaimTemp:  "main-etcd",
			updatedVolClaimTemp: "main-etcd",
			expectErr:           false,
		},
		{
			name:                "Invalid #1: Updated volumeClaimTemplate",
			etcdName:            "etcd-invalid-1-volclaim",
			initalVolClaimTemp:  "main-etcd",
			updatedVolClaimTemp: "new-vol-temp",
			expectErr:           true,
		},
		{
			name:                "Invalid #2: Set unset volumeClaimTemplate",
			etcdName:            "etcd-invalid-2-volclaim",
			initalVolClaimTemp:  "",
			updatedVolClaimTemp: "New-Value",
			expectErr:           true,
		},
		{
			name:                "Invalid #2: Unset set volumeClaimTemplate",
			etcdName:            "etcd-invalid-3-volclaim",
			initalVolClaimTemp:  "Inital-vc",
			updatedVolClaimTemp: "",
			expectErr:           true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			if test.initalVolClaimTemp != "" {
				etcd.Spec.VolumeClaimTemplate = &test.initalVolClaimTemp
			}

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			if test.updatedVolClaimTemp != "" {
				etcd.Spec.VolumeClaimTemplate = &test.updatedVolClaimTemp
			} else {
				etcd.Spec.VolumeClaimTemplate = nil
			}
			validateEtcdUpdate(g, etcd, test.expectErr, ctx, cl)
		})
	}
}

// TestValidateUpdateSpecExternallyManagedMemberAddresses checks the dynamic addition or removal of the field.
func TestValidateUpdateSpecExternallyManagedMemberAddresses(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)

	tests := []struct {
		name                              string
		etcdName                          string
		initialReplicas                   int
		updatedReplicas                   int
		initialExternallyManagedAddresses []string
		updatedExternallyManagedAddresses []string
		expectErr                         bool
	}{
		{
			name:                              "Valid #1: Unchanged non-empty externallyManagedMemberAddresses",
			etcdName:                          "etcd-valid-1-externally-managed",
			initialReplicas:                   3,
			updatedReplicas:                   3,
			initialExternallyManagedAddresses: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"},
			updatedExternallyManagedAddresses: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"},
			expectErr:                         false,
		},
		{
			name:                              "Valid #2: Unchanged empty externallyManagedMemberAddresses",
			etcdName:                          "etcd-valid-2-externally-managed",
			initialReplicas:                   3,
			updatedReplicas:                   3,
			initialExternallyManagedAddresses: []string{},
			updatedExternallyManagedAddresses: []string{},
			expectErr:                         false,
		},
		{
			name:                              "Invalid #1: Introduced externallyManagedMemberAddresses when members are being managed by druid",
			etcdName:                          "etcd-invalid-1-externally-managed",
			initialReplicas:                   3,
			updatedReplicas:                   3,
			initialExternallyManagedAddresses: nil,
			updatedExternallyManagedAddresses: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"},
			expectErr:                         true,
		},
		{
			name:                              "Invalid #2: Removed externallyManagedMemberAddresses when members are being managed externally",
			etcdName:                          "etcd-invalid-2-externally-managed",
			initialReplicas:                   3,
			updatedReplicas:                   3,
			initialExternallyManagedAddresses: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"},
			updatedExternallyManagedAddresses: nil,
			expectErr:                         true,
		},
		{
			name:                              "Invalid #3: Scaled down to 0 replicas with externallyManagedMemberAddresses set",
			etcdName:                          "etcd-invalid-3-externally-managed",
			initialReplicas:                   3,
			updatedReplicas:                   0,
			initialExternallyManagedAddresses: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"},
			updatedExternallyManagedAddresses: []string{},
			expectErr:                         true,
		},
		{
			name:                              "Invalid #4: Scaled up from 0 replicas with externallyManagedMemberAddresses set",
			etcdName:                          "etcd-invalid-4-externally-managed",
			initialReplicas:                   0,
			updatedReplicas:                   3,
			initialExternallyManagedAddresses: []string{},
			updatedExternallyManagedAddresses: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"},
			expectErr:                         true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(int32(test.initialReplicas)).Build()
			etcd.Spec.ExternallyManagedMemberAddresses = test.initialExternallyManagedAddresses

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			etcd.Spec.Replicas = int32(test.updatedReplicas)
			etcd.Spec.ExternallyManagedMemberAddresses = test.updatedExternallyManagedAddresses
			validateEtcdUpdate(g, etcd, test.expectErr, ctx, cl)
		})
	}
}
