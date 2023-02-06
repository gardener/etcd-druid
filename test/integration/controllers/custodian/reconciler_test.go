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

package custodian

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	componentpdb "github.com/gardener/etcd-druid/pkg/component/etcd/poddisruptionbudget"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	timeout         = time.Minute * 5
	pollingInterval = time.Second * 2
)

var _ = Describe("Custodian Controller", func() {
	Describe("Updating Etcd status", func() {
		Context("when statefulset status is updated", func() {
			var (
				instance *druidv1alpha1.Etcd
				sts      *appsv1.StatefulSet
				ctx      = context.TODO()
				name     = "foo11"
			)

			BeforeEach(func() {
				instance = testutils.EtcdBuilderWithDefaults(name, testNamespace.Name).Build()

				Expect(k8sClient.Create(ctx, instance)).To(Succeed())
				// wait for Etcd creation to succeed
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      name,
						Namespace: testNamespace.Name,
					}, instance)
				}, timeout, pollingInterval).Should(BeNil())

				// update etcd status.Ready to true so that custodian predicate is satisfied
				instance.Status.Ready = pointer.Bool(true)
				Expect(k8sClient.Status().Update(ctx, instance)).To(Succeed())
				// wait for etcd status update to succeed
				Eventually(func() error {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance); err != nil {
						return err
					}
					if *instance.Status.Ready != true {
						return fmt.Errorf("Etcd not ready yet")
					}
					return nil
				}, timeout, pollingInterval).Should(Succeed())

				// create sts manually, since there is no running etcd controller to create sts upon Etcd creation
				sts = testutils.CreateStatefulSet(name, testNamespace.Name, instance.UID, instance.Spec.Replicas)
				Expect(k8sClient.Create(ctx, sts)).To(Succeed())
				// wait for sts creation to succeed
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      name,
						Namespace: testNamespace.Name,
					}, sts)
				}, timeout, pollingInterval).Should(BeNil())
			})

			It("should update value of Etcd.Status.ReadyReplicas to value of Statefulset.Status.ReadyReplicas", func() {
				sts.Status.Replicas = 1
				sts.Status.ReadyReplicas = 1
				sts.Status.ObservedGeneration = 2
				Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())

				// wait for sts status update to succeed
				Eventually(func() error {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
						return err
					}
					if sts.Status.ReadyReplicas != 1 {
						return fmt.Errorf("Statefulset ReadyReplicas should be equal to 1")
					}
					return nil
				}, timeout, pollingInterval).Should(Succeed())

				// Wait for desired ReadyReplicas value to be reflected in etcd status
				Eventually(func() error {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance); err != nil {
						return err
					}

					if int(instance.Status.ReadyReplicas) != 1 {
						return fmt.Errorf("Etcd ready replicas should be equal to 1")
					}
					return nil
				}, timeout, pollingInterval).Should(BeNil())
			})

			It("should mark statefulset status not ready when no readyreplicas in statefulset", func() {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), sts)
				Expect(err).ToNot(HaveOccurred())

				// Forcefully change ReadyReplicas in statefulset to 0
				sts.Status.ReadyReplicas = 0
				Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())

				// wait for sts status update to succeed
				Eventually(func() error {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), sts)
					if err != nil {
						return err
					}

					if sts.Status.ReadyReplicas > 0 {
						return fmt.Errorf("No readyreplicas of statefulset should exist at this point")
					}

					return nil
				}, timeout, pollingInterval).Should(BeNil())

				// wait for etcd status to reflect the change in ReadyReplicas
				Eventually(func() error {
					err = k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
					if err != nil {
						return err
					}

					if instance.Status.ReadyReplicas > 0 {
						return fmt.Errorf("ReadyReplicas should be zero in ETCD instance")
					}

					return nil
				}, timeout, pollingInterval).Should(BeNil())
			})

			AfterEach(func() {
				// Delete etcd instance
				Expect(k8sClient.Delete(ctx, instance)).To(Succeed())
				// Delete sts
				Expect(k8sClient.Delete(ctx, sts)).To(Succeed())

				// Wait for etcd to be deleted
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
				}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

				// Wait for sts to be deleted
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(sts), sts)
				}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
			})
		})
	})

	Describe("PodDisruptionBudget", func() {
		Context("minAvailable of PodDisruptionBudget", func() {
			When("having a single node cluster", func() {
				etcd := testutils.EtcdBuilderWithDefaults("test", "default").WithReadyStatus().Build()

				Expect(len(etcd.Status.Members)).To(BeEquivalentTo(1))
				Expect(*etcd.Status.ClusterSize).To(BeEquivalentTo(1))

				It("should be set to 0", func() {
					etcd.Spec.Replicas = 1
					Expect(componentpdb.CalculatePDBMinAvailable(etcd)).To(BeEquivalentTo(0))
					etcd.Spec.Replicas = 0
					Expect(componentpdb.CalculatePDBMinAvailable(etcd)).To(BeEquivalentTo(0))
				})
			})

			When("clusterSize is nil", func() {
				etcd := testutils.EtcdBuilderWithDefaults("test", "default").WithReplicas(3).WithReadyStatus().Build()
				etcd.Status.ClusterSize = nil

				It("should be set to quorum size", func() {
					Expect(componentpdb.CalculatePDBMinAvailable(etcd)).To(BeEquivalentTo(2))
				})
			})

			When("having a multi node cluster", func() {
				etcd := testutils.EtcdBuilderWithDefaults("test", "default").WithReplicas(5).WithReadyStatus().Build()

				Expect(len(etcd.Status.Members)).To(BeEquivalentTo(5))
				Expect(*etcd.Status.ClusterSize).To(BeEquivalentTo(5))

				It("should calculate the value correctly", func() {
					Expect(componentpdb.CalculatePDBMinAvailable(etcd)).To(BeEquivalentTo(3))
				})
			})
		})
	})

})
