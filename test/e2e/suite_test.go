// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
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

package e2e

import (
	"context"
	"path"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	singleNodeEtcdTimeout = time.Minute
	multiNodeEtcdTimeout  = time.Minute * 5

	pollingInterval   = time.Second * 2
	envSourcePath     = "SOURCE_PATH"
	envKubeconfigPath = "KUBECONFIG"
	etcdNamespace     = "shoot"

	certsBasePath = "test/e2e/resources/tls"

	envStorageContainer = "TEST_ID"
)

var (
	logger         = zap.New(zap.WriteTo(GinkgoWriter))
	typedClient    *kubernetes.Clientset
	cl             client.Client
	sourcePath     string
	kubeconfigPath string

	storePrefix = "etcd-main"

	etcdKeyPrefix   = "foo"
	etcdValuePrefix = "bar"

	providers []TestProvider
	err       error
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "e2e Suite")
}

var _ = BeforeSuite(func() {
	ctx := context.Background()

	providers, err = getProviders()
	Expect(err).ToNot(HaveOccurred())
	Expect(len(providers)).To(BeNumerically(">", 0))

	sourcePath = getEnvOrFallback(envSourcePath, ".")
	kubeconfigPath = getEnvAndExpectNoError(envKubeconfigPath)

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	logger.V(1).Info("setting up k8s client", "KUBECONFIG", kubeconfigPath)
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

	// deploy TLS secrets
	certsPath := path.Join(sourcePath, certsBasePath)
	Expect(buildAndDeployTLSSecrets(ctx, cl, logger, etcdNamespace, certsPath, providers)).To(Succeed())
})

var _ = AfterSuite(func() {
	ctx := context.Background()

	kubeconfigPath, err := getEnvOrError(envKubeconfigPath)
	Expect(err).NotTo(HaveOccurred())

	logger.V(1).Info("setting up k8s client using", " KUBECONFIG", kubeconfigPath)
	cl, err := getKubernetesClient(kubeconfigPath)
	Expect(err).ShouldNot(HaveOccurred())

	namespaceLogger := logger.WithValues("namespace", etcdNamespace)

	namespaceLogger.Info("deleting namespace")
	err = cl.Delete(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdNamespace,
		},
	})
	err = client.IgnoreNotFound(err)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		return cl.Get(ctx, client.ObjectKey{Name: etcdNamespace}, &corev1.Namespace{})
	}, time.Minute*2, pollingInterval).Should(matchers.BeNotFoundError())
})
