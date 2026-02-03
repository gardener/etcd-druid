// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/it/setup"
	"github.com/gardener/etcd-druid/test/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "etcdopstask-validation-test"

var (
	itTestEnv          setup.DruidTestEnvironment
	k8sVersionAbove129 bool
)

// setupTestEnvironment creates namespace and test environment
func setupTestEnvironment(t *testing.T) (string, *WithT) {
	g := NewWithT(t)
	testNs := utils.GenerateTestNamespaceName(t, testNamespacePrefix, 8)

	t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
	t.Log("Setting up Client")

	g.Expect(itTestEnv.CreateManager(utils.NewTestClientBuilder())).To(Succeed())
	g.Expect(itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
	g.Expect(itTestEnv.StartManager()).To(Succeed())

	return testNs, g
}

// Helper function to validate EtcdOpsTask creation
func validateEtcdOpsTaskCreation(g *WithT, task *druidv1alpha1.EtcdOpsTask, expectErr bool) {
	cl := itTestEnv.GetClient()
	ctx := context.Background()

	err := cl.Create(ctx, task)
	if expectErr {
		g.Expect(err).To(HaveOccurred())
	} else {
		g.Expect(err).ToNot(HaveOccurred())
	}
}

// Helper function to validate EtcdOpsTask update
func validateEtcdOpsTaskUpdate(ctx context.Context, g *WithT, task *druidv1alpha1.EtcdOpsTask, expectErr bool, cl client.Client) {
	err := cl.Update(ctx, task)
	if expectErr {
		g.Expect(err).To(HaveOccurred())
	} else {
		g.Expect(err).ToNot(HaveOccurred())
	}
}
