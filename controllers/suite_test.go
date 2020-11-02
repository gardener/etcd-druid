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
	"path/filepath"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var mgr manager.Manager
var recFn reconcile.Reconciler
var requests chan reconcile.Request
var stopMgr chan struct{}
var mgrStopped *sync.WaitGroup
var (
	testLog = ctrl.Log.WithName("test")
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	var err error
	//logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))
	ctrl.SetLogger(zap.Logger(true))
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

	Expect(cfg).ToNot(BeNil())
	mgr, err = manager.New(cfg, manager.Options{
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	Expect(cfg).ToNot(BeNil())
	mgr, err = manager.New(cfg, manager.Options{})

	Expect(err).NotTo(HaveOccurred())
	defer WithWd("..")()
	er, err := NewEtcdReconcilerWithImageVector(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = er.SetupWithManager(mgr, 1, true)
	Expect(err).NotTo(HaveOccurred())

	stopMgr, mgrStopped = startTestManager(mgr)

	close(done)
}, 60)

var _ = AfterSuite(func() {
	close(stopMgr)
	mgrStopped.Wait()

})

func startTestManager(mgr manager.Manager) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	go func() {
		wg.Add(1)
		Expect(mgr.Start(stop)).NotTo(HaveOccurred())
		wg.Done()
	}()
	return stop, wg
}

func SetupWithManager(mgr ctrl.Manager, r reconcile.Reconciler) error {
	return ctrl.NewControllerManagedBy(mgr).WithOptions(controller.Options{
		MaxConcurrentReconciles: 10,
	}).
		For(&druidv1alpha1.Etcd{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
