// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations of etcd.spec.etcd.additionalAdvertisePeerUrls field.

package etcd

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"
)

// TestValidateSpecEtcdAdditionalAdvertisePeerUrlsMemberName validates the member name CEL validations:
// - Member name must start with {metadata.name}- prefix
// - Member name must match DNS label pattern with numeric suffix
// - Member name index must be less than replicas count
func TestValidateSpecEtcdAdditionalAdvertisePeerUrlsMemberName(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)

	tests := []struct {
		name       string
		etcdName   string
		replicas   int32
		memberName string
		urls       []string
		expectErr  bool
	}{
		{
			name:       "Valid: member name with correct prefix and index",
			etcdName:   "etcd-main",
			replicas:   3,
			memberName: "etcd-main-0",
			urls:       []string{"http://10.0.0.1:2380"},
			expectErr:  false,
		},
		{
			name:       "Valid: member name with index at boundary",
			etcdName:   "etcd-test",
			replicas:   5,
			memberName: "etcd-test-4",
			urls:       []string{"http://10.0.0.5:2380"},
			expectErr:  false,
		},
		{
			name:       "Invalid: member name without correct CR name prefix",
			etcdName:   "etcd-main",
			replicas:   3,
			memberName: "etcd-other-0",
			urls:       []string{"http://10.0.0.1:2380"},
			expectErr:  true,
		},
		{
			name:       "Invalid: member name index >= replicas",
			etcdName:   "etcd-main",
			replicas:   3,
			memberName: "etcd-main-3",
			urls:       []string{"http://10.0.0.4:2380"},
			expectErr:  true,
		},
		{
			name:       "Invalid: member name with non-numeric suffix",
			etcdName:   "etcd-main",
			replicas:   3,
			memberName: "etcd-main-abc",
			urls:       []string{"http://10.0.0.1:2380"},
			expectErr:  true,
		},
		{
			name:       "Invalid: member name without numeric suffix",
			etcdName:   "etcd-main",
			replicas:   3,
			memberName: "etcd-main-",
			urls:       []string{"http://10.0.0.1:2380"},
			expectErr:  true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).
				WithReplicas(test.replicas).
				Build()

			etcd.Spec.Etcd.AdditionalAdvertisePeerURLs = []druidv1alpha1.AdditionalPeerURL{
				{
					MemberName: test.memberName,
					URLs:       test.urls,
				},
			}

			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}

// TestValidateSpecEtcdAdditionalAdvertisePeerUrlsTLSScheme validates TLS scheme consistency:
// - HTTP URLs when peerUrlTLS is disabled
// - HTTPS URLs when peerUrlTLS is enabled
func TestValidateSpecEtcdAdditionalAdvertisePeerUrlsTLSScheme(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)

	tests := []struct {
		name       string
		etcdName   string
		tlsEnabled bool
		memberName string
		urls       []string
		expectErr  bool
	}{
		{
			name:       "Valid: HTTP URL without TLS",
			etcdName:   "etcd-http",
			tlsEnabled: false,
			memberName: "etcd-http-0",
			urls:       []string{"http://10.0.0.1:2380"},
			expectErr:  false,
		},
		{
			name:       "Valid: HTTPS URL with TLS enabled",
			etcdName:   "etcd-https",
			tlsEnabled: true,
			memberName: "etcd-https-0",
			urls:       []string{"https://10.0.0.1:2380"},
			expectErr:  false,
		},
		{
			name:       "Valid: multiple HTTP URLs without TLS",
			etcdName:   "etcd-multi-http",
			tlsEnabled: false,
			memberName: "etcd-multi-http-0",
			urls: []string{
				"http://10.0.0.1:2380",
				"http://10.0.0.2:2380",
			},
			expectErr: false,
		},
		{
			name:       "Valid: multiple HTTPS URLs with TLS",
			etcdName:   "etcd-multi-https",
			tlsEnabled: true,
			memberName: "etcd-multi-https-0",
			urls: []string{
				"https://10.0.0.1:2380",
				"https://10.0.0.2:2380",
			},
			expectErr: false,
		},
		{
			name:       "Invalid: HTTP URL when TLS enabled",
			etcdName:   "etcd-http-with-tls",
			tlsEnabled: true,
			memberName: "etcd-http-with-tls-0",
			urls:       []string{"http://10.0.0.1:2380"},
			expectErr:  true,
		},
		{
			name:       "Invalid: HTTPS URL when TLS disabled",
			etcdName:   "etcd-https-no-tls",
			tlsEnabled: false,
			memberName: "etcd-https-no-tls-0",
			urls:       []string{"https://10.0.0.1:2380"},
			expectErr:  true,
		},
		{
			name:       "Invalid: mixed HTTP/HTTPS URLs",
			etcdName:   "etcd-mixed",
			tlsEnabled: false,
			memberName: "etcd-mixed-0",
			urls: []string{
				"http://10.0.0.1:2380",
				"https://10.0.0.2:2380",
			},
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).
				WithReplicas(3)

			// Configure TLS if enabled
			if test.tlsEnabled {
				builder = builder.WithPeerTLS()
			}

			etcd := builder.Build()

			etcd.Spec.Etcd.AdditionalAdvertisePeerURLs = []druidv1alpha1.AdditionalPeerURL{
				{
					MemberName: test.memberName,
					URLs:       test.urls,
				},
			}

			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}

// TestValidateSpecEtcdAdditionalAdvertisePeerUrlsMultipleMembers validates
// configuration with multiple members
func TestValidateSpecEtcdAdditionalAdvertisePeerUrlsMultipleMembers(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)

	tests := []struct {
		name      string
		etcdName  string
		replicas  int32
		peerURLs  []druidv1alpha1.AdditionalPeerURL
		expectErr bool
	}{
		{
			name:     "Valid: multiple members with valid names and URLs",
			etcdName: "etcd-multi",
			replicas: 3,
			peerURLs: []druidv1alpha1.AdditionalPeerURL{
				{
					MemberName: "etcd-multi-0",
					URLs:       []string{"http://10.0.0.1:2380"},
				},
				{
					MemberName: "etcd-multi-1",
					URLs:       []string{"http://10.0.0.2:2380"},
				},
				{
					MemberName: "etcd-multi-2",
					URLs:       []string{"http://10.0.0.3:2380"},
				},
			},
			expectErr: false,
		},
		{
			name:     "Valid: subset of members configured",
			etcdName: "etcd-subset",
			replicas: 5,
			peerURLs: []druidv1alpha1.AdditionalPeerURL{
				{
					MemberName: "etcd-subset-0",
					URLs:       []string{"http://10.0.0.1:2380"},
				},
				{
					MemberName: "etcd-subset-2",
					URLs:       []string{"http://10.0.0.3:2380"},
				},
			},
			expectErr: false,
		},
		{
			name:     "Invalid: one member with wrong prefix",
			etcdName: "etcd-mixed",
			replicas: 3,
			peerURLs: []druidv1alpha1.AdditionalPeerURL{
				{
					MemberName: "etcd-mixed-0",
					URLs:       []string{"http://10.0.0.1:2380"},
				},
				{
					MemberName: "etcd-other-1",
					URLs:       []string{"http://10.0.0.2:2380"},
				},
			},
			expectErr: true,
		},
		{
			name:     "Invalid: one member with index out of bounds",
			etcdName: "etcd-bounds",
			replicas: 3,
			peerURLs: []druidv1alpha1.AdditionalPeerURL{
				{
					MemberName: "etcd-bounds-0",
					URLs:       []string{"http://10.0.0.1:2380"},
				},
				{
					MemberName: "etcd-bounds-5",
					URLs:       []string{"http://10.0.0.6:2380"},
				},
			},
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).
				WithReplicas(test.replicas).
				Build()

			etcd.Spec.Etcd.AdditionalAdvertisePeerURLs = test.peerURLs

			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}
