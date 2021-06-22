// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
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

package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"path"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apiextensionv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
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
	// etcdCRDBasePath    = "config/crd/bases/druid.gardener.cloud_etcds.yaml"
	etcdCRDBasePath    = "test/integration/resources/etcd/crd.yaml"
	druidChartBasePath = "charts/druid"
	druidReleaseName   = "etcd-druid"
	druidNamespace     = "garden"
	druidImage         = "eu.gcr.io/gardener-project/gardener/etcd-druid"
	envDruidVersion    = "DRUID_VERSION"
	etcdNamespace      = "shoot"

	etcdCASecretFile     = "ca-etcd.yaml"
	etcdClientSecretFile = "etcd-client-tls.yaml"
	etcdServerSecretFile = "etcd-server-cert.yaml"

	certsBasePath = "test/integration/resources/certs"

	envStorageContainer = "STORAGE_CONTAINER"
)

var (
	logger                  = logrus.New()
	err                     error
	typedClient             *kubernetes.Clientset
	sourcePath              string
	kubeconfigPath          string
	etcdCRDPath             string
	etcdCRDName             = "etcds.druid.gardener.cloud"
	druidChartPath          string
	druidVersion            string
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
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	providers := getProviders()
	Expect(len(providers)).To(BeNumerically(">", 0))
	for p := range providers {
		logger.Infof("Will run tests for provider %s", p)
	}

	sourcePath = getEnvOrFallback(envSourcePath, ".")
	kubeconfigPath = getEnvAndExpectNoError(envKubeconfigPath)
	druidVersion = getEnvAndExpectNoError(envDruidVersion)

	etcdCRDPath = path.Join(sourcePath, etcdCRDBasePath)
	etcdCRD, err := ioutil.ReadFile(etcdCRDPath)
	Expect(err).NotTo(HaveOccurred())

	decode = scheme.Codecs.UniversalDeserializer().Decode
	obj, _, err := decode(etcdCRD, nil, nil)
	Expect(err).NotTo(HaveOccurred())
	crdFromYAML := obj.(*apiextensionv1beta1.CustomResourceDefinition)

	logger.Printf("setting up k8s apiextension client using KUBECONFIG=%s", kubeconfigPath)
	apiExtensionClient, err := getKubernetesAPIExtensionClient(logger, kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())
	crdsClient := apiExtensionClient.ApiextensionsV1beta1().CustomResourceDefinitions()

	logger.Infof("deleting existing crd %s if exists", crdFromYAML.Name)
	err = crdsClient.Delete(context.TODO(), crdFromYAML.Name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logger.Infof("crd %s does not exist", etcdCRDName)
	} else {
		Expect(err).NotTo(HaveOccurred())
	}

	// wait for old crd to be deleted
	Eventually(func() error {
		_, err := crdsClient.Get(context.TODO(), crdFromYAML.Name, metav1.GetOptions{})
		return err
	}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

	logger.Infof("creating etcd crd %s", crdFromYAML.Name)
	crd, err := crdsClient.Create(context.TODO(), crdFromYAML, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	// wait for crd to be created
	Eventually(func() error {
		_, err := crdsClient.Get(context.TODO(), crd.Name, metav1.GetOptions{})
		return err
	}, timeout, pollingInterval).Should(BeNil())
	logger.Infof("created etcd crd %s", crd.Name)
	etcdCRDName = crd.Name // for deleting later

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	logger.Printf("setting up k8s client using KUBECONFIG=%s", kubeconfigPath)
	typedClient, err = getKubernetesTypedClient(logger, kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())
	namespacesClient := typedClient.CoreV1().Namespaces()

	logger.Infof("creating namespace %s", druidNamespace)
	ns, err := namespacesClient.Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: druidNamespace,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		_, err := namespacesClient.Get(context.TODO(), ns.Name, metav1.GetOptions{})
		return err
	}, timeout, pollingInterval).Should(BeNil())
	logger.Infof("created namespace %s", ns.Name)

	druidChartPath = path.Join(sourcePath, druidChartBasePath)
	logger.Infof("deploying druid helm chart at %s", druidChartPath)

	chartValues := map[string]interface{}{
		"image":                     druidImage,
		"version":                   druidVersion,
		"replicas":                  1,
		"ignoreOperationAnnotation": false,
	}

	err = helmDeployChart(logger, timeout, "install", kubeconfigPath, druidChartPath, druidReleaseName, druidNamespace, chartValues, true)
	Expect(err).NotTo(HaveOccurred())
	logger.Infof("deployed helm chart to release '%s'", druidReleaseName)

	logger.Infof("waiting for deployment %s to become ready", druidReleaseName)
	deploymentsClient := typedClient.AppsV1().Deployments(druidNamespace)
	Eventually(func() error {
		druidDeployment, err := deploymentsClient.Get(context.TODO(), druidReleaseName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if druidDeployment.Status.ReadyReplicas != 1 {
			return fmt.Errorf("etcd-druid deployment not ready")
		}
		return nil
	}, timeout, pollingInterval).Should(BeNil())
	logger.Infof("deployment %s is ready", druidReleaseName)

	logger.Infof("creating namespace %s", etcdNamespace)
	ns, err = namespacesClient.Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdNamespace,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		_, err := namespacesClient.Get(context.TODO(), ns.Name, metav1.GetOptions{})
		return err
	}, timeout, pollingInterval).Should(BeNil())
	logger.Infof("created namespace %s", ns.Name)

	// deploy TLS secrets
	secretsClient := typedClient.CoreV1().Secrets(etcdNamespace)

	certsPath := path.Join(sourcePath, certsBasePath)
	_, err = buildAndDeployTLSSecrets(logger, certsPath, providers, secretsClient)
	Expect(err).NotTo(HaveOccurred())

	// initialize object storage providers
	storageContainer = getEnvAndExpectNoError(envStorageContainer)

	for _, provider := range providers {
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
		logger.Infof("created secret %s", secret.Name)
	}
})

var _ = AfterSuite(func() {
	kubeconfigPath, err := getEnvOrError(envKubeconfigPath)
	Expect(err).NotTo(HaveOccurred())

	logger.Printf("setting up k8s client using KUBECONFIG=%s", kubeconfigPath)
	typedClient, err = getKubernetesTypedClient(logger, kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())
	namespacesClient := typedClient.CoreV1().Namespaces()

	logger.Infof("deleting namespace %s", etcdNamespace)
	namespacesClient = typedClient.CoreV1().Namespaces()
	err = namespacesClient.Delete(context.TODO(), etcdNamespace, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logger.Infof("namespace %s does not exist", etcdNamespace)
	} else {
		Expect(err).NotTo(HaveOccurred())
	}
	Eventually(func() error {
		_, err = namespacesClient.Get(context.TODO(), etcdNamespace, metav1.GetOptions{})
		return err
	}, timeout*2, pollingInterval).Should(matchers.BeNotFoundError())
	logger.Infof("deleted namespace %s", etcdNamespace)

	logger.Infof("deleting namespace %s", druidNamespace)
	namespacesClient = typedClient.CoreV1().Namespaces()
	err = namespacesClient.Delete(context.TODO(), druidNamespace, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logger.Infof("namespace %s does not exist", druidNamespace)
	} else {
		Expect(err).NotTo(HaveOccurred())
	}
	Eventually(func() error {
		_, err = namespacesClient.Get(context.TODO(), druidNamespace, metav1.GetOptions{})
		return err
	}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
	logger.Infof("deleted namespace %s", druidNamespace)

	logger.Printf("setting up k8s apiextension client using KUBECONFIG=%s", kubeconfigPath)
	apiExtensionClient, err := getKubernetesAPIExtensionClient(logger, kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())
	crdsClient := apiExtensionClient.ApiextensionsV1beta1().CustomResourceDefinitions()

	logger.Infof("deleting etcd crd %s", etcdCRDName)
	err = crdsClient.Delete(context.TODO(), etcdCRDName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		logger.Infof("crd %s does not exist", etcdCRDName)
	} else {
		Expect(err).NotTo(HaveOccurred())
	}

	// wait for etcd crd to be deleted
	Eventually(func() error {
		_, err := crdsClient.Get(context.TODO(), etcdCRDName, metav1.GetOptions{})
		return err
	}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
})

func getEnvAndExpectNoError(key string) string {
	val, err := getEnvOrError(key)
	Expect(err).NotTo(HaveOccurred())
	return val
}
