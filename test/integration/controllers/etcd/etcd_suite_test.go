// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"testing"

	"github.com/gardener/etcd-druid/controllers/etcd"
	"github.com/gardener/etcd-druid/pkg/features"
	"github.com/gardener/etcd-druid/test/integration/controllers/assets"
	"github.com/gardener/etcd-druid/test/integration/setup"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	intTestEnv *setup.IntegrationTestEnv
	k8sClient  client.Client
	restConfig *rest.Config
	namespace  string
)

const (
	testNamespacePrefix = "etcd-"
)

func TestEtcdController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"Etcd Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	crdPaths := []string{assets.GetEtcdCrdPath()}
	imageVector := assets.CreateImageVector()

	intTestEnv = setup.NewIntegrationTestEnv(testNamespacePrefix, "compaction-int-tests", crdPaths)
	intTestEnv.RegisterReconcilers(func(mgr manager.Manager) {
		reconciler, err := etcd.NewReconcilerWithImageVector(mgr, &etcd.Config{
			Workers:                            5,
			DisableEtcdServiceAccountAutomount: false,
			FeatureGates: map[featuregate.Feature]bool{
				features.UseEtcdWrapper: true,
			},
		}, imageVector)
		Expect(err).To(BeNil())
		Expect(reconciler.RegisterWithManager(mgr, true)).To(Succeed())
	}).StartManager()
	k8sClient = intTestEnv.K8sClient
	restConfig = intTestEnv.RestConfig
	namespace = intTestEnv.TestNs.Name
})
