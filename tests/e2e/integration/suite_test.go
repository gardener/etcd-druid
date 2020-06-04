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
	"flag"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druidcontroller "github.com/gardener/etcd-druid/controllers"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/utils/pointer"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	testEnv *envtest.Environment
	cfg *rest.Config
	k8sClient kubernetes.Interface
	mgr manager.Manager
	err error
	stopMgr chan struct{}
	mgrStopped *sync.WaitGroup
	s *runtime.Scheme
)

func init(){
	klog.InitFlags(nil)
	klog.SetOutput(GinkgoWriter)

	flag.Set("logtostderr", "false")
	flag.Set("alsologtostderr", "false")
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{envtest.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	defer WithWd("../../..")()
	flag.Parse()
	s = scheme.Scheme
	testEnv = &envtest.Environment{
		Config: ctrl.GetConfigOrDie(),
		UseExistingCluster: pointer.BoolPtr(true),
		CRDDirectoryPaths: []string{filepath.Join( "config", "crd", "bases")},
	}

	klog.Info("Starting tests")
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = druidv1alpha1.AddToScheme(s)
	Expect(err).ToNot(HaveOccurred())

	err = apiextensions.AddToScheme(s)
	Expect(err).ToNot(HaveOccurred())

	k8sClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error creating k8s clientset: %s", err.Error())
	}

	Expect(cfg).ToNot(BeNil())
	mgr, err = manager.New(cfg, manager.Options{
		Scheme: s,
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	er, err := druidcontroller.NewEtcdReconcilerWithImageVector(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = er.SetupWithManager(mgr, 1, true)
	Expect(err).NotTo(HaveOccurred())

	stopMgr, mgrStopped = startTestManager(mgr)

	close(done)
}, 60)

var _ = AfterSuite(func() {
	klog.Infof("After Suite...")
	etcdCrd := apiextensions.CustomResourceDefinition{}
	err := mgr.GetClient().Get(context.TODO(), types.NamespacedName{Name: "etcds.druid.gardener.cloud"}, &etcdCrd)
	Expect(err).NotTo(HaveOccurred())
	err = mgr.GetClient().Delete(context.TODO(), &etcdCrd)
	Expect(err).NotTo(HaveOccurred())
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
