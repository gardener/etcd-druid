package setup

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
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
)

type AddToManagerFn func(mgr manager.Manager)

type IntegrationTestEnv interface {
	RegisterReconciler(addToMgrFn AddToManagerFn)
	StartManager()
	GetClient() client.Client
	GetConfig() *rest.Config
	CreateTestNamespace(name string)
	GetLogger() logr.Logger
}

type itTestEnv struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	g        *WithT
	mgr      manager.Manager
	client   client.Client
	config   *rest.Config
	testEnv  *envtest.Environment
	logger   logr.Logger
}

type IntegrationTestEnvCloser func()

func NewIntegrationTestEnv(t *testing.T, loggerName string, crdDirectoryPaths []string) (IntegrationTestEnv, IntegrationTestEnvCloser) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	itEnv := &itTestEnv{
		ctx:      ctx,
		cancelFn: cancelFunc,
		g:        NewWithT(t),
		logger:   ctrl.Log.WithName(loggerName),
	}
	druidScheme := itEnv.prepareScheme()
	itEnv.createTestEnvironment(druidScheme, crdDirectoryPaths)
	itEnv.createManager()
	return itEnv, func() {
		itEnv.cancelFn()
		itEnv.g.Expect(itEnv.testEnv.Stop()).To(Succeed())
	}
}

func (t *itTestEnv) RegisterReconciler(addToMgrFn AddToManagerFn) {
	addToMgrFn(t.mgr)
}

func (t *itTestEnv) StartManager() {
	go func() {
		t.g.Expect(t.mgr.Start(t.ctx)).To(Succeed())
	}()
}

func (t *itTestEnv) GetClient() client.Client {
	return t.client
}

func (t *itTestEnv) GetConfig() *rest.Config {
	return t.config
}

func (t *itTestEnv) CreateTestNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	t.g.Expect(t.client.Create(t.ctx, ns)).To(Succeed())
}

func (t *itTestEnv) GetLogger() logr.Logger {
	return t.logger
}

func (t *itTestEnv) prepareScheme() *k8sruntime.Scheme {
	t.g.Expect(druidv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	return scheme.Scheme
}

func (t *itTestEnv) createTestEnvironment(scheme *k8sruntime.Scheme, crdDirectoryPaths []string) {
	testEnv := &envtest.Environment{
		Scheme:                scheme,
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     crdDirectoryPaths,
	}

	cfg, err := testEnv.Start()
	t.g.Expect(err).ToNot(HaveOccurred())
	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	t.g.Expect(err).ToNot(HaveOccurred())
	t.config = cfg
	t.testEnv = testEnv
	t.client = cl
}

func (t *itTestEnv) createManager() {
	mgr, err := manager.New(t.config, manager.Options{
		MetricsBindAddress: "0",
		ClientDisableCacheFor: []client.Object{
			&corev1.Event{},
			&eventsv1beta1.Event{},
			&eventsv1.Event{},
		},
	})
	t.g.Expect(err).ToNot(HaveOccurred())
	t.mgr = mgr
}
