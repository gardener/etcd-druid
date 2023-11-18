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

package utils

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Store tests", func() {

	Describe("#GetHostMountPathFromSecretRef", func() {
		var (
			ctx        context.Context
			fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
			storeSpec  *druidv1alpha1.StoreSpec
			sec        *corev1.Secret
			logger     = logr.Discard()
		)
		const (
			testNamespace = "test-ns"
		)

		BeforeEach(func() {
			ctx = context.Background()
			storeSpec = &druidv1alpha1.StoreSpec{}
		})

		AfterEach(func() {
			if sec != nil {
				err := fakeClient.Delete(ctx, sec)
				if err != nil {
					Expect(err).To(matchers.BeNotFoundError())
				}
			}
		})

		It("no secret ref configured, should return default mount path", func() {
			hostMountPath, err := GetHostMountPathFromSecretRef(ctx, fakeClient, logger, storeSpec, testNamespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(hostMountPath).To(Equal(LocalProviderDefaultMountPath))
		})

		It("secret ref points to an unknown secret, should return an error", func() {
			storeSpec.SecretRef = &corev1.SecretReference{
				Name:      "not-to-be-found-secret-ref",
				Namespace: testNamespace,
			}
			_, err := GetHostMountPathFromSecretRef(ctx, fakeClient, logger, storeSpec, testNamespace)
			Expect(err).ToNot(BeNil())
			Expect(err).To(matchers.BeNotFoundError())
		})

		It("secret ref points to a secret whose data does not have path set, should return default mount path", func() {
			const secretName = "backup-secret"
			sec = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{"bucketName": []byte("NDQ5YjEwZj")},
			}
			Expect(fakeClient.Create(ctx, sec)).To(Succeed())
			storeSpec.SecretRef = &corev1.SecretReference{
				Name:      "backup-secret",
				Namespace: testNamespace,
			}
			hostMountPath, err := GetHostMountPathFromSecretRef(ctx, fakeClient, logger, storeSpec, testNamespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(hostMountPath).To(Equal(LocalProviderDefaultMountPath))
		})

		It("secret ref points to a secret whose data has a path, should return the path defined in secret.Data", func() {
			const (
				secretName = "backup-secret"
				hostPath   = "/var/data/etcd-backup"
			)
			sec = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{"bucketName": []byte("NDQ5YjEwZj"), "hostPath": []byte(hostPath)},
			}
			Expect(fakeClient.Create(ctx, sec)).To(Succeed())
			storeSpec.SecretRef = &corev1.SecretReference{
				Name:      "backup-secret",
				Namespace: testNamespace,
			}
			hostMountPath, err := GetHostMountPathFromSecretRef(ctx, fakeClient, logger, storeSpec, testNamespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(hostMountPath).To(Equal(hostPath))
		})
	})
})
