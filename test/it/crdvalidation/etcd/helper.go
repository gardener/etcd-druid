// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/it/setup"
	"github.com/gardener/etcd-druid/test/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "etcd-validation-test"

var (
	itTestEnv          setup.DruidTestEnvironment
	k8sVersionAbove129 bool
)

// common function used by all the test cases to create an etcd resource based on the individual test case's specifications.
func validateEtcdCreation(g *WithT, etcd *druidv1alpha1.Etcd, expectErr bool) {
	cl := itTestEnv.GetClient()
	ctx := context.Background()

	if expectErr {
		g.Expect(cl.Create(ctx, etcd)).NotTo(Succeed())
		return
	}
	g.Expect(cl.Create(ctx, etcd)).To(Succeed())
}

func validateEtcdUpdate(g *WithT, etcd *druidv1alpha1.Etcd, expectErr bool, ctx context.Context, cl client.Client) {
	updateErr := cl.Update(ctx, etcd)
	if expectErr {
		g.Expect(updateErr).ToNot(BeNil())
	} else {
		g.Expect(updateErr).To(BeNil())
	}
}

var cronFieldTestCases = []struct {
	name      string
	etcdName  string
	value     string
	expectErr bool
}{
	{
		name:      "Valid cron expression #1",
		etcdName:  "etcd-valid-1",
		value:     "* * * * *",
		expectErr: false,
	},
	{
		name:      "Valid cron expression #2",
		etcdName:  "etcd-valid-2",
		value:     "0 */24 * * *",
		expectErr: false,
	},
	{
		name:      "Valid cron expression #3",
		etcdName:  "etcd-valid-3",
		value:     "*/57 23 30/31 6-12 2/7",
		expectErr: false,
	},
	{
		name:      "Valid cron expression #4",
		etcdName:  "etcd-valid-4",
		value:     "0 21-22 30/31 11-12 2/7",
		expectErr: false,
	},
	{
		name:      "Invalid cron expression #1",
		etcdName:  "etcd-invalid-1",
		value:     "61 23 * * *", // hours >23
		expectErr: true,
	},
	{
		name:      "Invalid cron expression #2",
		etcdName:  "etcd-invalid-2",
		value:     "3 */24 12 *", // missing field
		expectErr: true,
	},
}

var durationFieldTestCases = []struct {
	name      string
	etcdName  string
	value     string
	expectErr bool
}{
	{
		name:      "valid duration #1",
		etcdName:  "etcd-valid-1",
		value:     "1h2m3s",
		expectErr: false,
	},
	{
		name:      "valid duration #2",
		etcdName:  "etcd-valid-2",
		value:     "3m735s",
		expectErr: false,
	},
	{
		name:      "Invalid duration #1",
		etcdName:  "etcd-invalid-1",
		value:     "10h5m6m",
		expectErr: true,
	},
	{
		name:      "invalid duration #2",
		etcdName:  "etcd-invalid-2",
		value:     "hms",
		expectErr: true,
	},
	{
		name:      "invalid duration #3",
		etcdName:  "etcd-invalid-3",
		value:     "-10h2m3s",
		expectErr: true,
	},
}

// Takes in string as input and parses it to the type expected by the duration fields ie metav1.Duration
func parseDuration(value string, expectErr bool, t *testing.T) (*metav1.Duration, error) {
	if value == "" {
		return nil, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return nil, err
	}
	return &metav1.Duration{Duration: duration}, nil
}

// helper function to handle the parsing of the duration string value.
func validateDuration(t *testing.T, value string, expectErr bool) (*metav1.Duration, bool) {
	duration, err := parseDuration(value, expectErr, t)
	if expectErr {
		return nil, false
	} else if err != nil {
		return nil, false
	}
	return duration, true
}

// skips the test case in case the kubernetes version is less than 1.29
func skipCELTestsForOlderK8sVersions(t *testing.T) {
	if !k8sVersionAbove129 {
		t.Skip()
	}
}

// initial setup for setting up namespace and test environment
func setupTestEnvironment(t *testing.T) (string, *WithT) {
	g := NewWithT(t)
	testNs := utils.GenerateTestNamespaceName(t, testNamespacePrefix)

	t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
	t.Log("Setting up Client")

	g.Expect(itTestEnv.CreateManager(utils.NewTestClientBuilder())).To(Succeed())
	g.Expect(itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
	g.Expect(itTestEnv.StartManager()).To(Succeed())

	return testNs, g
}
