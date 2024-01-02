// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
