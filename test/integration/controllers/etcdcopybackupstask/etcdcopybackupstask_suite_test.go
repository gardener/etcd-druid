// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdcopybackupstask

import (
	"github.com/gardener/etcd-druid/internal/utils/imagevector"
	"testing"

	"github.com/gardener/etcd-druid/internal/controller/etcdcopybackupstask"
	"github.com/gardener/etcd-druid/test/integration/controllers/assets"
	"github.com/gardener/etcd-druid/test/integration/setup"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	k8sClient   client.Client
	intTestEnv  *setup.IntegrationTestEnv
	imageVector imagevector.ImageVector
	namespace   string
)

const (
	testNamespacePrefix = "etcdcopybackupstask-"
)

func TestEtcdCopyBackupsTaskController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"EtcdCopyBackupsTask Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	crdPaths := []string{assets.GetEtcdCopyBackupsTaskCrdPath()}
	imageVector = assets.CreateImageVector()
	intTestEnv = setup.NewIntegrationTestEnv(testNamespacePrefix, "etcdcopybackupstask-int-tests", crdPaths)
	intTestEnv.RegisterReconcilers(func(mgr manager.Manager) {
		reconciler := etcdcopybackupstask.NewReconcilerWithImageVector(mgr, &etcdcopybackupstask.Config{
			Workers: 5,
		}, imageVector)
		Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	}).StartManager()
	k8sClient = intTestEnv.K8sClient
	namespace = intTestEnv.TestNs.Name
})
