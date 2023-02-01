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

package custodian

import (
	"context"
	"sync"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/custodian"
	"github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
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
	revertFunc func()
	testLog    = ctrl.Log.WithName("test")
)

func TestCustodianController(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(
		t,
		"Custodian Controller Suite",
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

	reconciler, err := custodian.NewReconciler(mgr, &custodian.Config{
		Workers:    5,
		SyncPeriod: 15 * time.Second,
		EtcdMember: custodian.EtcdMemberConfig{
			NotReadyThreshold: 1 * time.Minute,
			UnknownThreshold:  2 * time.Minute,
		},
	})
	Expect(err).NotTo(HaveOccurred())

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err = reconciler.AddToManager(ctx, mgr, true)
	Expect(err).NotTo(HaveOccurred())

	mgrCtx, mgrCancel = context.WithCancel(context.Background())
	mgrStopped = utils.StartManager(mgrCtx, mgr)
})

var _ = AfterSuite(func() {
	utils.StopManager(mgrCancel, mgrStopped, testEnv, revertFunc)
})
