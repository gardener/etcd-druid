package setup

import (
	"context"
	"time"

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

//func NewDefaultIntegrationTestEnv(loggerName string) *IntegrationTestEnv {
//	return NewIntegrationTestEnv(loggerName, nil)
//}

func NewIntegrationTestEnv(namespacePrefix string, loggerName string, crdDirectoryPaths []string) *IntegrationTestEnv {
	testEnv := &IntegrationTestEnv{ctx: context.Background()}

	testEnv.Logger = ctrl.Log.WithName(loggerName)
	//mgrCtx, mgrCancelFn := context.WithCancel(context.Background())
	//testEnv.mgrCtx = mgrCtx
	//testEnv.mgrCancelFn = mgrCancelFn

	druidScheme := prepareScheme()
	testEnv.createTestEnvironment(druidScheme, crdDirectoryPaths)
	testEnv.createTestNamespace(namespacePrefix)
	Expect(testEnv.createManager()).To(Succeed())

	return testEnv
}

func (e *IntegrationTestEnv) RegisterReconcilers(addToMgrFn AddToManager) *IntegrationTestEnv {
	By("Register reconciler")
	addToMgrFn(e.mgr)
	return e
}

func (e *IntegrationTestEnv) StartManager(timeout time.Duration) *IntegrationTestEnv {
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
	//syncCtx, syncCancelFn := context.WithTimeout(e.mgrCtx, timeout)
	//defer syncCancelFn()
	//e.mgr.GetCache().WaitForCacheSync(syncCtx)
	return e
}

//func (e *IntegrationTestEnv) Close() {
//	if e.mgrCancelFn != nil {
//		e.mgrCancelFn()
//	}
//	if e.mgrStopWg != nil {
//		e.mgrStopWg.Wait()
//	}
//	if e.testEnv != nil {
//		if err := e.testEnv.Stop(); err != nil {
//			e.Logger.Error(err, "failed to cleanly stop env-test")
//		}
//	}
//}

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
		By("Stopping test environment for etcdcopybackupstask integration test")
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
