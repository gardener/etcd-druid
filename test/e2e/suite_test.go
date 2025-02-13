// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"path"
	"sync"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	singleNodeEtcdTimeout = time.Minute * 20
	multiNodeEtcdTimeout  = time.Minute * 20

	pollingInterval   = time.Second * 2
	envSourcePath     = "SOURCE_PATH"
	envKubeconfigPath = "KUBECONFIG"
	etcdNamespace     = "shoot"

	certsBasePath = "test/e2e/resources/tls"

	envStorageContainer = "TEST_ID"
)

var once sync.Once

var (
	logger         = zap.New(zap.WriteTo(GinkgoWriter))
	typedClient    *kubernetes.Clientset
	cl             client.Client
	sourcePath     string
	kubeconfigPath string

	storePrefix = "etcd-test"

	etcdKeyPrefix   = "foo"
	etcdValuePrefix = "bar"

	providers []TestProvider
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "e2e Suite")
}

var _ = BeforeSuite(func() {
	var err error
	ctx := context.Background()

	providers, err = getProviders()
	Expect(err).ToNot(HaveOccurred())
	Expect(len(providers)).To(BeNumerically(">", 0))

	sourcePath = getEnvOrFallback(envSourcePath, ".")
	kubeconfigPath = getEnvAndExpectNoError(envKubeconfigPath)

	err = druidv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	logger.V(1).Info("setting up k8s client", "KUBECONFIG", kubeconfigPath)
	log.SetLogger(logr.Discard())
	cl, err = getKubernetesClient(kubeconfigPath)
	Expect(err).ShouldNot(HaveOccurred())

	typedClient, err = getKubernetesTypedClient(kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())

	logger.Info("creating namespace", "namespace", etcdNamespace)
	err = cl.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdNamespace,
		},
	})

	if apierrors.IsAlreadyExists(err) {
		err = nil
	}
	Expect(err).NotTo(HaveOccurred())

	certsPath := path.Join(sourcePath, certsBasePath)
	err = buildAndDeployTLSSecrets(ctx, cl, logger, etcdNamespace, certsPath, providers)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).To(Succeed())
	}

	// Deploy backup secrets
	storageContainer := getEnvAndExpectNoError(envStorageContainer)
	for _, provider := range providers {
		err = deployBackupSecret(ctx, cl, logger, provider, etcdNamespace, storageContainer)
		if !apierrors.IsAlreadyExists(err) {
			Expect(err).To(Succeed())
		}
	}
})

var _ = SynchronizedAfterSuite(func() {

}, func() {
	ctx := context.Background()

	// Ensure that the KUBECONFIG path is properly set
	kubeconfigPath, err := getEnvOrError(envKubeconfigPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to get KUBECONFIG path")

	logger.V(1).Info("Setting up Kubernetes client", "KUBECONFIG", kubeconfigPath)
	cl, err := getKubernetesClient(kubeconfigPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to set up Kubernetes client")

	namespaceLogger := logger.WithValues("namespace", etcdNamespace)
	namespaceLogger.Info("Checking for Etcd resources before deleting namespace", "namespace", etcdNamespace)

	var etcds druidv1alpha1.EtcdList
	// List all Etcd resources in the specified namespace
	err = cl.List(ctx, &etcds, client.InNamespace(etcdNamespace))
	Expect(err).NotTo(HaveOccurred(), "Failed to list Etcd resources")

	// Skip namespace deletion if there are still Etcd resources present
	if len(etcds.Items) > 0 {
		namespaceLogger.Info("Skipping namespace deletion; Etcd resources still present", "count", len(etcds.Items))
		return
	}

	// Proceed with namespace deletion if no Etcd resources are found
	namespaceLogger.Info("No Etcd resources found; proceeding with namespace deletion", "namespace", etcdNamespace)
	err = cl.Delete(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdNamespace,
		},
	})
	err = client.IgnoreNotFound(err)
	Expect(err).NotTo(HaveOccurred(), "Failed to delete namespace")

	// Verify that the namespace is indeed deleted
	Eventually(func() error {
		var ns corev1.Namespace
		return cl.Get(ctx, client.ObjectKey{Name: etcdNamespace}, &ns)
	}, 2*time.Minute, pollingInterval).Should(testutils.BeNotFoundError(), "Namespace still exists after deletion attempt")
})
