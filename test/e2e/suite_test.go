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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/gardener/pkg/utils/test/matchers"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	timeout           = time.Minute * 1
	pollingInterval   = time.Second * 2
	envSourcePath     = "SOURCE_PATH"
	envKubeconfigPath = "KUBECONFIG"
	etcdNamespace     = "shoot"

	certsBasePath = "test/e2e/resources/certs"

	envStorageContainer = "STORAGE_CONTAINER"
)

var (
	logger                  = zap.New(zap.WriteTo(GinkgoWriter))
	err                     error
	typedClient             *kubernetes.Clientset
	sourcePath              string
	kubeconfigPath          string
	storageContainer        string
	providers               map[string]Provider
	storageProvider         *v1alpha1.StorageProvider
	S3AccessKeyID           string
	S3SecretAccessKey       string
	S3Region                string
	storePrefix             = "etcd-main"
	etcdConfigMapVolumeName = "etcd-config-file"

	etcdKeyPrefix   = "foo"
	etcdValuePrefix = "bar"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "e2e Suite")
}

var _ = BeforeSuite(func() {
	providers, err := getProviders()
	Expect(err).ToNot(HaveOccurred())
	Expect(len(providers)).To(BeNumerically(">", 0))
	for name := range providers {
		logger.Info("Will run tests for provider", "providerName", name)
	}

	sourcePath = getEnvOrFallback(envSourcePath, ".")
	kubeconfigPath = getEnvAndExpectNoError(envKubeconfigPath)

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	logger.V(1).Info("setting up k8s client", "KUBECONFIG", kubeconfigPath)
	typedClient, err = getKubernetesTypedClient(kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())
	namespacesClient := typedClient.CoreV1().Namespaces()

	logger.Info("creating namespace", "namespace", etcdNamespace)
	ns, err := namespacesClient.Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdNamespace,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		_, err := namespacesClient.Get(context.TODO(), ns.Name, metav1.GetOptions{})
		return err
	}, timeout, pollingInterval).Should(BeNil())
	logger.Info("created namespace", "namespace", client.ObjectKeyFromObject(ns))

	// deploy TLS secrets
	secretsClient := typedClient.CoreV1().Secrets(etcdNamespace)

	certsPath := path.Join(sourcePath, certsBasePath)
	_, err = buildAndDeployTLSSecrets(logger, certsPath, providers, secretsClient)
	Expect(err).NotTo(HaveOccurred())

	// initialize object storage providers
	storageContainer = getEnvAndExpectNoError(envStorageContainer)

	for providerName, provider := range providers {
		if providerName == providerLocal {
			continue
		}

		snapstoreProvider := provider.StorageProvider
		store, err := getSnapstore(snapstoreProvider, storageContainer, storePrefix)
		Expect(err).ShouldNot(HaveOccurred())

		// purge any existing backups in bucket
		err = purgeSnapstore(store)
		Expect(err).ShouldNot(HaveOccurred())
	}

	// build and deploy etcd-backup secrets
	secrets, err := deployBackupSecrets(logger, secretsClient, providers, etcdNamespace, storageContainer)
	Expect(err).NotTo(HaveOccurred())
	for _, secret := range secrets {
		Eventually(func() error {
			_, err := secretsClient.Get(context.TODO(), secret.Name, metav1.GetOptions{})
			return err
		}, timeout, pollingInterval).Should(BeNil())
		logger.Info("created secret", "secret", client.ObjectKeyFromObject(secret))
	}
})

var _ = AfterSuite(func() {
	kubeconfigPath, err := getEnvOrError(envKubeconfigPath)
	Expect(err).NotTo(HaveOccurred())

	logger.V(1).Info("setting up k8s client using", " KUBECONFIG", kubeconfigPath)
	typedClient, err = getKubernetesTypedClient(kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())
	namespacesClient := typedClient.CoreV1().Namespaces()

	namespaceLogger := logger.WithValues("namespace", etcdNamespace)

	namespaceLogger.Info("deleting namespace")
	namespacesClient = typedClient.CoreV1().Namespaces()
	err = namespacesClient.Delete(context.TODO(), etcdNamespace, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		namespaceLogger.Info("namespace does not exist")
	} else {
		Expect(err).NotTo(HaveOccurred())
	}
	Eventually(func() error {
		_, err = namespacesClient.Get(context.TODO(), etcdNamespace, metav1.GetOptions{})
		return err
	}, timeout*2, pollingInterval).Should(matchers.BeNotFoundError())
	namespaceLogger.Info("deleted namespace")
})

func getEnvAndExpectNoError(key string) string {
	val, err := getEnvOrError(key)
	Expect(err).NotTo(HaveOccurred())
	return val
}
