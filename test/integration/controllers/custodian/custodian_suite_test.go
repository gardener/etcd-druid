// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package custodian

import (
	"context"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/controllers/custodian"
	"github.com/gardener/etcd-druid/test/integration/controllers/assets"
	"github.com/gardener/etcd-druid/test/integration/setup"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	k8sClient  client.Client
	intTestEnv *setup.IntegrationTestEnv
	namespace  string
)

const (
	testNamespacePrefix = "custodian-"
)

func TestCustodianController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"Custodian Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	crdPaths := []string{assets.GetEtcdCrdPath()}
	ctx, cancelFn := context.WithTimeout(context.Background(), time.Minute)
	defer cancelFn()
	intTestEnv = setup.NewIntegrationTestEnv(testNamespacePrefix, "custodian-int-tests", crdPaths)
	intTestEnv.RegisterReconcilers(func(mgr manager.Manager) {
		reconciler := custodian.NewReconciler(mgr, &custodian.Config{
			Workers:    5,
			SyncPeriod: 15 * time.Second,
			EtcdMember: custodian.EtcdMemberConfig{
				NotReadyThreshold: 1 * time.Minute,
				UnknownThreshold:  2 * time.Minute,
			},
		})
		Expect(reconciler.RegisterWithManager(ctx, mgr, true)).To(Succeed())
	}).StartManager()
	k8sClient = intTestEnv.K8sClient
	namespace = intTestEnv.TestNs.Name
})
