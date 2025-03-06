// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations on spec updates for etcd.spec fields.
package etcd

import (
	"context"
	"testing"

	"github.com/gardener/etcd-druid/test/utils"

	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/gomega"
)

// etcd.spec.storageClass is immutable
func TestValidateUpdateSpecStorageClass(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)

	tests := []struct {
		name                    string
		etcdName                string
		initalStorageClassName  string
		updatedStorageClassName string
		expectErr               bool
	}{
		{
			name:                    "Valid #1: Unchanged storageClass",
			etcdName:                "etcd-valid-1",
			initalStorageClassName:  "gardener.cloud-fast",
			updatedStorageClassName: "gardener.cloud-fast",
			expectErr:               false,
		},
		{
			name:                    "Invalid #1: Updated storageClass",
			etcdName:                "etcd-invalid-1",
			initalStorageClassName:  "gardener.cloud-fast",
			updatedStorageClassName: "default",
			expectErr:               true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.StorageClass = &test.initalStorageClassName

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			etcd.Spec.StorageClass = &test.updatedStorageClassName
			validateEtcdUpdation(t, g, etcd, test.expectErr, ctx, cl)
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
			name:            "Valid updation of replicas #1",
			etcdName:        "etcd-valid-inc",
			initialReplicas: 3,
			updatedReplicas: 5,
			expectErr:       false,
		},
		{
			name:            "Valid updation of replicas #2",
			etcdName:        "etcd-valid-zero",
			initialReplicas: 3,
			updatedReplicas: 0,
			expectErr:       false,
		},
		{
			name:            "Invalid updation of replicas #1",
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
			validateEtcdUpdation(t, g, etcd, test.expectErr, ctx, cl)
		})
	}
}

// checks the immutablility of the etcd.spec.StorageCapacity field
func TestValidateUpdateSpecStorageCapacity(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	tests := []struct {
		name                   string
		etcdName               string
		initalStorageCapacity  resource.Quantity
		updatedStorageCapacity resource.Quantity
		expectErr              bool
	}{
		{
			name:                   "Valid #1: Unchanged storageCapacity",
			etcdName:               "etcd-valid-1-storagecap",
			initalStorageCapacity:  resource.MustParse("25Gi"),
			updatedStorageCapacity: resource.MustParse("25Gi"),
			expectErr:              false,
		},
		{
			name:                   "Invalid #1: Updated storageCapacity",
			etcdName:               "etcd-invalid-1-storagecap",
			initalStorageCapacity:  resource.MustParse("15Gi"),
			updatedStorageCapacity: resource.MustParse("20Gi"),
			expectErr:              true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.StorageCapacity = &test.initalStorageCapacity

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			etcd.Spec.StorageCapacity = &test.updatedStorageCapacity
			validateEtcdUpdation(t, g, etcd, test.expectErr, ctx, cl)
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
			name:                "Invalid #1: Updated storageCapacity",
			etcdName:            "etcd-invalid-1-volclaim",
			initalVolClaimTemp:  "main-etcd",
			updatedVolClaimTemp: "new-vol-temp",
			expectErr:           true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.VolumeClaimTemplate = &test.initalVolClaimTemp

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			etcd.Spec.VolumeClaimTemplate = &test.updatedVolClaimTemp
			validateEtcdUpdation(t, g, etcd, test.expectErr, ctx, cl)
		})
	}
}
