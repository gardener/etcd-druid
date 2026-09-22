// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	"context"
	"crypto/tls"
	"testing"

	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
	kutil "github.com/gardener/etcd-druid/internal/utils/kubernetes"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/gomega"
)

const (
	tlsTestNamespace = "test-ns"
)

func tlsTestSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: tlsTestNamespace},
		Data:       data,
	}
}

// TestBuildBackupRestoreCATLSConfig verifies the CA-only config assembly for the
// backup-restore client: the CA is resolved from the backup-restore-ca volume, the
// data key override is honoured, and absence of the volume yields (nil, nil).
func TestBuildBackupRestoreCATLSConfig(t *testing.T) {
	caPEM, err := testutils.GenerateCACert("etcdbr")
	if err != nil {
		t.Fatalf("failed to generate CA cert: %v", err)
	}

	stsWithCA := func() *appsv1.StatefulSet {
		return testutils.AddBackupRestoreCAVolume(
			testutils.CreateStatefulSet("test-sts", tlsTestNamespace, uuid.NewUUID(), 1),
			"ca-etcdbr",
		)
	}

	tests := []struct {
		name      string
		sts       *appsv1.StatefulSet
		caDataKey string
		objects   []client.Object
		wantNil   bool
		wantErr   bool
	}{
		{
			name:      "CA under bundle.crt populates RootCAs",
			sts:       stsWithCA(),
			caDataKey: "bundle.crt",
			objects:   []client.Object{tlsTestSecret("ca-etcdbr", map[string][]byte{"bundle.crt": caPEM})},
		},
		{
			name:      "CA under overridden data key",
			sts:       stsWithCA(),
			caDataKey: "ca.crt",
			objects:   []client.Object{tlsTestSecret("ca-etcdbr", map[string][]byte{"ca.crt": caPEM})},
		},
		{
			name:      "nil StatefulSet returns nil config",
			sts:       nil,
			caDataKey: "bundle.crt",
			wantNil:   true,
		},
		{
			name:      "missing CA volume returns nil config",
			sts:       testutils.CreateStatefulSet("test-sts", tlsTestNamespace, uuid.NewUUID(), 1),
			caDataKey: "bundle.crt",
			wantNil:   true,
		},
		{
			name:      "missing CA secret is an error",
			sts:       stsWithCA(),
			caDataKey: "bundle.crt",
			wantErr:   true,
		},
		{
			name:      "unparsable CA bundle is an error",
			sts:       stsWithCA(),
			caDataKey: "bundle.crt",
			objects:   []client.Object{tlsTestSecret("ca-etcdbr", map[string][]byte{"bundle.crt": []byte("not-a-pem")})},
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).WithObjects(tc.objects...).Build()

			tlsConfig, err := kutil.BuildBackupRestoreCATLSConfig(context.Background(), cl, tc.sts, tlsTestNamespace, tc.caDataKey)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			if tc.wantNil {
				g.Expect(tlsConfig).To(BeNil())
				return
			}
			g.Expect(tlsConfig).NotTo(BeNil())
			g.Expect(tlsConfig.RootCAs).NotTo(BeNil())
			g.Expect(tlsConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
		})
	}
}
