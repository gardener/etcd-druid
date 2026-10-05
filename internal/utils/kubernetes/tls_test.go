// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	"context"
	"crypto/tls"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
	kutil "github.com/gardener/etcd-druid/internal/utils/kubernetes"
	testutils "github.com/gardener/etcd-druid/test/utils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/gomega"
)

const (
	tlsTestNamespace = "test-ns"
	caSecretName     = "etcd-client-ca"
	clientSecretName = "etcd-client-cert"
)

func tlsTestSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: tlsTestNamespace},
		Data:       data,
	}
}

// etcdWithClientTLS returns an Etcd whose spec.etcd.clientUrlTLS references the
// given CA and (optionally) client-cert Secrets. An empty clientCertSecretName
// leaves the client cert unset, exercising the CA-only path.
func etcdWithClientTLS(caCertDataKey, clientCertSecretName string) *druidv1alpha1.Etcd {
	etcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{Name: "etcd-test", Namespace: tlsTestNamespace},
	}
	etcd.Spec.Etcd.ClientUrlTLS = &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{Name: caSecretName},
			DataKey:         ptr.To(caCertDataKey),
		},
		ClientTLSSecretRef: corev1.SecretReference{Name: clientCertSecretName},
	}
	return etcd
}

// TestGetEtcdClientSchemeAndTLSConfig verifies scheme/TLS resolution from the Etcd
// CR: the plaintext path when clientUrlTLS is unset, CA-only and full-mTLS configs
// when it is set, the CA data-key override, and fail-closed behaviour on a missing
// or unparsable CA and on a missing client-cert Secret.
func TestGetEtcdClientSchemeAndTLSConfig(t *testing.T) {
	caPEM, err := testutils.GenerateCACert("etcd-client")
	if err != nil {
		t.Fatalf("failed to generate CA cert: %v", err)
	}
	clientCertPEM, clientKeyPEM, err := testutils.GenerateClientCert("etcd-client")
	if err != nil {
		t.Fatalf("failed to generate client cert: %v", err)
	}

	tests := []struct {
		name         string
		etcd         *druidv1alpha1.Etcd
		objects      []client.Object
		wantScheme   string
		wantTLS      bool
		wantClientCA bool
		wantErr      bool
	}{
		{
			name:       "no client TLS returns http and nil config",
			etcd:       &druidv1alpha1.Etcd{ObjectMeta: metav1.ObjectMeta{Name: "etcd-test", Namespace: tlsTestNamespace}},
			wantScheme: "http",
		},
		{
			name:       "CA-only config populates RootCAs without client cert",
			etcd:       etcdWithClientTLS("ca.crt", ""),
			objects:    []client.Object{tlsTestSecret(caSecretName, map[string][]byte{"ca.crt": caPEM})},
			wantScheme: "https",
			wantTLS:    true,
		},
		{
			name:       "CA under overridden data key",
			etcd:       etcdWithClientTLS("bundle.crt", ""),
			objects:    []client.Object{tlsTestSecret(caSecretName, map[string][]byte{"bundle.crt": caPEM})},
			wantScheme: "https",
			wantTLS:    true,
		},
		{
			name: "client cert secret adds mTLS keypair",
			etcd: etcdWithClientTLS("ca.crt", clientSecretName),
			objects: []client.Object{
				tlsTestSecret(caSecretName, map[string][]byte{"ca.crt": caPEM}),
				tlsTestSecret(clientSecretName, map[string][]byte{"tls.crt": clientCertPEM, "tls.key": clientKeyPEM}),
			},
			wantScheme:   "https",
			wantTLS:      true,
			wantClientCA: true,
		},
		{
			name:    "missing CA secret is an error",
			etcd:    etcdWithClientTLS("ca.crt", ""),
			wantErr: true,
		},
		{
			name:    "unparsable CA bundle is an error",
			etcd:    etcdWithClientTLS("ca.crt", ""),
			objects: []client.Object{tlsTestSecret(caSecretName, map[string][]byte{"ca.crt": []byte("not-a-pem")})},
			wantErr: true,
		},
		{
			name: "missing client cert secret is an error",
			etcd: etcdWithClientTLS("ca.crt", clientSecretName),
			objects: []client.Object{
				tlsTestSecret(caSecretName, map[string][]byte{"ca.crt": caPEM}),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).WithObjects(tc.objects...).Build()

			scheme, tlsConfig, err := kutil.GetEtcdClientSchemeAndTLSConfig(context.Background(), cl, tc.etcd)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(scheme).To(Equal(tc.wantScheme))
			if !tc.wantTLS {
				g.Expect(tlsConfig).To(BeNil())
				return
			}
			g.Expect(tlsConfig).NotTo(BeNil())
			g.Expect(tlsConfig.RootCAs).NotTo(BeNil())
			g.Expect(tlsConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
			if tc.wantClientCA {
				g.Expect(tlsConfig.Certificates).To(HaveLen(1))
			} else {
				g.Expect(tlsConfig.Certificates).To(BeEmpty())
			}
		})
	}
}
