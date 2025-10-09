// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gardener/etcd-druid/api/core/v1alpha1/crds"
	"github.com/gardener/etcd-druid/test/it/assets"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "etcdopstask-test"

var (
	sharedITTestEnv         setup.DruidTestEnvironment
	sharedReconcilerTestEnv ReconcilerTestEnv
	k8sVersionAbove129      bool
)

func TestMain(m *testing.M) {
	var (
		itTestEnvCloser setup.DruidTestEnvCloser
		err             error
	)

	k8sVersion, err := assets.GetK8sVersionFromEnv()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to get the kubernetes version: %v\n", err)
		os.Exit(1)
	}

	k8sVersionAbove129, err = crds.IsK8sVersionEqualToOrAbove129(k8sVersion)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to compare k8s version: %v\n", err)
		os.Exit(1)
	}

	sharedITTestEnv, itTestEnvCloser, err = setup.NewDruidTestEnvironment(testNamespacePrefix, []string{
		assets.GetEtcdOpsTaskCrdPath(),
		assets.GetEtcdCrdPath(k8sVersionAbove129),
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create integration test environment: %v\n", err)
		os.Exit(1)
	}

	// Create shared reconciler test environment
	sharedReconcilerTestEnv = initializeEtcdOpsTaskReconcilerTestEnv(nil, sharedITTestEnv, testutils.NewTestClientBuilder())

	// os.Exit() does not respect defer statements
	exitCode := m.Run()
	itTestEnvCloser()
	os.Exit(exitCode)
}

// -----------------------------------OnDemandSnapshot Task Tests-----------------------------------------------------

// TestETcdOpsTaskConfigValidation tests the validation of EtcdOpsTask spec.config values.
func TestEtcdOpsTaskConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv)
	}{
		{"should fail to create task with empty config", testEtcdOpsTaskCreationFailEmptyConfig},
		{"should successfully create task with onDemandSnapshot config", testEtcdOpsTaskCreationSuccessValidConfig},
	}

	g := NewWithT(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testNs := testutils.GenerateTestNamespaceName(t, testNamespacePrefix)
			t.Logf("successfully created namespace: %s to run test => '%s'", testNs, t.Name())
			g.Expect(sharedReconcilerTestEnv.itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
			tc.fn(t, testNs, sharedReconcilerTestEnv)
		})
	}
}

// TestEtcdOpsTaskAdmitConditions tests the admit conditions of EtcdOpsTask reconciler.
func TestEtcdOpsTaskAdmitConditions(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv)
	}{
		{"should add finalizer to etcdops task", testEtcdOpsTaskAddFinalizer},
		{"Task should be rejected if the etcd's backup is disabled", testEtcdOpsTaskRejecEtcdBackupDisabled},
		{"Task should be rejected if the etcd is not in ready state", testEtcdOpsTaskRejectedIfEtcdNotReady},
		{"Task should be rejected if there is a duplicate task for the same etcd", testEtcdOpsTaskRejectedDuplicateTask},
	}

	g := NewWithT(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testNs := testutils.GenerateTestNamespaceName(t, testNamespacePrefix)
			t.Logf("successfully created namespace: %s to run test => '%s'", testNs, t.Name())
			g.Expect(sharedReconcilerTestEnv.itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
			tc.fn(t, testNs, sharedReconcilerTestEnv)
		})
	}
}

// TestEtcdOpsTaskLifecycleWithHTTPMock tests the lifecycle with HTTP client mocking.
func TestEtcdOpsTaskLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		fn         func(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv)
		httpClient *http.Client
	}{
		{
			"Successful Lifecycle",
			testEtcdOpsTaskSuccessfulLifecycle,
			&http.Client{
				Transport: &testutils.MockRoundTripper{
					Response: &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
						Body:       io.NopCloser(strings.NewReader("OK")),
					},
					Err: nil,
				},
			},
		},
		{
			"UnSuccessful Lifecycle: Run fails",
			testEtcdOpsTaskUnsuccessfulLifecycle,
			&http.Client{
				Transport: &testutils.MockRoundTripper{
					Response: &http.Response{
						StatusCode: http.StatusInternalServerError,
						Status:     "500 Internal Server Error",
						Body:       io.NopCloser(strings.NewReader("Internal Server Error")),
					},
					Err: nil,
				},
			},
		},
	}

	g := NewWithT(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testNs := testutils.GenerateTestNamespaceName(t, testNamespacePrefix)
			t.Logf("successfully created namespace: %s to run test => '%s'", testNs, t.Name())
			g.Expect(sharedReconcilerTestEnv.itTestEnv.CreateTestNamespace(testNs)).To(Succeed())

			sharedReconcilerTestEnv.InjectHTTPClient(tc.httpClient)
			tc.fn(t, testNs, sharedReconcilerTestEnv)
		})
	}
}
