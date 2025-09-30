// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations on spec updates for etcd.spec fields.
package etcd

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"

	"github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/gomega"
)

var (
	defaultStorageCapacity = apiresource.MustParse("16Gi")
	updatedStorageCapacity = apiresource.MustParse("8Gi")
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
		name                        string
		etcdName                    string
		initalVolClaimTemp          string
		updatedVolClaimTemp         string
		initialEmptyDirVolumeSource *corev1.EmptyDirVolumeSource
		updatedEmptyDirVolumeSource *corev1.EmptyDirVolumeSource
		expectErr                   bool
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
			updatedVolClaimTemp: "main-etcd-1",
			expectErr:           true,
		},
		{
			name:                        "Valid #2: Remove volumeClaimTemplate and add emptyDirVolumeSource",
			etcdName:                    "etcd-valid-2-volclaim-emptydir",
			initalVolClaimTemp:          "main-etcd",
			updatedVolClaimTemp:         "",
			initialEmptyDirVolumeSource: nil,
			updatedEmptyDirVolumeSource: &corev1.EmptyDirVolumeSource{
				SizeLimit: &defaultStorageCapacity,
			},
			expectErr: false,
		},
		{
			name:                        "Invalid #2: Keep volumeClaimTemplate and add emptyDirVolumeSource",
			etcdName:                    "etcd-invalid-2-volclaim-emptydir",
			initalVolClaimTemp:          "main-etcd",
			updatedVolClaimTemp:         "main-etcd",
			initialEmptyDirVolumeSource: nil,
			updatedEmptyDirVolumeSource: &corev1.EmptyDirVolumeSource{
				SizeLimit: &defaultStorageCapacity,
			},
			expectErr: true,
		},
		{
			name:     "Valid #3: Unchanged emptyDirVolumeSource",
			etcdName: "etcd-valid-3-emptydir",
			initialEmptyDirVolumeSource: &corev1.EmptyDirVolumeSource{
				SizeLimit: &defaultStorageCapacity,
			},
			updatedEmptyDirVolumeSource: &corev1.EmptyDirVolumeSource{
				SizeLimit: &defaultStorageCapacity,
			},
			expectErr: false,
		},
		{
			name:     "Invalid #4: Updated emptyDirVolumeSource",
			etcdName: "etcd-invalid-4-emptydir",
			initialEmptyDirVolumeSource: &corev1.EmptyDirVolumeSource{
				SizeLimit: &defaultStorageCapacity,
			},
			updatedEmptyDirVolumeSource: &corev1.EmptyDirVolumeSource{
				SizeLimit: &updatedStorageCapacity,
			},
			expectErr: true,
		},
		{
			name:                "Invalid #5: Remove emptyDirVolumeSource and add volumeClaimTemplate",
			etcdName:            "etcd-invalid-5-emptydir-volclaim",
			updatedVolClaimTemp: "main-etcd",
			initialEmptyDirVolumeSource: &corev1.EmptyDirVolumeSource{
				SizeLimit: &defaultStorageCapacity,
			},
			updatedEmptyDirVolumeSource: nil,
			expectErr:                   true,
		},
		{
			name:     "Invalid #6: Remove emptyDirVolumeSource",
			etcdName: "etcd-invalid-6-emptydir-volclaim",
			initialEmptyDirVolumeSource: &corev1.EmptyDirVolumeSource{
				SizeLimit: &defaultStorageCapacity,
			},
			updatedEmptyDirVolumeSource: nil,
			expectErr:                   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			if test.initalVolClaimTemp != "" {
				etcd.Spec.VolumeClaimTemplate = &test.initalVolClaimTemp
			}
			etcd.Spec.EmptyDirVolumeSource = test.initialEmptyDirVolumeSource

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			if test.updatedVolClaimTemp != "" {
				etcd.Spec.VolumeClaimTemplate = &test.updatedVolClaimTemp
			} else {
				etcd.Spec.VolumeClaimTemplate = nil
			}
			etcd.Spec.EmptyDirVolumeSource = test.updatedEmptyDirVolumeSource
			validateEtcdUpdate(g, etcd, test.expectErr, ctx, cl)
		})
	}
}
