// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"fmt"
	"net/http"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"
)

type mockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.DoFunc != nil {
		return m.DoFunc(req)
	}
	return nil, nil
}

// ---------------------- Full Snapshot Tests ----------------
func TestFullSnapshot(t *testing.T) {
	var (
		eb               = testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testutils.TestNamespace)
		backupPort int32 = 8080
	)

	testCases := []struct {
		name        string
		httpScheme  string
		etcd        *druidv1alpha1.Etcd
		httpClient  httpClientInterface
		expectError bool
		checkErr    func(error) bool
	}{
		{
			name:       "should trigger full snapshot with HTTP scheme (success)",
			httpScheme: "http",
			etcd:       eb.WithBackupPort(&backupPort).Build(),
			httpClient: &mockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					if req.URL.String() != fmt.Sprintf(
						"http://%s.%s.svc.cluster.local:%d/snapshot/full",
						druidv1alpha1.GetClientServiceName(eb.Build().ObjectMeta),
						testutils.TestNamespace,
						backupPort,
					) {
						return nil, fmt.Errorf("unexpected URL: %s", req.URL.String())
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       http.NoBody,
					}, nil
				},
			},
			expectError: false,
		},
		{
			name:       "should trigger full snapshot with HTTPS scheme (success)",
			httpScheme: "https",
			etcd:       eb.WithBackupPort(&backupPort).Build(),
			httpClient: &mockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					if req.URL.String() != fmt.Sprintf(
						"https://%s.%s.svc.cluster.local:%d/snapshot/full",
						druidv1alpha1.GetClientServiceName(eb.Build().ObjectMeta),
						testutils.TestNamespace,
						backupPort,
					) {
						return nil, fmt.Errorf("unexpected URL: %s", req.URL.String())
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       http.NoBody,
					}, nil
				},
			},
			expectError: false,
		},
		{
			name:       "should return error if HTTP client returns error",
			httpScheme: "http",
			etcd:       eb.WithBackupPort(&backupPort).Build(),
			httpClient: &mockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					if req.URL.String() != fmt.Sprintf(
						"http://%s.%s.svc.cluster.local:%d/snapshot/full",
						druidv1alpha1.GetClientServiceName(eb.Build().ObjectMeta),
						testutils.TestNamespace,
						backupPort,
					) {
						return nil, fmt.Errorf("unexpected URL: %s", req.URL.String())
					}
					return nil, assertErr("client error")
				},
			},
			expectError: true,
			checkErr: func(err error) bool {
				return err != nil && err.Error() == "failed to trigger full snapshot: client error"
			},
		},
		{
			name:       "should return error if status code is not 200",
			httpScheme: "http",
			etcd:       eb.WithBackupPort(&backupPort).Build(),
			httpClient: &mockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					if req.URL.String() != fmt.Sprintf(
						"http://%s.%s.svc.cluster.local:%d/snapshot/full",
						druidv1alpha1.GetClientServiceName(eb.Build().ObjectMeta),
						testutils.TestNamespace,
						backupPort,
					) {
						return nil, fmt.Errorf("unexpected URL: %s", req.URL.String())
					}
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       http.NoBody,
					}, nil
				},
			},
			expectError: true,
			checkErr: func(err error) bool {
				return err != nil && err.Error() == "failed to take full snapshot, received status code: 500"
			},
		},
		{
			name:        "should return error if request creation fails (invalid URL)",
			httpScheme:  string([]byte{123}), // invalid scheme to force error
			etcd:        eb.WithBackupPort(&backupPort).Build(),
			httpClient:  &mockHTTPClient{},
			expectError: true,
		},
	}

	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := fullSnapshot(
				t.Context(),
				tc.etcd,
				tc.httpClient,
				tc.httpScheme,
			)
			if tc.expectError {
				if tc.checkErr != nil {
					if !tc.checkErr(err) {
						t.Errorf("expected different error, got: %w", err)
					}
				} else if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %w", err)
				}
			}
		})
	}
}

func assertErr(msg string) error {
	return &testError{msg: msg}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
