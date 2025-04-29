// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/internal/controller/compaction"
	"github.com/gardener/etcd-druid/test/integration/controllers/assets"
	"github.com/gardener/etcd-druid/test/integration/setup"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	intTestEnv *setup.IntegrationTestEnv
	k8sClient  client.Client
	namespace  string
)

const (
	testNamespacePrefix = "compaction-"
)

func TestCompactionController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"Compaction Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	crdPaths := []string{assets.GetEtcdCrdPath()}
	imageVector := assets.CreateImageVector()

	intTestEnv = setup.NewIntegrationTestEnv(testNamespacePrefix, "compaction-int-tests", crdPaths)
	intTestEnv.RegisterReconcilers(func(mgr manager.Manager) {
		reconciler := compaction.NewReconcilerWithImageVector(mgr, configv1alpha1.CompactionControllerConfiguration{
			Enabled:                   true,
			ConcurrentSyncs:           ptr.To(5),
			EventsThreshold:           100,
			ActiveDeadlineDuration:    metav1.Duration{Duration: 2 * time.Minute},
			MetricsScrapeWaitDuration: metav1.Duration{Duration: 60 * time.Second},
		}, imageVector)
		Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	}).StartManager()
	k8sClient = intTestEnv.K8sClient
	namespace = intTestEnv.TestNs.Name
})
