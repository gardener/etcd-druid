// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
	testutils "github.com/gardener/etcd-druid/test/utils"

	corev1 "k8s.io/api/core/v1"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/gomega"
)

const (
	memberClientEtcdName  = "etcd-main"
	memberClientNamespace = "test-ns"
)

// TestNewMemberClientFailsClosedWhenTLSExpectedButUnresolved verifies that when
// client TLS is enabled on the Etcd spec but the CA cannot be resolved from the
// running StatefulSet (StatefulSet or CA volume absent), NewMemberClient fails
// closed rather than falling back to a plaintext dial.
func TestNewMemberClientFailsClosedWhenTLSExpectedButUnresolved(t *testing.T) {
	g := NewWithT(t)

	etcd := testutils.EtcdBuilderWithoutDefaults(memberClientEtcdName, memberClientNamespace).Build()
	etcd.Spec.Etcd.ClientUrlTLS = &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{Name: "ca-etcd", Namespace: memberClientNamespace},
		},
	}

	// No StatefulSet object exists, so the CA cannot be resolved from a volume.
	cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).Build()

	factory := NewMemberClientFactory()
	_, err := factory.NewMemberClient(context.Background(), cl, etcd)
	g.Expect(err).To(HaveOccurred())
}

// TestClientv3MemberClientCloseNilClient verifies Close is nil-safe: a
// client with no underlying connection (e.g. constructed then never connected)
// closes without panicking or erroring.
func TestClientv3MemberClientCloseNilClient(t *testing.T) {
	g := NewWithT(t)
	c := &v3Client{cli: nil}
	g.Expect(c.Close()).To(Succeed())
}
