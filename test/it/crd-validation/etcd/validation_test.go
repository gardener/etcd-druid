package etcd

import (
	"context"
	"fmt"
	"os"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/it/assets"
	"github.com/gardener/etcd-druid/test/it/setup"
	"github.com/gardener/etcd-druid/test/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "etcd-validation-test"

var (
	itTestEnv setup.DruidTestEnvironment
)

func TestMain(m *testing.M) {
	var (
		itTestEnvCloser setup.DruidTestEnvCloser
		err             error
	)
	itTestEnv, itTestEnvCloser, err = setup.NewDruidTestEnvironment("etcd-validation", []string{assets.GetEtcdCrdPath()})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create integration test environment: %v\n", err)
		os.Exit(1)
	}

	// os.Exit() does not respect defer statements
	exitCode := m.Run()
	itTestEnvCloser()
	os.Exit(exitCode)
}

// ------------------------------ validation tests ------------------------------

// TestValidateGarbageCollectionPolicy tests the validation of `Spec.Backup.GarbageCollectionPolicy` field in the Etcd resource.
func TestValidateGarbageCollectionPolicy(t *testing.T) {
	tests := []struct {
		name                    string
		etcdName                string
		garbageCollectionPolicy string
		expectErr               bool
	}{
		{"valid garbage collection policy (Exponential)", "etcd1", "Exponential", false},
		{"valid garbage collection policy (LimitBased)", "etcd2", "LimitBased", false},
		{"invalid garbage collection policy", "etcd3", "Invalid", true},
		{"empty garbage collection policy", "etcd4", "", true},
	}

	g := NewWithT(t)
	testNs := utils.GenerateTestNamespaceName(t, testNamespacePrefix)
	t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
	g.Expect(itTestEnv.CreateTestNamespace(testNs)).To(Succeed())

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						GarbageCollectionPolicy: (*druidv1alpha1.GarbageCollectionPolicy)(&test.garbageCollectionPolicy),
					},
				},
			}
			cl := itTestEnv.GetClient()
			ctx := context.Background()

			// create etcd resource
			if test.expectErr {
				g.Expect(cl.Create(ctx, etcd)).NotTo(Succeed())
				return
			}
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())
		})
	}
}

// TODO: add one TestValidate... function for each field that needs validation
