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
		withBackupTLS    bool
		withBackup       bool
		expectedRequests []reconcile.Request
	}{
		{
			name:             "etcd configured with no client and peer TLS and with no backup store",
			expectedRequests: []reconcile.Request{},
		},
		{
			name:       "etcd configured with backup store and with no client and peer TLS",
			withBackup: true,
			expectedRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: testutils.BackupStoreSecretName, Namespace: testutils.TestNamespace}},
			},
		},
		{
			name:          "etcd configured with only client TLS and with no backup store",
			withClientTLS: true,
			expectedRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSClientCertSecretName, Namespace: testutils.TestNamespace}},
			},
		},
		{
			name:          "etcd configured with both client and peer TLS but with no backup store",
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
			name:          "etcd configured with only backup-restore TLS",
			withBackupTLS: true,
			expectedRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: testutils.BackupRestoreTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.BackupRestoreTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.BackupRestoreTLSClientCertSecretName, Namespace: testutils.TestNamespace}},
			},
		},
		{
			name:          "etcd configured with both client and peer TLS and with backup store",
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
		{
			name:          "etcd configured with client TLS, peer TLS and backup-restore TLS and with backup store",
			withClientTLS: true,
			withPeerTLS:   true,
			withBackup:    true,
			withBackupTLS: true,
			expectedRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.ClientTLSClientCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.PeerTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.PeerTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.BackupRestoreTLSCASecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.BackupRestoreTLSServerCertSecretName, Namespace: testutils.TestNamespace}},
				{NamespacedName: types.NamespacedName{Name: testutils.BackupRestoreTLSClientCertSecretName, Namespace: testutils.TestNamespace}},
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
			if tc.withBackupTLS {
				etcdBuilder.WithBackupRestoreTLS()
			}
			etcd := etcdBuilder.Build()
			actualRequests := mapEtcdToSecret(context.Background(), etcd)
			g.Expect(actualRequests).To(ConsistOf(tc.expectedRequests))
		})
	}
}
