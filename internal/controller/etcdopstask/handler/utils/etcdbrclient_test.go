// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"net/http"
	"testing"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	testutils "github.com/gardener/etcd-druid/test/utils"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// TestConfigureHTTPClientForEtcdBR tests the ConfigureHTTPClientForEtcdBR function.
func TestConfigureHTTPClientForEtcdBR(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name            string
		etcd            *druidv1alpha1.Etcd
		existingObjects []client.Object
		defaultClient   http.Client
		phase           druidapicommon.LastOperationType
		expectedScheme  string
		expectedResult  taskhandler.Result
		validateTLS     bool
	}{
		{
			name: "Should return http scheme and default client when TLS is not configured",
			etcd: &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-etcd",
					Namespace: "test-namespace",
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						TLS: nil,
					},
				},
			},
			existingObjects: nil,
			defaultClient:   http.Client{},
			phase:           druidv1alpha1.LastOperationTypeExecution,
			expectedScheme:  "http",
			expectedResult:  taskhandler.Result{},
			validateTLS:     false,
		},
		{
			name: "Should return error without requeue when CA secret is not found",
			etcd: &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-etcd",
					Namespace: "test-namespace",
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						TLS: &druidv1alpha1.TLSConfig{
							TLSCASecretRef: druidv1alpha1.SecretReference{
								SecretReference: v1.SecretReference{
									Name: "ca-secret",
								},
								DataKey: ptr.To("ca.crt"),
							},
						},
					},
				},
			},
			existingObjects: nil,
			defaultClient:   http.Client{},
			phase:           druidv1alpha1.LastOperationTypeExecution,
			expectedScheme:  "",
			expectedResult: taskhandler.Result{
				Error: &druiderr.DruidError{
					Code:      taskhandler.ErrGetCASecret,
					Operation: string(druidv1alpha1.LastOperationTypeExecution),
				},
				Requeue: false,
			},
			validateTLS: false,
		},
		{
			name: "Should return error without requeue when CA cert data key is not found in secret",
			etcd: &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-etcd",
					Namespace: "test-namespace",
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						TLS: &druidv1alpha1.TLSConfig{
							TLSCASecretRef: druidv1alpha1.SecretReference{
								SecretReference: v1.SecretReference{
									Name: "ca-secret",
								},
								DataKey: ptr.To("ca.crt"),
							},
						},
					},
				},
			},
			existingObjects: []client.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ca-secret",
						Namespace: "test-namespace",
					},
					Data: map[string][]byte{
						"wrong-key": []byte("some-data"),
					},
				},
			},
			defaultClient:  http.Client{},
			phase:          druidv1alpha1.LastOperationTypeExecution,
			expectedScheme: "",
			expectedResult: taskhandler.Result{
				Error: &druiderr.DruidError{
					Code:      taskhandler.ErrCADataKeyNotFound,
					Operation: string(druidv1alpha1.LastOperationTypeExecution),
				},
				Requeue: false,
			},
			validateTLS: false,
		},
		{
			name: "Should return error without requeue when CA cert data is invalid",
			etcd: &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-etcd",
					Namespace: "test-namespace",
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						TLS: &druidv1alpha1.TLSConfig{
							TLSCASecretRef: druidv1alpha1.SecretReference{
								SecretReference: v1.SecretReference{
									Name: "ca-secret",
								},
								DataKey: ptr.To("ca.crt"),
							},
						},
					},
				},
			},
			existingObjects: []client.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ca-secret",
						Namespace: "test-namespace",
					},
					Data: map[string][]byte{
						"ca.crt": []byte("invalid-cert-data"),
					},
				},
			},
			defaultClient:  http.Client{},
			phase:          druidv1alpha1.LastOperationTypeExecution,
			expectedScheme: "",
			expectedResult: taskhandler.Result{
				Error: &druiderr.DruidError{
					Code:      taskhandler.ErrAppendCACerts,
					Operation: string(druidv1alpha1.LastOperationTypeExecution),
				},
				Requeue: false,
			},
			validateTLS: false,
		},
		{
			name: "Should successfully configure HTTPS client with valid CA cert",
			etcd: &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-etcd",
					Namespace: "test-namespace",
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						TLS: &druidv1alpha1.TLSConfig{
							TLSCASecretRef: druidv1alpha1.SecretReference{
								SecretReference: v1.SecretReference{
									Name: "ca-secret",
								},
								DataKey: ptr.To("ca.crt"),
							},
						},
					},
				},
			},
			existingObjects: []client.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ca-secret",
						Namespace: "test-namespace",
					},
					Data: map[string][]byte{
						"ca.crt": testutils.GenerateTestCA(),
					},
				},
			},
			defaultClient:  http.Client{},
			phase:          druidv1alpha1.LastOperationTypeExecution,
			expectedScheme: "https",
			expectedResult: taskhandler.Result{},
			validateTLS:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cl := testutils.NewTestClientBuilder().
				WithScheme(kubernetes.Scheme).
				WithObjects(tc.existingObjects...).
				Build()

			httpClient, httpScheme, errResult := ConfigureHTTPClientForEtcdBR(
				context.Background(),
				cl,
				tc.etcd,
				tc.defaultClient,
				tc.phase,
			)

			if tc.expectedResult.Error != nil {
				g.Expect(errResult).ToNot(BeNil())
				g.Expect(errResult.Error).To(HaveOccurred())
				g.Expect(errResult.Requeue).To(Equal(tc.expectedResult.Requeue))

				expectedErr := tc.expectedResult.Error.(*druiderr.DruidError)
				var actualErr *druiderr.DruidError
				g.Expect(errors.As(errResult.Error, &actualErr)).To(BeTrue())
				g.Expect(actualErr.Code).To(Equal(expectedErr.Code))
				g.Expect(actualErr.Operation).To(Equal(expectedErr.Operation))
			} else {
				g.Expect(errResult).To(BeNil())
				g.Expect(httpScheme).To(Equal(tc.expectedScheme))

				if tc.validateTLS {
					g.Expect(httpClient.Transport).ToNot(BeNil())
					transport, ok := httpClient.Transport.(*http.Transport)
					g.Expect(ok).To(BeTrue())
					g.Expect(transport.TLSClientConfig).ToNot(BeNil())
					g.Expect(transport.TLSClientConfig.RootCAs).ToNot(BeNil())
				} else if tc.expectedScheme == "http" {
					g.Expect(httpClient).To(Equal(tc.defaultClient))
				}
			}
		})
	}
}
