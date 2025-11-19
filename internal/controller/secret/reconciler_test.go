// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"testing"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

type etcdBuildInfo struct {
	name          string
	withClientTLS bool
	withPeerTLS   bool
	withBackup    bool
}

func TestIsFinalizerNeeded(t *testing.T) {
	const unknownSecretName = "some-secret"
	testCases := []struct {
		name          string
		secretName    string
		etcdResources []etcdBuildInfo
		expected      bool
	}{
		{
			name:       "there is no etcd resource",
			secretName: unknownSecretName,
			expected:   false,
		},
		{
			name:       "there is an etcd resource with no TLS and no backup store",
			secretName: unknownSecretName,
			etcdResources: []etcdBuildInfo{
				{name: testutils.TestEtcdName, withClientTLS: false, withPeerTLS: false, withBackup: false},
			},
			expected: false,
		},
		{
			name:       "there is one etcd resource with backup stored configured but no TLS, secret name does not match backup store secret",
			secretName: testutils.ClientTLSCASecretName,
			etcdResources: []etcdBuildInfo{
				{name: "test-etcd-with-backup", withClientTLS: false, withPeerTLS: false, withBackup: true},
			},
			expected: false,
		},
		{
			name:       "etcd is configured with client and peer TLS and has backup store configured, but secretname does not match any secret",
			secretName: unknownSecretName,
			etcdResources: []etcdBuildInfo{
				{name: "test-etcd-with-backup", withClientTLS: true, withPeerTLS: true, withBackup: true},
			},
			expected: false,
		},
		{
			name:       "there is one etcd resource with backup stored configured but no TLS, secret name matches backup store secret",
			secretName: testutils.BackupStoreSecretName,
			etcdResources: []etcdBuildInfo{
				{name: "test-etcd-with-backup", withClientTLS: false, withPeerTLS: false, withBackup: true},
				{name: "test-etcd-without-backup", withClientTLS: true, withPeerTLS: false, withBackup: false},
			},
			expected: true,
		},
		{
			name:       "there is one etcd resource with client TLS, secret name matches client TLS CA secret",
			secretName: testutils.ClientTLSCASecretName,
			etcdResources: []etcdBuildInfo{
				{name: "test-etcd-with-client-tls", withClientTLS: true, withPeerTLS: false, withBackup: false},
				{name: "test-etcd-with-backup", withClientTLS: false, withPeerTLS: false, withBackup: true},
			},
			expected: true,
		},
		{
			name:       "there is at least one etcd with client TLS, secret name matches client TLS server secret",
			secretName: testutils.ClientTLSServerCertSecretName,
			etcdResources: []etcdBuildInfo{
				{name: "test-etcd-with-client-tls", withClientTLS: true, withPeerTLS: false, withBackup: false},
				{name: "test-etcd-with-backup", withClientTLS: false, withPeerTLS: false, withBackup: true},
			},
			expected: true,
		},
		{
			name:       "there is at least one etcd with client TLS, secret name matches client TLS client secret",
			secretName: testutils.ClientTLSServerCertSecretName,
			etcdResources: []etcdBuildInfo{
				{name: "test-etcd-with-client-tls", withClientTLS: true, withPeerTLS: false, withBackup: false},
				{name: "test-etcd-with-backup", withClientTLS: false, withPeerTLS: false, withBackup: true},
			},
			expected: true,
		},
		{
			name:       "there is at least one etcd with peer TLS, secret name matches peer TLS CA secret",
			secretName: testutils.PeerTLSCASecretName,
			etcdResources: []etcdBuildInfo{
				{name: "test-etcd-with-peer-tls", withClientTLS: false, withPeerTLS: true, withBackup: false},
				{name: "test-etcd-with-client-tls-and-backup", withClientTLS: true, withPeerTLS: false, withBackup: true},
			},
			expected: true,
		},
		{
			name:       "there is at least one etcd with peer TLS, secret name matches peer TLS server cert secret",
			secretName: testutils.PeerTLSServerCertSecretName,
			etcdResources: []etcdBuildInfo{
				{name: "test-etcd-with-client-and-peer-tls", withClientTLS: true, withPeerTLS: true, withBackup: false},
			},
			expected: true,
		},
	}

	g := NewWithT(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			etcdList := createEtcdList(tc.etcdResources)
			actual, _ := isFinalizerNeeded(tc.secretName, &etcdList)
			g.Expect(actual).To(Equal(tc.expected))
		})
	}
}

func TestAddFinalizer(t *testing.T) {
	const testSecretName = "test-secret"
	testCases := []struct {
		name          string
		secretExists  bool
		hasFinalizer  bool
		patchErr      *apierrors.StatusError
		expectedError *apierrors.StatusError
	}{
		{
			name:          "secret does not exist",
			expectedError: nil,
		},
		{
			name:          "secret already has finalizer",
			secretExists:  true,
			hasFinalizer:  true,
			expectedError: nil,
		},
		{
			name:          "secret does not have finalizer",
			secretExists:  true,
			hasFinalizer:  false,
			expectedError: nil,
		},
		{
			name:          "secret does not have finalizer, patch error",
			secretExists:  true,
			hasFinalizer:  false,
			patchErr:      testutils.TestAPIInternalErr,
			expectedError: testutils.TestAPIInternalErr,
		},
	}

	g := NewWithT(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var (
				secret     *corev1.Secret
				cl         client.Client
				ctx        = context.Background()
				finalizers []string
			)
			if tc.hasFinalizer {
				finalizers = []string{druidapicommon.EtcdFinalizerName}
			}
			secret = buildSecretResource(testSecretName, testutils.TestNamespace, finalizers)
			if tc.secretExists {
				cl = testutils.CreateTestFakeClientForObjects(nil, nil, tc.patchErr, nil, []client.Object{secret}, client.ObjectKeyFromObject(secret))
			} else {
				secret.ResourceVersion = "1"
				cl = testutils.CreateDefaultFakeClient()
			}

			err := addFinalizer(ctx, logr.Discard(), cl, secret)
			if tc.expectedError != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tc.expectedError))
			} else {
				g.Expect(err).To(BeNil())
				updatedSecret := &corev1.Secret{}
				getErr := cl.Get(ctx, client.ObjectKeyFromObject(secret), updatedSecret)
				if tc.secretExists {
					g.Expect(getErr).To(BeNil())
					g.Expect(updatedSecret.Finalizers).To(ContainElement(druidapicommon.EtcdFinalizerName))
				} else {
					g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
				}
			}
		})
	}
}

func TestRemoveFinalizer(t *testing.T) {
	const testSecretName = "test-secret"
	testCases := []struct {
		name                        string
		secretExists                bool
		finalizers                  []string
		patchErr                    *apierrors.StatusError
		expectedError               *apierrors.StatusError
		expectedRemainingFinalizers []string
	}{
		{
			name:          "secret does not exist",
			expectedError: nil,
		},
		{
			name:                        "secret does not have finalizer",
			secretExists:                true,
			finalizers:                  []string{"bingo"},
			expectedError:               nil,
			expectedRemainingFinalizers: []string{"bingo"},
		},
		{
			name:                        "secret has desired finalizer",
			secretExists:                true,
			finalizers:                  []string{druidapicommon.EtcdFinalizerName, "bingo"},
			expectedError:               nil,
			expectedRemainingFinalizers: []string{"bingo"},
		},
		{
			name:                        "secret has finalizer, patch error",
			secretExists:                true,
			finalizers:                  []string{druidapicommon.EtcdFinalizerName, "bingo"},
			patchErr:                    testutils.TestAPIInternalErr,
			expectedError:               testutils.TestAPIInternalErr,
			expectedRemainingFinalizers: []string{druidapicommon.EtcdFinalizerName, "bingo"},
		},
	}

	g := NewWithT(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var (
				secret *corev1.Secret
				cl     client.Client
				ctx    = context.Background()
			)
			secret = buildSecretResource(testSecretName, testutils.TestNamespace, tc.finalizers)
			if tc.secretExists {
				cl = testutils.CreateTestFakeClientForObjects(nil, nil, tc.patchErr, nil, []client.Object{secret}, client.ObjectKeyFromObject(secret))
			} else {
				secret.ResourceVersion = "1"
				cl = testutils.CreateDefaultFakeClient()
			}

			err := removeFinalizer(ctx, logr.Discard(), cl, secret)
			if tc.expectedError != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tc.expectedError))
			} else {
				g.Expect(err).To(BeNil())
				updatedSecret := &corev1.Secret{}
				getErr := cl.Get(ctx, client.ObjectKeyFromObject(secret), updatedSecret)
				if tc.secretExists {
					g.Expect(getErr).To(BeNil())
					g.Expect(updatedSecret.Finalizers).To(Equal(tc.expectedRemainingFinalizers))
				} else {
					g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
				}
			}
		})
	}
}

func createEtcdList(etcdBuildInfos []etcdBuildInfo) druidv1alpha1.EtcdList {
	etcdList := druidv1alpha1.EtcdList{}
	for _, info := range etcdBuildInfos {
		etcdBuilder := testutils.EtcdBuilderWithoutDefaults(info.name, testutils.TestNamespace)
		if info.withPeerTLS {
			_ = etcdBuilder.WithPeerTLS()
		}
		if info.withClientTLS {
			_ = etcdBuilder.WithClientTLS()
		}
		if info.withBackup {
			_ = etcdBuilder.WithDefaultBackup()
		}
		etcdList.Items = append(etcdList.Items, *etcdBuilder.Build())
	}
	return etcdList
}

func buildSecretResource(name, namespace string, finalizers []string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: finalizers,
		},
		Data: map[string][]byte{
			"key": []byte("tringo"),
		},
	}
}
