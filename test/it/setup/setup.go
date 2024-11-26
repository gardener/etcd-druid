// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"context"
	"os"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	eventsv1beta1 "k8s.io/api/events/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const envUseExistingK8SCluster = "USE_EXISTING_K8S_CLUSTER"

type AddToManagerFn func(mgr manager.Manager)

type ManagerRegisterer interface {
	RegisterReconciler(addToMgrFn AddToManagerFn)
	CreateManager(clientBuilder *testutils.TestClientBuilder) error
	StartManager() error
}

type DruidTestEnvironment interface {
	ManagerRegisterer
	GetClient() client.Client
	CreateTestNamespace(name string) error
	GetLogger() logr.Logger
	GetContext() context.Context
	GetEnvTestEnvironment() *envtest.Environment
}

type itTestEnv struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	mgr      manager.Manager
	client   client.Client
	config   *rest.Config
	scheme   *k8sruntime.Scheme
	testEnv  *envtest.Environment
	logger   logr.Logger
}

type DruidTestEnvCloser func()

func NewDruidTestEnvironment(loggerName string, crdDirectoryPaths []string) (DruidTestEnvironment, DruidTestEnvCloser, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	logger := ctrl.Log.WithName(loggerName)
	itEnv := &itTestEnv{
		ctx:      ctx,
		cancelFn: cancelFunc,
		logger:   logger,
	}
	if err := itEnv.prepareScheme(); err != nil {
		return nil, nil, err
	}
	if err := itEnv.startTestEnvironment(crdDirectoryPaths); err != nil {
		return nil, nil, err
	}
	return itEnv, func() {
		itEnv.cancelFn()
		if err := itEnv.testEnv.Stop(); err != nil {
			logger.Error(err, "failed to stop test environment")
		}
		logger.Info("stopped test environment")
	}, nil
}

func (t *itTestEnv) RegisterReconciler(addToMgrFn AddToManagerFn) {
	addToMgrFn(t.mgr)
}

func (t *itTestEnv) CreateManager(clientBuilder *testutils.TestClientBuilder) error {
	mgr, err := manager.New(t.config, manager.Options{
		Scheme: t.scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		NewClient: func(config *rest.Config, options client.Options) (client.Client, error) {
			options.Cache.DisableFor = []client.Object{
				&corev1.Event{},
				&eventsv1beta1.Event{},
				&eventsv1.Event{},
			}
			cl, err := client.New(config, options)
			if err != nil {
				return nil, err
			}
			testCl := clientBuilder.WithClient(cl).Build()
			return testCl, nil
		},
	})
	if err != nil {
		return err
	}
	t.mgr = mgr
	t.client = mgr.GetClient()
	ctrl.SetLogger(logr.Discard())
	return nil
}

func (t *itTestEnv) StartManager() (err error) {
	go func() {
		err = t.mgr.Start(t.ctx)
		if err != nil {
			t.logger.Error(err, "failed to start manager")
			return
		}
	}()
	return err
}

func (t *itTestEnv) GetClient() client.Client {
	return t.client
}

func (t *itTestEnv) CreateTestNamespace(name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return t.client.Create(t.ctx, ns)
}

func (t *itTestEnv) GetLogger() logr.Logger {
	return t.logger
}

func (t *itTestEnv) GetContext() context.Context {
	return t.ctx
}

func (t *itTestEnv) GetEnvTestEnvironment() *envtest.Environment {
	return t.testEnv
}

func (t *itTestEnv) prepareScheme() error {
	if err := druidv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		return err
	} else {
		t.scheme = scheme.Scheme
	}
	return nil
}

func (t *itTestEnv) startTestEnvironment(crdDirectoryPaths []string) error {
	testEnv := &envtest.Environment{
		Scheme:                t.scheme,
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     crdDirectoryPaths,
	}
	if useExistingK8SCluster() {
		testEnv.UseExistingCluster = ptr.To(true)
	}

	cfg, err := testEnv.Start()
	if err != nil {
		return err
	}
	t.config = cfg
	t.testEnv = testEnv
	return nil
}

// useExistingK8SCluster checks if the existing K8S cluster should be used for testing. If the environment variable
// USE_EXISTING_K8S_CLUSTER is set to true, the existing cluster will be used. Otherwise, a new k8s cluster via envtest
// will be created.
func useExistingK8SCluster() bool {
	useExistingCluster := os.Getenv(envUseExistingK8SCluster)
	return strings.ToLower(useExistingCluster) == "true"
}
