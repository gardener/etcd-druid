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

package setup

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type IntegrationTestEnv struct {
	ctx        context.Context
	K8sClient  client.Client
	RestConfig *rest.Config
	Logger     logr.Logger
	testEnv    *envtest.Environment
	mgr        manager.Manager
	TestNs     *corev1.Namespace
}

type AddToManager func(mgr manager.Manager)

// NewIntegrationTestEnv creates a new integration test environment. It will do the following:
// 1. It will set the scheme
// 2. It will create a kubebuilder envtest environment which in turn will provide rest.Config, k8s client.
// 3. Creates a test namespace that will be used by all the tests.
// 4. Creates a controller manager
// Sets up deferred cleanup of all the above resources which will be cleaned up when the test suit completes.
func NewIntegrationTestEnv(namespacePrefix string, loggerName string, crdDirectoryPaths []string) *IntegrationTestEnv {
	testEnv := &IntegrationTestEnv{ctx: context.Background()}
	testEnv.Logger = ctrl.Log.WithName(loggerName)

	druidScheme := prepareScheme()
	testEnv.createTestEnvironment(druidScheme, crdDirectoryPaths)
	testEnv.createTestNamespace(namespacePrefix)
	Expect(testEnv.createManager()).To(Succeed())

	return testEnv
}

// RegisterReconcilers allows consumers to register one or more reconcilers to the controller manager passed via the callback function AddToManager.
func (e *IntegrationTestEnv) RegisterReconcilers(addToMgrFn AddToManager) *IntegrationTestEnv {
	By("Register reconciler")
	addToMgrFn(e.mgr)
	return e
}

// StartManager starts controller manager.
func (e *IntegrationTestEnv) StartManager() *IntegrationTestEnv {
	By("Start manager")
	mgrCtx, mgrCancelFn := context.WithCancel(e.ctx)
	go func() {
		defer GinkgoRecover()
		Expect(e.mgr.Start(mgrCtx)).To(Succeed())
	}()
	DeferCleanup(func() {
		By("Stop manager")
		mgrCancelFn()
	})
	return e
}

func (e *IntegrationTestEnv) createTestEnvironment(scheme *k8sruntime.Scheme, crdDirectoryPaths []string) {
	By("Create test environment")
	testEnv := &envtest.Environment{
		Scheme:                scheme,
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     crdDirectoryPaths,
	}

	cfg, err := testEnv.Start()
	Expect(err).To(BeNil())

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).To(BeNil())

	e.testEnv = testEnv
	e.RestConfig = cfg
	e.K8sClient = k8sClient

	DeferCleanup(func() {
		By("Stopping test environment for integration test")
		Expect(testEnv.Stop()).To(Succeed())
	})
}

func (e *IntegrationTestEnv) createTestNamespace(namespacePrefix string) {
	By("Create test namespace")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namespacePrefix,
		},
	}
	err := e.K8sClient.Create(e.ctx, ns)
	Expect(err).To(BeNil())
	e.TestNs = ns
}

func (e *IntegrationTestEnv) createManager() error {
	By("Setup manager")
	uncachedObjects := []client.Object{
		&corev1.Event{},
		&eventsv1beta1.Event{},
		&eventsv1.Event{},
	}

	mgr, err := manager.New(e.RestConfig, manager.Options{
		MetricsBindAddress:    "0",
		ClientDisableCacheFor: uncachedObjects,
	})
	if err != nil {
		return err
	}
	e.mgr = mgr

	return nil
}

func prepareScheme() *k8sruntime.Scheme {
	Expect(druidv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	return scheme.Scheme
}
