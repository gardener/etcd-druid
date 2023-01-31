package etcd

import (
	"context"
	"sync"
	"testing"

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
	testEnv, err = utils.SetupTestEnvironment()
	Expect(err).ToNot(HaveOccurred())

	k8sClient, err := client.New(testEnv.Config, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	revertFunc = utils.SwitchDirectory("../..")

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
