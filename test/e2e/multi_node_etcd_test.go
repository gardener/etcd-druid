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
	"fmt"
	"strings"
	"time"

	"github.com/gardener/etcd-druid/api/v1alpha1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Druid multi-node", func() {
	var entries []TableEntry
	providers, err := getProviders()
	Expect(err).ToNot(HaveOccurred())

	Context("with ignore-annotation=false", func() {

		Context("Etcd cluster operation", func() {

			entries = getEntries(providers, "should successfully create a multi-node etcd cluster of 3 members")
			DescribeTable("creation of 3-member etcd cluster", func(etcdName string, provider Provider, providerName string) {
				etcd := getDefaultMultiNodeEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
				client, err := getKubernetesClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()

				_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
					if providerName == providerLocal {
						etcd.Spec.Backup.Store = nil
					} else {
						etcd.Spec.Backup.Store.SecretRef = &corev1.SecretReference{
							Name:      fmt.Sprintf("%s-%s", etcdBackupSecretPrefix, provider.Suffix),
							Namespace: namespace,
						}
					}
					return nil
				})
				Expect(err).ShouldNot(HaveOccurred())

				logger.Info("waiting for sts to become ready", "statefulsetName", etcdName)
				stsClient := typedClient.AppsV1().StatefulSets(namespace)
				checkEventuallyEtcdStsReady(etcdName, multiNodeEtcdReplicas, stsClient)
				logger.Info("sts is ready", "statefulSetName", etcdName)

				etcdSts, err := getEtcdStsObj(stsClient, etcdName)
				Expect(err).ShouldNot(HaveOccurred())

				volumes := etcdSts.Spec.Template.Spec.Volumes
				cmName := ""
				for _, v := range volumes {
					if v.Name == etcdConfigMapVolumeName {
						cmName = v.ConfigMap.Name
					}
				}
				Expect(cmName).ShouldNot(BeEmpty())

				svcName := etcdSts.Spec.ServiceName
				Expect(svcName).ShouldNot(BeEmpty())

				logger.Info("waiting for configmap to be created", "configmapName", cmName)
				cmClient := typedClient.CoreV1().ConfigMaps(namespace)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := cmClient.Get(ctx, cmName, metav1.GetOptions{})
					if err != nil {
						return fmt.Errorf("configmap %s does not exist: %v", cmName, err)
					}
					return nil
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("configmap has been created", "configmapName", cmName)

				logger.Info("waiting for service to be created", "serviceName", svcName)
				svcClient := typedClient.CoreV1().Services(namespace)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := svcClient.Get(ctx, svcName, metav1.GetOptions{})
					if err != nil {
						return fmt.Errorf("service %s does not exist: %v", svcName, err)
					}
					return nil
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("service has been created", "serviceName", svcName)

				logger.Info("waiting for etcd to become ready", "etcdName", etcdName)
				checkEventuallyEtcdClusterReady(client, namespace, etcdName, multiNodeEtcdReplicas)
				logger.Info("etcd is ready", "etcdName", etcdName)
			}, entries...)

			entries = getEntries(providers, "should successfully scale down etcd replicas from 3 to 0")
			DescribeTable("hibernation ", func(etcdName string, provider Provider, providerName string) {
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()

				etcd := getEmptyEtcd(etcdName, namespace)
				client, err := getKubernetesClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())

				_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
					annotations := etcd.GetAnnotations()
					if annotations == nil {
						annotations = make(map[string]string)
					}
					// annotate etcd to trigger reconciliation
					annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
					etcd.SetAnnotations(annotations)
					// scale down etcd replicas 3 -> 0
					etcd.Spec.Replicas = int32(0)
					return nil
				})
				Expect(err).ShouldNot(HaveOccurred())

				err = getEtcdObj(client, etcd)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(etcd.Spec.Replicas).Should(BeNumerically("==", 0))

				logger.Info("waiting to scale down etcd sts replicas from 3 to 0", "etcdName", etcdName)
				stsClient := typedClient.AppsV1().StatefulSets(namespace)
				Eventually(func() error {
					etcdSts, err := getEtcdStsObj(stsClient, etcdName)
					if err != nil {
						return err
					}

					if etcdSts.Status.ReadyReplicas != 0 {
						return fmt.Errorf("sts %s not ready", etcdName)
					}
					return nil
				}, timeout*3, pollingInterval).Should(BeNil())
				logger.Info("etcd sts scaled down replicas from 3 to 0", "etcdName", etcdName)

				logger.Info("waiting to scale down etcd replicas from 3 to 0", "etcdName", etcdName)
				Eventually(func() error {
					etcd := getEmptyEtcd(etcdName, namespace)
					err := getEtcdObj(client, etcd)
					if err != nil {
						return err
					}

					if etcd != nil && etcd.Status.Ready != nil {
						if *etcd.Status.Ready != true {
							return fmt.Errorf("etcd %s is not ready", etcdName)
						}
					}

					if etcd.Status.ClusterSize == nil {
						return fmt.Errorf("etcd %s cluster size is empty", etcdName)
					}
					// TODO: uncomment me once scale down is supported,
					// currently ClusterSize is not updated while scaling down.
					// if *etcd.Status.ClusterSize != 0 {
					// 	return fmt.Errorf("etcd %q cluster size is %d, but expected to be 0",
					// 		etcdName, *etcd.Status.ClusterSize)
					// }

					for _, c := range etcd.Status.Conditions {
						if c.Status != v1alpha1.ConditionUnknown {
							return fmt.Errorf("etcd %s status condition is %q, but expected to be %s ",
								etcdName, c.Status, v1alpha1.ConditionUnknown)
						}
					}

					return nil
				}, timeout*3, pollingInterval).Should(BeNil())
				logger.Info("etcd is successfully scaled down replicas from 3 to 0", "etcdName", etcdName)
			}, entries...)

			entries = getEntries(providers, "should successfully scale up etcd replicas from 0 to 3")
			DescribeTable("hibernation wakeup ", func(etcdName string, provider Provider, providerName string) {
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()

				etcd := getEmptyEtcd(etcdName, namespace)
				client, err := getKubernetesClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())

				_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
					annotations := etcd.GetAnnotations()
					if annotations == nil {
						annotations = make(map[string]string)
					}
					// annotate etcd to trigger reconciliation
					annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
					etcd.SetAnnotations(annotations)
					// set replicas to scale up 0 -> 3
					etcd.Spec.Replicas = int32(multiNodeEtcdReplicas)
					return nil
				})
				Expect(err).ShouldNot(HaveOccurred())

				// ensure etcd replicas are updated properly
				err = getEtcdObj(client, etcd)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(etcd.Spec.Replicas).Should(BeNumerically("==", 3))

				logger.Info("waiting for sts to become ready", "statefulsetName", etcdName)
				stsClient := typedClient.AppsV1().StatefulSets(namespace)
				checkEventuallyEtcdStsReady(etcdName, multiNodeEtcdReplicas, stsClient)
				logger.Info("sts is ready", "statefulSetName", etcdName)

				logger.Info("waiting for etcd replicas to scale up from 0 to 3", "etcdName", etcdName)
				checkEventuallyEtcdClusterReady(client, namespace, etcdName, multiNodeEtcdReplicas)
				logger.Info("etcd is ready and successfully scaled up", "etcdName", etcdName)
			}, entries...)

			// TODO: Need to investigate why etcd cluster is not healthy while rolling update
			entries = getEntries(providers, "should successfully update pod labels with etcd cluster zero downtime")
			XDescribeTable("zero downtime while rolling updates ", func(etcdName string, provider Provider, providerName string) {
				etcd := getEmptyEtcd(etcdName, namespace)
				client, err := getKubernetesClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())

				err = getEtcdObj(client, etcd)
				Expect(err).ShouldNot(HaveOccurred())
				oldEtcdObservedGeneration := *etcd.Status.ObservedGeneration

				jobClient := typedClient.BatchV1().Jobs(namespace)
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()
				zeroDownTimeValidator, err := jobClient.Create(
					ctx, etcdZeroDownTimeValidatorJob(etcdName+"-client", "rolling-update"), metav1.CreateOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				defer cleanUpTestHelperJob(jobClient, zeroDownTimeValidator.Name)

				// Wait until zeroDownTimeValidator job is up and running.
				checkEventuallyJobReady(typedClient, jobClient, zeroDownTimeValidator.Name)

				stsClient := typedClient.AppsV1().StatefulSets(namespace)
				etcdSts, err := getEtcdStsObj(stsClient, etcdName)
				Expect(err).ShouldNot(HaveOccurred())
				oldStsObservedGeneration := etcdSts.Status.ObservedGeneration

				_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
					annotations := etcd.GetAnnotations()
					if annotations == nil {
						annotations = make(map[string]string)
					}
					// annotate etcd to trigger reconciliation
					annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
					etcd.SetAnnotations(annotations)
					// adding a new label, to stimulate rolling update.
					etcd.Spec.Labels["testing"] = "e2e"
					return nil
				})
				Expect(err).ShouldNot(HaveOccurred())

				logger.Info("waiting for rolling update to be completed", "etcdName", etcdName)
				// Wait until Etcd rolling update is complete.
				checkEventuallyEtcdRollingUpdateDone(client, stsClient, etcdName,
					oldStsObservedGeneration, oldEtcdObservedGeneration)
				logger.Info("rolling update is completed", "etcdName", etcdName)

				// Checking Etcd cluster is healthy and there is no downtime while rolling update.
				// K8s job zeroDownTimeValidator will fail, if there is downtime in Etcd cluster health.
				ctx, cancelFunc = context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()
				job, err := jobClient.Get(ctx, zeroDownTimeValidator.Name, metav1.GetOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(job.Status.Failed).Should(BeNumerically("==", 0))
				logger.Info("etcd cluster is healthy and there is no downtime while rolling update", "etcdName", etcdName)
			}, entries...)

			entries = getEntries(providers, "should successfully defragment all etcd members with zero downtime")
			DescribeTable("defragmentation ", func(etcdName string, provider Provider, providerName string) {
				etcd := getEmptyEtcd(etcdName, namespace)
				client, err := getKubernetesClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())

				err = getEtcdObj(client, etcd)
				Expect(err).ShouldNot(HaveOccurred())
				oldEtcdObservedGeneration := *etcd.Status.ObservedGeneration

				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()
				jobClient := typedClient.BatchV1().Jobs(namespace)
				zeroDownTimeValidator, err := jobClient.Create(ctx, etcdZeroDownTimeValidatorJob(etcdName+"-client", "defragmentation"), metav1.CreateOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				defer cleanUpTestHelperJob(jobClient, zeroDownTimeValidator.Name) // Job is removed, Once test case is done.

				// Wait until zeroDownTimeValidator job is up and running.
				checkEventuallyJobReady(typedClient, jobClient, zeroDownTimeValidator.Name)

				stsClient := typedClient.AppsV1().StatefulSets(namespace)
				etcdSts, err := getEtcdStsObj(stsClient, etcdName)
				Expect(err).ShouldNot(HaveOccurred())
				oldStsObservedGeneration := etcdSts.Status.ObservedGeneration
				var defragmentationSchedule = "*/1 * * * *"
				_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
					annotations := etcd.GetAnnotations()
					if annotations == nil {
						annotations = make(map[string]string)
					}
					// annotate etcd to trigger reconciliation
					annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
					etcd.SetAnnotations(annotations)
					// configure defragmentation schedule for every one minute for this test case.
					*etcd.Spec.Etcd.DefragmentationSchedule = defragmentationSchedule
					return nil
				})
				Expect(err).ShouldNot(HaveOccurred())

				// ensure defragmentation schedule is updated.
				err = getEtcdObj(client, etcd)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(*etcd.Spec.Etcd.DefragmentationSchedule).Should(BeEquivalentTo(defragmentationSchedule))

				logger.Info("waiting for etcd to update new config and schedule defragmentation for every 1 minute",
					"etcdName", etcdName)
				// Wait until Etcd rolling update is complete.
				checkEventuallyEtcdRollingUpdateDone(client, stsClient, etcdName,
					oldStsObservedGeneration, oldEtcdObservedGeneration)
				logger.Info("updated etcd config to schedule defragmentation for every 1 minute",
					"etcdName", etcdName)

				// Wait until etcd cluster defragmentation finish.
				Eventually(func() error {
					leaderPodName, err := getEtcdLeaderPodName(typedClient, namespace)
					if err != nil {
						return err
					}
					// Get etcd leader pod logs to ensure etcd cluster defrgmentation is finished or not.
					logs, err := getPodLogs(typedClient, namespace, leaderPodName, &corev1.PodLogOptions{
						Container:    "backup-restore",
						SinceSeconds: pointer.Int64(60),
					})

					if err != nil {
						return fmt.Errorf("error occurred while getting [%s] pod logs, error: %v ", leaderPodName, err)
					}

					for i := 0; i < int(multiNodeEtcdReplicas); i++ {
						if !strings.Contains(logs,
							fmt.Sprintf("Finished defragmenting etcd member[http://%s-%d.%s-peer.%s.svc:2379]",
								etcd.Name, i, etcd.Name, namespace)) {
							return fmt.Errorf("etcd %q defragmentaiton is not finished for member %q-%d", etcdName, etcdName, i)
						}
					}

					ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
					defer cancelFunc()
					// Checking Etcd cluster is healthy and there is no downtime while rolling update.
					// K8s job zeroDownTimeValidator will fail, if there is downtime in Etcd cluster health.
					job, err := jobClient.Get(ctx, zeroDownTimeValidator.Name, metav1.GetOptions{})
					Expect(err).ShouldNot(HaveOccurred())
					Expect(job.Status.Failed).Should(BeNumerically("==", 0))
					return nil
				}, time.Minute*6, pollingInterval).Should(BeNil())
			}, entries...)

			entries = getEntries(providers, "should successfully delete 3 member multi-node etcd cluster")
			DescribeTable("deletion of 3-member etcd cluster", func(etcdName string, provider Provider, providerName string) {
				etcd := getDefaultMultiNodeEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
				client, err := getKubernetesClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())
				typedClient, err := getKubernetesTypedClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())

				stsClient := typedClient.AppsV1().StatefulSets(namespace)
				etcdSts, err := getEtcdStsObj(stsClient, etcdName)
				Expect(err).ShouldNot(HaveOccurred())

				volumes := etcdSts.Spec.Template.Spec.Volumes
				cmName := ""
				for _, v := range volumes {
					if v.Name == etcdConfigMapVolumeName {
						cmName = v.ConfigMap.Name
					}
				}
				Expect(cmName).ShouldNot(BeEmpty())

				svcName := etcdSts.Spec.ServiceName
				Expect(svcName).ShouldNot(BeEmpty())

				ctx, etcdCancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer etcdCancelFunc()

				err = kutil.DeleteObjects(ctx, client, etcd)
				Expect(err).ShouldNot(HaveOccurred())
				logger.Info("issued delete call to etcd", "etcdName", etcdName)

				logger.Info("waiting for sts to be deleted", "etcdName", etcdName)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := stsClient.Get(ctx, etcdName, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							return nil
						}
						return err
					}
					return fmt.Errorf("sts %s still exists", etcdName)
				}, timeout*3, pollingInterval).Should(BeNil())
				logger.Info("sts has been deleted", "statefulSetName", etcdName)

				logger.Info("waiting for configmap to be deleted", "configMapName", cmName)
				cmClient := typedClient.CoreV1().ConfigMaps(namespace)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := cmClient.Get(ctx, cmName, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							return nil
						}
						return err
					}
					return fmt.Errorf("configmap %s still exists", cmName)
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("configmap has been deleted", "configMapName", cmName)

				logger.Info("waiting for service to be deleted", "serviceName", svcName)
				svcClient := typedClient.CoreV1().Services(namespace)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := svcClient.Get(ctx, svcName, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							return nil
						}
						return err
					}
					return fmt.Errorf("service %s still exists", svcName)
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("service has been deleted", "serviceName", svcName)

				logger.Info("waiting for etcd to be deleted", "etcdName", etcdName)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					err := client.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: namespace}, &v1alpha1.Etcd{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							return nil
						}
						return err
					}
					return fmt.Errorf("etcd %s still exists", etcdName)
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("etcd has been deleted", "etcdName", etcdName)
			}, entries...)

		})

		XContext("Scaling", func() {

			entries = getEntries(providers, "should successfully create etcd and corresponding resources")
			DescribeTable("creating single node etcd", func(etcdName string, provider Provider, providerName string) {
				etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
				client, err := getKubernetesClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()

				_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
					if providerName == providerLocal {
						etcd.Spec.Backup.Store = nil
					} else {
						etcd.Spec.Backup.Store.SecretRef = &corev1.SecretReference{
							Name:      fmt.Sprintf("%s-%s", etcdBackupSecretPrefix, provider.Suffix),
							Namespace: namespace,
						}
					}
					return nil
				})
				Expect(err).ShouldNot(HaveOccurred())

				logger.Info("waiting for sts to become ready", "statefulsetName", etcdName)
				stsClient := typedClient.AppsV1().StatefulSets(namespace)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					etcdSts, err := stsClient.Get(ctx, etcdName, metav1.GetOptions{})
					if err != nil {
						return err
					}
					if etcdSts.Status.ReadyReplicas != etcd.Spec.Replicas {
						return fmt.Errorf("sts %s not ready", etcdName)
					}
					return nil
				}, timeout*3, pollingInterval).Should(BeNil())
				logger.Info("sts is ready", "statefulSetName", etcdName)

				ctx, stsCancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer stsCancelFunc()
				etcdSts, err := stsClient.Get(ctx, etcdName, metav1.GetOptions{})
				Expect(err).ShouldNot(HaveOccurred())

				volumes := etcdSts.Spec.Template.Spec.Volumes
				cmName := ""
				for _, v := range volumes {
					if v.Name == etcdConfigMapVolumeName {
						cmName = v.ConfigMap.Name
					}
				}
				Expect(cmName).ShouldNot(BeEmpty())

				svcName := etcdSts.Spec.ServiceName
				Expect(svcName).ShouldNot(BeEmpty())

				logger.Info("waiting for configmap to be created", "configmapName", cmName)
				cmClient := typedClient.CoreV1().ConfigMaps(namespace)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := cmClient.Get(ctx, cmName, metav1.GetOptions{})
					if err != nil {
						return fmt.Errorf("configmap %s does not exist: %v", cmName, err)
					}
					return nil
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("configmap has been created", "configmapName", cmName)

				logger.Info("waiting for service to be created", "serviceName", svcName)
				svcClient := typedClient.CoreV1().Services(namespace)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := svcClient.Get(ctx, svcName, metav1.GetOptions{})
					if err != nil {
						return fmt.Errorf("service %s does not exist: %v", svcName, err)
					}
					return nil
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("service has been created", "serviceName", svcName)

				logger.Info("waiting for etcd to become ready", "etcdName", etcdName)
				Eventually(func() error {
					etcd := getEmptyEtcd(etcdName, namespace)

					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					err := client.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: namespace}, etcd)
					if err != nil || apierrors.IsNotFound(err) {
						return err
					}
					if &etcd.Status != nil && etcd.Status.Ready != nil {
						if *etcd.Status.Ready != true {
							return fmt.Errorf("etcd %s not ready", etcdName)
						}
					}
					return nil
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("etcd is ready", "etcdName", etcdName)

			}, entries...)

			entries = getEntries(providers, "should successfully scale up etcd nodes from 1 -> 3")
			DescribeTable("healthy cluster ", func(etcdName string, provider Provider, providerName string) {
				// annotate etcd to trigger reconciliation
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()

				etcd := getEmptyEtcd(etcdName, namespace)
				client, err := getKubernetesClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())

				_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
					annotations := etcd.GetAnnotations()
					if annotations == nil {
						annotations = make(map[string]string)
					}
					annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
					etcd.SetAnnotations(annotations)
					etcd.Spec.Replicas = multiNodeEtcdReplicas
					return nil
				})
				Expect(err).ShouldNot(HaveOccurred())

				err = getEtcdObj(client, etcd)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(etcd.Spec.Replicas).Should(BeNumerically("==", 3))

				logger.Info("waiting to scale up etcd replicas from 1 to 3", "etcdName", etcdName)
				checkEventuallyEtcdClusterReady(client, namespace, etcdName, multiNodeEtcdReplicas)
				logger.Info("successfully scaled up healthy etcd cluster nodes from 1 to 3", "etcdName", etcdName)
			}, entries...)

			entries = getEntries(providers, "should successfully delete 3 member multi-node etcd cluster")
			DescribeTable("deletion of 3-member etcd cluster", func(etcdName string, provider Provider, providerName string) {
				etcd := getDefaultMultiNodeEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
				client, err := getKubernetesClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())
				typedClient, err := getKubernetesTypedClient(kubeconfigPath)
				Expect(err).ShouldNot(HaveOccurred())

				stsClient := typedClient.AppsV1().StatefulSets(namespace)
				etcdSts, err := getEtcdStsObj(stsClient, etcdName)
				Expect(err).ShouldNot(HaveOccurred())

				volumes := etcdSts.Spec.Template.Spec.Volumes
				cmName := ""
				for _, v := range volumes {
					if v.Name == etcdConfigMapVolumeName {
						cmName = v.ConfigMap.Name
					}
				}
				Expect(cmName).ShouldNot(BeEmpty())

				svcName := etcdSts.Spec.ServiceName
				Expect(svcName).ShouldNot(BeEmpty())

				ctx, etcdCancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer etcdCancelFunc()

				err = kutil.DeleteObjects(ctx, client, etcd)
				Expect(err).ShouldNot(HaveOccurred())
				logger.Info("issued delete call to etcd", "etcdName", etcdName)

				logger.Info("waiting for sts to be deleted", "etcdName", etcdName)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := stsClient.Get(ctx, etcdName, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							return nil
						}
						return err
					}
					return fmt.Errorf("sts %s still exists", etcdName)
				}, timeout*3, pollingInterval).Should(BeNil())
				logger.Info("sts has been deleted", "statefulSetName", etcdName)

				logger.Info("waiting for configmap to be deleted", "configMapName", cmName)
				cmClient := typedClient.CoreV1().ConfigMaps(namespace)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := cmClient.Get(ctx, cmName, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							return nil
						}
						return err
					}
					return fmt.Errorf("configmap %s still exists", cmName)
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("configmap has been deleted", "configMapName", cmName)

				logger.Info("waiting for service to be deleted", "serviceName", svcName)
				svcClient := typedClient.CoreV1().Services(namespace)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					_, err := svcClient.Get(ctx, svcName, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							return nil
						}
						return err
					}
					return fmt.Errorf("service %s still exists", svcName)
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("service has been deleted", "serviceName", svcName)

				logger.Info("waiting for etcd to be deleted", "etcdName", etcdName)
				Eventually(func() error {
					ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
					defer cancelFunc()

					err := client.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: namespace}, &v1alpha1.Etcd{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							return nil
						}
						return err
					}
					return fmt.Errorf("etcd %s still exists", etcdName)
				}, timeout, pollingInterval).Should(BeNil())
				logger.Info("etcd has been deleted", "etcdName", etcdName)
			}, entries...)
		})
	})
})
