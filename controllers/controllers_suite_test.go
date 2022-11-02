// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	controllersconfig "github.com/gardener/etcd-druid/controllers/config"

	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	mgrCtx     context.Context
	mgrCancel  context.CancelFunc
	cfg        *rest.Config
	k8sClient  client.Client
	testEnv    *envtest.Environment
	mgr        manager.Manager
	mgrStopped *sync.WaitGroup

	activeDeadlineDuration time.Duration

	revertFns []func()

	testLog = ctrl.Log.WithName("test")
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	mgrCtx, mgrCancel = context.WithCancel(context.Background())
	var err error
	//logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))
	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}

	testLog.Info("Starting tests")
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = druidv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	revertFns = []func(){
		test.WithVar(&DefaultTimeout, 20*time.Second),
		WithWd(".."),
	}

	Expect(cfg).ToNot(BeNil())
	mgr, err = manager.New(cfg, manager.Options{
		MetricsBindAddress:    "0",
		ClientDisableCacheFor: UncachedObjectList,
	})
	Expect(err).NotTo(HaveOccurred())

	er, err := NewEtcdReconcilerWithImageVector(mgr, false)
	Expect(err).NotTo(HaveOccurred())

	err = er.SetupWithManager(mgr, 5, true)
	Expect(err).NotTo(HaveOccurred())

	secret := NewSecret(mgr)

	err = secret.SetupWithManager(mgr, 5)
	Expect(err).NotTo(HaveOccurred())

	custodian := NewEtcdCustodian(mgr, controllersconfig.EtcdCustodianController{
		EtcdMember: controllersconfig.EtcdMemberConfig{
			EtcdMemberNotReadyThreshold: 1 * time.Minute,
		},
	})

	err = custodian.SetupWithManager(mgrCtx, mgr, 5, true)
	Expect(err).NotTo(HaveOccurred())

	etcdCopyBackupsTaskReconciler, err := NewEtcdCopyBackupsTaskReconcilerWithImageVector(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = etcdCopyBackupsTaskReconciler.SetupWithManager(mgr, 5)
	Expect(err).NotTo(HaveOccurred())

	activeDeadlineDuration, err = time.ParseDuration("2m")
	Expect(err).NotTo(HaveOccurred())

	lc, err := NewCompactionLeaseControllerWithImageVector(mgr, controllersconfig.CompactionLeaseConfig{
		CompactionEnabled:      true,
		EventsThreshold:        1000000,
		ActiveDeadlineDuration: activeDeadlineDuration,
	})
	Expect(err).NotTo(HaveOccurred())

	err = lc.SetupWithManager(mgr, 1)
	Expect(err).NotTo(HaveOccurred())

	mgrStopped = startTestManager(mgrCtx, mgr)

	close(done)
}, 60)

var _ = AfterSuite(func() {
	mgrCancel()
	mgrStopped.Wait()
	Expect(testEnv.Stop()).To(Succeed())
	for _, f := range revertFns {
		f()
	}
})

func startTestManager(ctx context.Context, mgr manager.Manager) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
		wg.Done()
	}()
	syncCtx, syncCancel := context.WithTimeout(ctx, 1*time.Minute)
	defer syncCancel()
	mgr.GetCache().WaitForCacheSync(syncCtx)
	return wg
}
