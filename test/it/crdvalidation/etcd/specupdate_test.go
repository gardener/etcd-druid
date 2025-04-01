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

// TestValidateUpdateSpecStorageClass tests the immutability of etcd.spec.storageClass
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
			initialStorageClassName: "storageClass1",
			updatedStorageClassName: "storageClass1",
			expectErr:               false,
		},
		{
			name:                    "Invalid #1: Updated storageClass",
			etcdName:                "etcd-invalid-1",
			initialStorageClassName: "storageClass1",
			updatedStorageClassName: "storageClass2",
			expectErr:               true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.StorageClass = &test.initialStorageClassName

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			etcd.Spec.StorageClass = &test.updatedStorageClassName
			validateEtcdUpdate(g, etcd, test.expectErr, ctx, cl)
		})
	}
}

// TestValidateUpdateSpecReplicas tests the update on the etcd.spec.replicas field
func TestValidateUpdateSpecReplicas(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testCases := []struct {
		name            string
		etcdName        string
		initialReplicas int32
		updatedReplicas int32
		clusterSize     int32
		expectErr       bool
	}{
		{
			// allow etcd cluster to be unhibernated
			name:            "0 -> n, where n > 0 and clusterSize = n",
			etcdName:        "etcd-valid-replicas-1",
			initialReplicas: 0,
			updatedReplicas: 3,
			clusterSize:     3,
			expectErr:       false,
		},
		{
			// etcd cluster can be unhibernated only to the size of the cluster, to allow scale-up logic to run
			name:            "0 -> n, where n > 0 and clusterSize > n",
			etcdName:        "etcd-invalid-replicas-1",
			initialReplicas: 0,
			updatedReplicas: 3,
			clusterSize:     5,
			expectErr:       true,
		},
		{
			// etcd cluster can be unhibernated only to the size of the cluster, because the cluster cannot be scale down
			name:            "0 -> m, where m > 0 and clusterSize < m",
			etcdName:        "etcd-invalid-replicas-2",
			initialReplicas: 0,
			updatedReplicas: 3,
			clusterSize:     5,
			expectErr:       true,
		},
		{
			// allow hibernated etcd cluster to continue being hibernated
			name:            "0 -> 0, where clusterSize = 0 (etcd cluster not started)",
			etcdName:        "etcd-valid-replicas-2",
			initialReplicas: 0,
			updatedReplicas: 0,
			clusterSize:     0,
			expectErr:       false,
		},
		{
			// allow hibernated etcd cluster to continue being hibernated
			name:            "0 -> 0, where clusterSize != 0 (etcd cluster already started)",
			etcdName:        "etcd-valid-replicas-3",
			initialReplicas: 0,
			updatedReplicas: 0,
			clusterSize:     1,
			expectErr:       false,
		},
		{
			// etcd replicas remains the same as cluster size
			name:            "n -> n, where n > 0",
			etcdName:        "etcd-valid-replicas-4",
			initialReplicas: 3,
			updatedReplicas: 3,
			clusterSize:     3,
			expectErr:       false,
		},
		{
			// etcd replicas remains the same, but does not match cluster size
			name:            "n -> n, where n > 0 and clusterSize != n",
			etcdName:        "etcd-valid-replicas-5",
			initialReplicas: 5,
			updatedReplicas: 5,
			clusterSize:     3,
			expectErr:       false,
		},
		{
			// scale up etcd cluster
			name:            "n -> m, where m > n > 0",
			etcdName:        "etcd-valid-replicas-6",
			initialReplicas: 3,
			updatedReplicas: 5,
			clusterSize:     3,
			expectErr:       false,
		},
		{
			// hibernate etcd cluster to 0 replicas
			name:            "m -> 0, where m > 0",
			etcdName:        "etcd-valid-replicas-7",
			initialReplicas: 3,
			updatedReplicas: 0,
			clusterSize:     3,
			expectErr:       false,
		},
		{
			// etcd cluster cannot be scaled down
			name:            "m -> n, where m > n > 0",
			etcdName:        "etcd-invalid-replicas-3",
			initialReplicas: 5,
			updatedReplicas: 3,
			clusterSize:     5,
			expectErr:       true,
		},
	}

	testNs, g := setupTestEnvironment(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(tc.etcdName, testNs).
				WithReplicas(tc.initialReplicas).
				Build()
			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())
			// update status subresource, since status is not created in the object Create() call
			etcd.Status.ClusterSize = tc.clusterSize
			g.Expect(cl.Status().Update(ctx, etcd)).To(Succeed())

			etcd.Spec.Replicas = tc.updatedReplicas
			validateEtcdUpdate(g, etcd, tc.expectErr, ctx, cl)
		})
	}
}

// TestValidateUpdateSpecVolumeClaimTemplate tests the immutability of the etcd.spec.VolumeClaimTemplate field
func TestValidateUpdateSpecVolumeClaimTemplate(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
	testNs, g := setupTestEnvironment(t)
	tests := []struct {
		name                string
		etcdName            string
		initialVolClaimTemp string
		updatedVolClaimTemp string
		expectErr           bool
	}{
		{
			name:                "Valid #1: Unchanged volumeClaimTemplate",
			etcdName:            "etcd-valid-1-volclaim",
			initialVolClaimTemp: "main-etcd",
			updatedVolClaimTemp: "main-etcd",
			expectErr:           false,
		},
		{
			name:                "Invalid #1: Updated storageCapacity",
			etcdName:            "etcd-invalid-1-volclaim",
			initialVolClaimTemp: "main-etcd",
			updatedVolClaimTemp: "new-vol-temp",
			expectErr:           true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.VolumeClaimTemplate = &test.initialVolClaimTemp

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			etcd.Spec.VolumeClaimTemplate = &test.updatedVolClaimTemp
			validateEtcdUpdate(g, etcd, test.expectErr, ctx, cl)
		})
	}
}
