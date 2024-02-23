// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SecretController", func() {
	var (
		testSecretName = "test-secret"
		testNamespace  = "test-namespace"
	)

	Describe("#isFinalizerNeeded", func() {
		var (
			etcdList druidv1alpha1.EtcdList
		)

		BeforeEach(func() {
			etcdList = druidv1alpha1.EtcdList{}
			for i := 0; i < 3; i++ {
				etcdList.Items = append(etcdList.Items, druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("etcd-%d", i),
						Namespace: testNamespace,
					},
				})
			}
		})

		It("should return false if secret is not referred to by any Etcd object", func() {
			isFinalizerNeeded, _ := isFinalizerNeeded(testSecretName, &etcdList)
			Expect(isFinalizerNeeded).To(BeFalse())
		})

		It("should return true if secret is referred to in an Etcd object's Spec.Etcd.ClientUrlTLS section", func() {
			etcdList.Items[0].Spec.Etcd.ClientUrlTLS = &druidv1alpha1.TLSConfig{
				ServerTLSSecretRef: v1.SecretReference{
					Name:      testSecretName,
					Namespace: testNamespace,
				},
			}
			isFinalizerNeeded, _ := isFinalizerNeeded(testSecretName, &etcdList)
			Expect(isFinalizerNeeded).To(BeTrue())
		})

		It("should return true if secret is referred to in an Etcd object's Spec.Etcd.PeerUrlTLS section", func() {
			etcdList.Items[1].Spec.Etcd.PeerUrlTLS = &druidv1alpha1.TLSConfig{
				ServerTLSSecretRef: v1.SecretReference{
					Name:      testSecretName,
					Namespace: testNamespace,
				},
			}
			isFinalizerNeeded, _ := isFinalizerNeeded(testSecretName, &etcdList)
			Expect(isFinalizerNeeded).To(BeTrue())
		})

		It("should return true if secret is referred to in an Etcd object's Spec.Backup.Store section", func() {
			etcdList.Items[2].Spec.Backup.Store = &druidv1alpha1.StoreSpec{
				SecretRef: &v1.SecretReference{
					Name:      testSecretName,
					Namespace: testNamespace,
				},
			}
			isFinalizerNeeded, _ := isFinalizerNeeded(testSecretName, &etcdList)
			Expect(isFinalizerNeeded).To(BeTrue())
		})
	})

	Describe("#addFinalizer", func() {
		var (
			secret     *corev1.Secret
			ctx        context.Context
			logger     = logr.Discard()
			fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
		)

		Context("test #addFinalizer on existing secret", func() {
			BeforeEach(func() {
				ctx = context.Background()
				secret = ensureSecretCreation(ctx, testSecretName, testNamespace, fakeClient)
			})

			AfterEach(func() {
				ensureSecretRemoval(ctx, testSecretName, testNamespace, fakeClient)
			})

			It("should return nil if secret already has finalizer", func() {
				Expect(controllerutils.AddFinalizers(ctx, fakeClient, secret, common.FinalizerName)).To(Succeed())
				Expect(addFinalizer(ctx, logger, fakeClient, secret)).To(BeNil())
			})

			It("should add finalizer on secret if finalizer does not exist", func() {
				Expect(addFinalizer(ctx, logger, fakeClient, secret)).To(Succeed())
				finalizers := sets.NewString(secret.Finalizers...)
				Expect(finalizers.Has(common.FinalizerName)).To(BeTrue())
			})
		})

		Context("test #addFinalizer when secret does not exist", func() {
			BeforeEach(func() {
				ctx = context.Background()
				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-existent",
						Namespace: testNamespace,
						// resource version is required as OptimisticLock is used for the merge patch
						// operation used by controllerutils.AddFinalizers()
						ResourceVersion: "42",
					},
				}
			})

			It("should not return an error if the secret is not found", func() {
				Expect(addFinalizer(ctx, logger, fakeClient, secret)).To(BeNil())
			})
		})
	})

	Describe("#removeFinalizer", func() {
		var (
			secret     *corev1.Secret
			ctx        context.Context
			logger     = logr.Discard()
			fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
		)

		Context("test remove finalizer on an existing secret", func() {
			BeforeEach(func() {
				ctx = context.Background()
				secret = ensureSecretCreation(ctx, testSecretName, testNamespace, fakeClient)
			})

			AfterEach(func() {
				ensureSecretRemoval(ctx, testSecretName, testNamespace, fakeClient)
			})

			It("should return nil if secret does not have finalizer", func() {
				Expect(controllerutils.RemoveAllFinalizers(ctx, fakeClient, secret)).Should(Succeed())
				Expect(removeFinalizer(ctx, logger, fakeClient, secret)).To(BeNil())
			})

			It("should remove finalizer from secret if finalizer exists", func() {
				Expect(removeFinalizer(ctx, logger, fakeClient, secret)).To(Succeed())
				finalizers := sets.NewString(secret.Finalizers...)
				Expect(finalizers.Has(common.FinalizerName)).To(BeFalse())
			})
		})

		Context("test remove finalizer on a non-existing secret", func() {
			BeforeEach(func() {
				ctx = context.Background()
				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-existent",
						Namespace: testNamespace,
					},
				}
			})

			It("should not return an error if the secret is not found", func() {
				Expect(removeFinalizer(ctx, logger, fakeClient, secret)).To(BeNil())
			})
		})

	})
})

func ensureSecretCreation(ctx context.Context, name, namespace string, fakeClient client.WithWatch) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Finalizers: []string{
				common.FinalizerName,
			},
		},
	}

	By("Create Secret")
	Expect(fakeClient.Create(ctx, secret)).To(Succeed())

	By("Ensure Secret is created")
	Eventually(func() error {
		return fakeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
	}).Should(Succeed())

	return secret
}

func ensureSecretRemoval(ctx context.Context, name, namespace string, fakeClient client.WithWatch) {
	secret := &corev1.Secret{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err != nil {
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		return
	}

	By("Remove any existing finalizers on Secret")
	Expect(controllerutils.RemoveAllFinalizers(ctx, fakeClient, secret)).To(Succeed())

	By("Delete Secret")
	Expect(fakeClient.Delete(ctx, secret)).To(Succeed())

	By("Ensure Secret is deleted")
	Eventually(func() error {
		return fakeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
	}).Should(BeNotFoundError())
}
