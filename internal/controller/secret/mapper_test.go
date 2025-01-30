// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"testing"

	testutils "github.com/gardener/etcd-druid/test/utils"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/gomega"
)

func TestMapEtcdToSecret(t *testing.T) {
	testCases := []struct {
		name             string
		withClientTLS    bool
		withPeerTLS      bool
		withBackup       bool
		expectedRequests []reconcile.Request
	}{
		{
			name:             "etcd with no client and peer TLS and no store configured",
			expectedRequests: []reconcile.Request{},
		},
		{
			name:       "etcd with no client and peer TLS and backup store configured",
			withBackup: true,
			expectedRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: testutils.BackupStoreSecretName, Namespace: testutils.TestNamespace}},
			},
		},
		{
			name:          "etcd with only client TLS and no store configured",
			withClientTLS: true,
			expectedRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSClientCertSecretName, Namespace: testutils.TestNamespace}},
			},
		},
		{
			name:          "etcd with both client and peer TLS and no store configured",
			withClientTLS: true,
			withPeerTLS:   true,
			expectedRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSClientCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.PeerTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.PeerTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
			},
		},
		{
			name:          "etcd with both client and peer TLS and backup store configured",
			withClientTLS: true,
			withPeerTLS:   true,
			withBackup:    true,
			expectedRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSClientCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.PeerTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.PeerTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.BackupStoreSecretName, Namespace: testutils.TestNamespace}},
			},
		},
	}

	g := NewWithT(t)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			etcdBuilder := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testutils.TestNamespace)
			if tc.withClientTLS {
				etcdBuilder.WithClientTLS()
			}
			if tc.withPeerTLS {
				etcdBuilder.WithPeerTLS()
			}
			if tc.withBackup {
				etcdBuilder.WithDefaultBackup()
			}
			etcd := etcdBuilder.Build()
			actualRequests := mapEtcdToSecret(context.Background(), etcd)
			g.Expect(actualRequests).To(ConsistOf(tc.expectedRequests))
		})
	}
}
