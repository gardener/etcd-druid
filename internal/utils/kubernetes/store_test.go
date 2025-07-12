// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

func TestMountPathLocalStore(t *testing.T) {
	newEtcdBuilder := func() *testutils.EtcdBuilder {
		return testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace)
	}
	testCases := []struct {
		name           string
		etcd           *druidv1alpha1.Etcd
		provider       *string
		expectedResult string
	}{
		{
			name:           "should return empty string if backup store is not enabled",
			etcd:           newEtcdBuilder().Build(),
			expectedResult: "",
		},
		{
			name:           "should return empty string if provider is nil",
			etcd:           newEtcdBuilder().WithProviderLocal("").Build(),
			expectedResult: "",
		},
		{
			name:           "should return empty string if provider is not 'Local'",
			etcd:           newEtcdBuilder().WithProviderS3("").Build(),
			provider:       ptr.To("S3"),
			expectedResult: "",
		},
		{
			name:           "should return non-root homedir with path if container is set",
			etcd:           newEtcdBuilder().WithProviderLocal("").Build(),
			provider:       ptr.To("Local"),
			expectedResult: "/home/nonroot/default.bkp",
		},
		{
			name:           "should return root homedir with path if container is set",
			etcd:           newEtcdBuilder().WithProviderLocal("").WithRunAsRoot(ptr.To(true)).Build(),
			provider:       ptr.To("Local"),
			expectedResult: "/root/default.bkp",
		},
		{
			name: "should return non-root homedir without path if container is nil",
			etcd: func() *druidv1alpha1.Etcd {
				etcd := newEtcdBuilder().WithProviderLocal("").Build()
				etcd.Spec.Backup.Store.Container = nil
				return etcd
			}(),
			provider:       ptr.To("Local"),
			expectedResult: "/home/nonroot",
		},
		{
			name: "should return root homedir without path if container is nil",
			etcd: func() *druidv1alpha1.Etcd {
				etcd := newEtcdBuilder().WithProviderLocal("").WithRunAsRoot(ptr.To(true)).Build()
				etcd.Spec.Backup.Store.Container = nil
				return etcd
			}(),
			provider:       ptr.To("Local"),
			expectedResult: "/root",
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g.Expect(MountPathLocalStore(tc.etcd, tc.provider)).To(Equal(tc.expectedResult))
		})
	}
}
