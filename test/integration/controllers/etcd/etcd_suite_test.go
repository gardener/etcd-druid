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
	"context"
	"sync"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/etcd"
	"github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testEnv    *envtest.Environment
	mgrCtx     context.Context
	mgrCancel  context.CancelFunc
	mgrStopped *sync.WaitGroup
	k8sClient  client.Client
	restConfig *rest.Config
	revertFunc func()
	testLog    = ctrl.Log.WithName("test")
)

func TestEtcdController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"Etcd Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	var err error

	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testLog.Info("Setting up test environment")
	testEnv, err = utils.SetupTestEnvironment(4)
	Expect(err).ToNot(HaveOccurred())

	err = druidv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(testEnv.Config, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	revertFunc = utils.SwitchDirectory("../../../..")

	mgr, err := utils.GetManager(testEnv.Config)
	Expect(err).NotTo(HaveOccurred())
	restConfig = mgr.GetConfig()

	reconciler, err := etcd.NewReconciler(mgr, &etcd.Config{
		Workers:                            5,
		DisableEtcdServiceAccountAutomount: false,
	})
	Expect(err).NotTo(HaveOccurred())

	err = reconciler.AddToManager(mgr, true)
	Expect(err).NotTo(HaveOccurred())

	mgrCtx, mgrCancel = context.WithCancel(context.Background())
	mgrStopped = utils.StartManager(mgrCtx, mgr)
})

var _ = AfterSuite(func() {
	utils.StopManager(mgrCancel, mgrStopped, testEnv, revertFunc)
})
