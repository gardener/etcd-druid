// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/util/yaml"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"os"
	"time"

	"context"

	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const insertCommand = "export ETCDCTL_API=3; for i in $(seq 1 %d); do etcdctl --cacert=/var/etcd/ssl/ca/ca.crt --cert=/var/etcd/ssl/client/tls.crt --key=/var/etcd/ssl/client/tls.key --endpoints=etcd-main-local:2379 put key$i val$i; done"
const deleteCommand = "export ETCDCTL_API=3; for i in $(seq 1 %d); do etcdctl --cacert=/var/etcd/ssl/ca/ca.crt --cert=/var/etcd/ssl/client/tls.crt --key=/var/etcd/ssl/client/tls.key --endpoints=etcd-main-local:2379 del key$i; done"
const getKeyCommand = "export ETCDCTL_API=3; etcdctl --cacert=/var/etcd/ssl/ca/ca.crt --cert=/var/etcd/ssl/client/tls.crt --key=/var/etcd/ssl/client/tls.key --endpoints=etcd-main-local:2379 get key1000"
const corruptCommand = "echo \"corrupt\" > /var/etcd/data/new.etcd/member/snap/db"

const timeout = time.Minute * 2
const pollingInterval = time.Second * 2

var _ = Describe("Druid", func() {
	DescribeTable("when adding etcd resources with statefulset already present",
		func(provider string) {
			var err error
			var instance *druidv1alpha1.Etcd
			var c client.Client
			var sts appsv1.StatefulSet
			var secrets map[string]corev1.Secret
			var executor podExecutor
			instance = &druidv1alpha1.Etcd{}
			err = getEtcdToDeploy(instance, provider)
			Expect(err).NotTo(HaveOccurred())
			secrets, err = getSecretsToDeploy()
			Expect(err).NotTo(HaveOccurred())
			c = mgr.GetClient()
			executor = podExecutor{
				client: &executorClient{
					client: k8sClient,
					config: cfg,
				},
			}
			defer WithWd("../../..")()
			err = c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())

			for _, secret := range secrets {
				err = c.Create(context.TODO(), &secret)
				//Expect(err).NotTo(HaveOccurred())
			}
			Eventually(func() error { return statefulSetIsReady(c, instance, &sts) }, timeout, pollingInterval).Should(BeNil())
			logger.Info("Adding key/values to etcd...")
			Eventually(func() error { return addKeyValuesToEtcd(&executor, instance) }, timeout, pollingInterval).Should(BeNil())
			//wait till the delta snapshots have been taken
			time.Sleep(70 * time.Second)
			logger.Info("Corrupting etcd...")

			err = corruptEtcd(&executor, instance)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error { return statefulSetIsReady(c, instance, &sts) }, timeout, pollingInterval).Should(BeNil())
			Eventually(func() (string, error) { return getKeyFromEtcd(&executor, instance) }, timeout, pollingInterval).Should(Equal("key1000\nval1000\n"))
			err = deleteKeyValuesToEtcd(&executor, instance)
			Expect(err).NotTo(HaveOccurred())
			err = c.Delete(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error { return etcdRemoved(c, instance) }, timeout, pollingInterval).Should(BeNil())
		},
		Entry("when etcd resource deployed for aws", "aws"),
		Entry("when etcd resource deployed for azure", "az"),
		Entry("when etcd resource deployed for gcp", "gcp"),
	)

	//Reconciliation of new etcd resource deployment without any existing statefulsets.
	Context("when adding etcd resources ", func() {

	})
})

func etcdRemoved(c client.Client, etcd *druidv1alpha1.Etcd) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	e := &druidv1alpha1.Etcd{}
	req := types.NamespacedName{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}
	if err := c.Get(ctx, req, e); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers
			return nil
		}
		return err
	}
	return fmt.Errorf("etcd not deleted")
}

func statefulSetIsReady(c client.Client, etcd *druidv1alpha1.Etcd, ss *appsv1.StatefulSet) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}
	if err := c.Get(ctx, req, ss); err != nil {
		return err
	}

	if err := health.CheckStatefulSet(ss); err != nil {
		return err
	}
	return nil
}

// WithWd sets the working directory and returns a function to revert to the previous one.
func WithWd(path string) func() {
	oldPath, err := os.Getwd()
	if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	if err := os.Chdir(path); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	return func() {
		if err := os.Chdir(oldPath); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func getEtcdToDeploy(etcd *druidv1alpha1.Etcd, provider string ) error {
	pathToEtcdResource, err := filepath.Abs(fmt.Sprintf("resources/etcd-main-%s.yaml", provider))
	if err != nil {
		return err
	}
	f, err := os.Open(pathToEtcdResource)
	if err != nil {
		return err
	}
	err = yaml.NewYAMLOrJSONDecoder(f, 4096).Decode(&etcd)
	if err != nil {
		return err
	}
	return nil
}

func getSecretsToDeploy() (map[string]corev1.Secret, error) {
	secretsToFetch := []string{
		"ca-etcd.yaml",
		"etcd-client-tls.yaml",
		"etcd-server-cert.yaml",
	}
	secrets := map[string]corev1.Secret{}
	for _, s := range secretsToFetch {
		pathToSecret, err := filepath.Abs(fmt.Sprintf("resources/%s", s))
		if err != nil {
			return nil, err
		}
		f, err := os.Open(pathToSecret)
		if err != nil {
			return nil, err
		}
		secret := corev1.Secret{}
		err = yaml.NewYAMLOrJSONDecoder(f, 4096).Decode(&secret)
		if err != nil {
			return nil, err
		}
		secrets[s] = secret

	}
	return secrets, nil
}

func addKeyValuesToEtcd(executor PodExecutor, etcd *druidv1alpha1.Etcd) error {
	_, err := executor.Execute(context.TODO(), etcd.Namespace, fmt.Sprintf("%s-0", etcd.Name), "etcd", fmt.Sprintf(insertCommand, 1000))
	return err
}

func deleteKeyValuesToEtcd(executor PodExecutor, etcd *druidv1alpha1.Etcd) error {
	_, err := executor.Execute(context.TODO(), etcd.Namespace, fmt.Sprintf("%s-0", etcd.Name), "etcd", fmt.Sprintf(deleteCommand, 1000))
	return err
}

func getKeyFromEtcd(executor PodExecutor, etcd *druidv1alpha1.Etcd) (string, error) {
	r, err := executor.Execute(context.TODO(), etcd.Namespace, fmt.Sprintf("%s-0", etcd.Name), "etcd", getKeyCommand)
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return "", err
	}
	fmt.Println(string(buf))
	return string(buf), nil
}

func corruptEtcd(executor PodExecutor, etcd *druidv1alpha1.Etcd) error {
	_, err := executor.Execute(context.TODO(), etcd.Namespace, fmt.Sprintf("%s-0", etcd.Name), "etcd", corruptCommand)
	return err
}
