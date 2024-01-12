// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
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
	"time"

	brtypes "github.com/gardener/etcd-backup-restore/pkg/types"
	"github.com/gardener/etcd-druid/pkg/common"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Etcd Compaction", func() {
	var (
		parentCtx context.Context
	)

	BeforeEach(func() {
		parentCtx = context.Background()
	})

	Context("when compaction is enabled for single-node etcd", func() {
		providers, err := getProviders()
		Expect(err).ToNot(HaveOccurred())

		var (
			cl               client.Client
			etcdName         string
			storageContainer string
		)

		for _, p := range providers {
			provider := p
			Context(fmt.Sprintf("with provider %s", provider.Name), func() {
				BeforeEach(func() {
					cl, err = getKubernetesClient(kubeconfigPath)
					Expect(err).ShouldNot(HaveOccurred())

					etcdName = fmt.Sprintf("etcd-%s", provider.Name)

					storageContainer = getEnvAndExpectNoError(envStorageContainer)

					snapstoreProvider := provider.Storage.Provider
					store, err := getSnapstore(string(snapstoreProvider), storageContainer, storePrefix)
					Expect(err).ShouldNot(HaveOccurred())

					// purge any existing backups in bucket
					Expect(purgeSnapstore(store)).To(Succeed())

					Expect(deployBackupSecret(parentCtx, cl, logger, provider, etcdNamespace, storageContainer))
				})

				It("should test compaction on backup", func() {
					ctx, cancelFunc := context.WithTimeout(parentCtx, 10*time.Minute)
					defer cancelFunc()

					etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
					objLogger := logger.WithValues("etcd", client.ObjectKeyFromObject(etcd))

					By("Create etcd")
					createAndCheckEtcd(ctx, cl, objLogger, etcd, singleNodeEtcdTimeout)

					By("Create debug pod")
					debugPod := createDebugPod(ctx, etcd)

					By("Check initial snapshot is available")

					latestSnapshotsBeforePopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, 8080)
					Expect(err).ShouldNot(HaveOccurred())
					// We don't expect any delta snapshot as the cluster
					Expect(latestSnapshotsBeforePopulate.DeltaSnapshots).To(HaveLen(0))

					latestSnapshotBeforePopulate := latestSnapshotsBeforePopulate.FullSnapshot
					Expect(latestSnapshotBeforePopulate).To(Not(BeNil()))

					By("Put keys into etcd")
					logger.Info("populating etcd with sequential key-value pairs",
						"fromKey", fmt.Sprintf("%s-1", etcdKeyPrefix), "fromValue", fmt.Sprintf("%s-1", etcdValuePrefix),
						"toKey", fmt.Sprintf("%s-10", etcdKeyPrefix), "toValue", fmt.Sprintf("%s-10", etcdValuePrefix))

					// populate 10 keys in etcd, finishing in 10 seconds
					err = populateEtcd(logger, kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, etcdKeyPrefix, etcdValuePrefix, 1, 10, time.Second*1)
					Expect(err).ShouldNot(HaveOccurred())

					By("Check snapshot after putting data into etcd")

					latestSnapshotsAfterPopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, 8080)
					Expect(err).ShouldNot(HaveOccurred())

					Eventually(func() int {
						latestSnapshotsAfterPopulate, err = getLatestSnapshots(kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, 8080)
						Expect(err).NotTo(HaveOccurred())
						return len(latestSnapshotsAfterPopulate.DeltaSnapshots)
					}, singleNodeEtcdTimeout, pollingInterval).Should(BeNumerically(">", len(latestSnapshotsBeforePopulate.DeltaSnapshots)))

					latestSnapshotAfterPopulate := latestSnapshotsAfterPopulate.FullSnapshot
					if numDeltas := len(latestSnapshotsAfterPopulate.DeltaSnapshots); numDeltas > 0 {
						latestSnapshotAfterPopulate = latestSnapshotsAfterPopulate.DeltaSnapshots[numDeltas-1]
					}

					Expect(latestSnapshotsAfterPopulate).To(Not(BeNil()))
					Expect(latestSnapshotAfterPopulate.CreatedOn.After(latestSnapshotBeforePopulate.CreatedOn)).To(BeTrue())

					By("Put additional data into etcd")
					logger.Info("populating etcd with sequential key-value pairs",
						"fromKey", fmt.Sprintf("%s-11", etcdKeyPrefix), "fromValue", fmt.Sprintf("%s-11", etcdValuePrefix),
						"toKey", fmt.Sprintf("%s-15", etcdKeyPrefix), "toValue", fmt.Sprintf("%s-15", etcdValuePrefix))
					// populate 5 keys in etcd, finishing in 5 seconds
					err = populateEtcd(logger, kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, etcdKeyPrefix, etcdValuePrefix, 11, 15, time.Second*1)
					Expect(err).ShouldNot(HaveOccurred())

					By("Trigger on-demand delta snapshot")
					_, err = triggerOnDemandSnapshot(kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, 8080, brtypes.SnapshotKindDelta)
					Expect(err).ShouldNot(HaveOccurred())

					snapshotTriggeredTime := time.Now()
					logger.Info("waiting for compaction job to become successful")
					Eventually(func() error {
						ctx, cancelFunc := context.WithTimeout(context.Background(), singleNodeEtcdTimeout)
						defer cancelFunc()

						config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
						if err != nil {
							return err
						}

						clientset, err := kubernetes.NewForConfig(config)
						if err != nil {
							return err
						}
						eventsClient := clientset.CoreV1().Events(etcd.Namespace)
						eventList, err := eventsClient.List(ctx, metav1.ListOptions{
							FieldSelector: fmt.Sprintf("involvedObject.kind=Etcd,involvedObject.name=%s", etcd.Name),
						})
						if err != nil {
							return err
						}

						for _, event := range eventList.Items {
							if event.FirstTimestamp.Time.Sub(snapshotTriggeredTime) > 0 {
								if event.Reason == common.EventReasonSnapshotCompactionSucceeded {
									return nil
								} else if event.Reason == common.EventReasonSnapshotCompactionFailed {
									return fmt.Errorf("snapshot compaction job failed")
								}
							}
						}
						return fmt.Errorf("no events with reason %s or %s found", common.EventReasonSnapshotCompactionSucceeded, common.EventReasonSnapshotCompactionFailed)
					}, singleNodeEtcdTimeout, pollingInterval).Should(BeNil())
					logger.Info("compaction job is successful")

					By("Verify that all the delta snapshots are compacted to full snapshots by compaction triggerred at first 15th revision")
					latestSnapshotsAfterPopulate, err = getLatestSnapshots(kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, 8080)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(len(latestSnapshotsAfterPopulate.DeltaSnapshots)).Should(BeNumerically("==", 0))

					By("Put additional data into etcd")
					logger.Info("populating etcd with sequential key-value pairs",
						"fromKey", fmt.Sprintf("%s-16", etcdKeyPrefix), "fromValue", fmt.Sprintf("%s-16", etcdValuePrefix),
						"toKey", fmt.Sprintf("%s-20", etcdKeyPrefix), "toValue", fmt.Sprintf("%s-20", etcdValuePrefix))
					// populate 5 keys in etcd, finishing in 5 seconds
					err = populateEtcd(logger, kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, etcdKeyPrefix, etcdValuePrefix, 16, 20, time.Second*1)
					Expect(err).ShouldNot(HaveOccurred())

					By("Trigger on-demand delta snapshot")
					_, err = triggerOnDemandSnapshot(kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, 8080, brtypes.SnapshotKindDelta)
					Expect(err).ShouldNot(HaveOccurred())

					By("Verify that there are new delta snapshots as compaction is not triggered yet because delta events have not reached next 15 revision")
					latestSnapshotsAfterPopulate, err = getLatestSnapshots(kubeconfigPath, namespace, etcdName, debugPod.Name, debugPod.Spec.Containers[0].Name, 8080)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(len(latestSnapshotsAfterPopulate.DeltaSnapshots)).Should(BeNumerically(">", 0))

					By("Delete debug pod")
					Expect(cl.Delete(ctx, debugPod)).ToNot(HaveOccurred())

					By("Delete etcd")
					deleteAndCheckEtcd(ctx, cl, objLogger, etcd, singleNodeEtcdTimeout)
				})
			})
		}
	})
})
