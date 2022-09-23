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
	"time"

	"github.com/gardener/etcd-druid/api/v1alpha1"

	brtypes "github.com/gardener/etcd-backup-restore/pkg/types"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s_labels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Etcd Backup", func() {
	var (
		parentCtx context.Context
	)

	BeforeEach(func() {
		parentCtx = context.Background()
	})

	Context("when single-node etcd is configured", func() {
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

				It("should create, test backup and delete etcd with backup", func() {
					ctx, cancelFunc := context.WithTimeout(parentCtx, 10*time.Minute)
					defer cancelFunc()

					etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
					objLogger := logger.WithValues("etcd", client.ObjectKeyFromObject(etcd))

					By("Create etcd")
					createAndCheckEtcd(ctx, cl, objLogger, etcd, singleNodeEtcdTimeout)

					By("Check initial snapshot is available")
					var podName = fmt.Sprintf("%s-0", etcdName)

					latestSnapshotsBeforePopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, podName, "backup-restore", 8080)
					Expect(err).ShouldNot(HaveOccurred())
					// We don't expect any delta snapshot as the cluster
					Expect(latestSnapshotsBeforePopulate.DeltaSnapshots).To(HaveLen(0))

					latestSnapshotBeforePopulate := latestSnapshotsBeforePopulate.FullSnapshot
					Expect(latestSnapshotBeforePopulate).To(Not(BeNil()))

					By("Put keys into etcd")
					logger.Info(fmt.Sprintf("populating etcd with %s-1 to %s-10", etcdKeyPrefix, etcdKeyPrefix))
					// populate 10 keys in etcd, finishing in 10 seconds
					err = populateEtcdWithCount(logger, kubeconfigPath, namespace, etcdName, podName, "etcd", etcdKeyPrefix, etcdValuePrefix, 1, 10, time.Second*1)
					Expect(err).ShouldNot(HaveOccurred())

					By("Check snapshot after putting data into etcd")
					// allow 5 second buffer to upload full/delta snapshot
					time.Sleep(time.Second * 5)

					latestSnapshotsAfterPopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, podName, "backup-restore", 8080)
					Expect(err).ShouldNot(HaveOccurred())

					latestSnapshotAfterPopulate := latestSnapshotsAfterPopulate.FullSnapshot
					if numDeltas := len(latestSnapshotsAfterPopulate.DeltaSnapshots); numDeltas > 0 {
						latestSnapshotAfterPopulate = latestSnapshotsAfterPopulate.DeltaSnapshots[numDeltas-1]
					}

					Expect(latestSnapshotsAfterPopulate).To(Not(BeNil()))
					Expect(latestSnapshotAfterPopulate.CreatedOn.After(latestSnapshotBeforePopulate.CreatedOn)).To(BeTrue())

					By("Trigger on-demand full snapshot")
					fullSnapshot, err := triggerOnDemandSnapshot(kubeconfigPath, namespace, podName, "backup-restore", 8080, brtypes.SnapshotKindFull)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(fullSnapshot.LastRevision).To(Equal(10 + latestSnapshotBeforePopulate.LastRevision))

					By("Put additional data into etcd")
					logger.Info(fmt.Sprintf("populating etcd with %s-11 to %s-15", etcdKeyPrefix, etcdKeyPrefix))
					// populate 5 keys in etcd, finishing in 5 seconds
					err = populateEtcdWithCount(logger, kubeconfigPath, namespace, etcdName, podName, "etcd", etcdKeyPrefix, etcdValuePrefix, 11, 15, time.Second*1)
					Expect(err).ShouldNot(HaveOccurred())

					By("Trigger on-demand delta snapshot")
					_, err = triggerOnDemandSnapshot(kubeconfigPath, namespace, podName, "backup-restore", 8080, brtypes.SnapshotKindDelta)
					Expect(err).ShouldNot(HaveOccurred())

					By("Test cluster restoration by deleting data directory")
					Expect(deleteDir(kubeconfigPath, namespace, podName, "backup-restore", "/var/etcd/data/new.etcd/member")).To(Succeed())

					logger.Info("waiting for sts to become unready", "statefulSetName", etcdName)
					Eventually(func() error {
						ctx, cancelFunc := context.WithTimeout(context.Background(), singleNodeEtcdTimeout)
						defer cancelFunc()

						sts := &appsv1.StatefulSet{}
						if err := cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts); err != nil {
							return err
						}
						if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
							return fmt.Errorf("sts %s is still in ready state", etcdName)
						}
						return nil
					}, singleNodeEtcdTimeout, pollingInterval).Should(BeNil())
					logger.Info("sts is unready", "statefulSetName", etcdName)

					logger.Info("waiting for sts to become ready again", "statefulSetName", etcdName)
					Eventually(func() error {
						ctx, cancelFunc := context.WithTimeout(context.Background(), singleNodeEtcdTimeout)
						defer cancelFunc()

						sts := &appsv1.StatefulSet{}
						if err := cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts); err != nil {
							return err
						}
						if sts.Status.ReadyReplicas != *sts.Spec.Replicas {
							return fmt.Errorf("sts %s unready", etcdName)
						}
						return nil
					}, singleNodeEtcdTimeout, pollingInterval).Should(BeNil())
					logger.Info("sts is ready", "statefulSetName", etcdName)

					// verify existence and correctness of keys 1 to 30
					logger.Info("fetching etcd key-value pairs")
					keyValueMap, err := getEtcdKeys(logger, kubeconfigPath, namespace, etcdName, podName, "etcd", etcdKeyPrefix, 1, 15)
					Expect(err).ShouldNot(HaveOccurred())

					for i := 1; i <= 15; i++ {
						Expect(keyValueMap[fmt.Sprintf("%s-%d", etcdKeyPrefix, i)]).To(Equal(fmt.Sprintf("%s-%d", etcdValuePrefix, i)))
					}

					By("Delete etcd")
					deleteAndCheckEtcd(ctx, cl, objLogger, etcd, singleNodeEtcdTimeout)

				})
			})
		}
	})
})

func createAndCheckEtcd(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd, timeout time.Duration) {
	ExpectWithOffset(1, cl.Create(ctx, etcd)).ShouldNot(HaveOccurred())
	checkEtcdReady(ctx, cl, logger, etcd, timeout)
}

func checkEtcdReady(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd, timeout time.Duration) {
	logger.Info("Waiting for etcd to become ready")
	EventuallyWithOffset(2, func() error {
		ctx, cancelFunc := context.WithTimeout(ctx, timeout)
		defer cancelFunc()

		err := cl.Get(ctx, types.NamespacedName{Name: etcd.Name, Namespace: namespace}, etcd)
		if err != nil {
			return err
		}

		if etcd.Status.Ready == nil || *etcd.Status.Ready != true {
			return fmt.Errorf("etcd %s is not ready", etcd.Name)
		}

		if etcd.Status.ClusterSize == nil {
			return fmt.Errorf("etcd %s cluster size is empty", etcd.Name)
		}

		if *etcd.Status.ClusterSize != etcd.Spec.Replicas {
			return fmt.Errorf("etcd %s cluster size is %v, but it's not expected size as %v",
				etcd.Name, etcd.Status.ClusterSize, etcd.Spec.Replicas)
		}

		if len(etcd.Status.Conditions) == 0 {
			return fmt.Errorf("etcd %s status conditions is empty", etcd.Name)
		}

		for _, c := range etcd.Status.Conditions {
			// skip BackupReady status check if etcd.Spec.Backup.Store is not configured.
			if etcd.Spec.Backup.Store == nil && c.Type == v1alpha1.ConditionTypeBackupReady {
				continue
			}
			if c.Status != v1alpha1.ConditionTrue {
				return fmt.Errorf("etcd %q status %q condition %s is not True",
					etcd.Name, c.Type, c.Status)
			}
		}
		return nil
	}, timeout, pollingInterval).Should(BeNil())
	logger.Info("etcd is ready")

	logger.Info("Checking statefulset")
	sts := &appsv1.StatefulSet{}
	ExpectWithOffset(2, cl.Get(ctx, client.ObjectKeyFromObject(etcd), sts)).To(Succeed())
	ExpectWithOffset(2, sts.Status.ReadyReplicas).To(Equal(etcd.Spec.Replicas))

	logger.Info("Checking configmap")
	cm := &corev1.ConfigMap{}
	ExpectWithOffset(2, cl.Get(ctx, client.ObjectKey{Name: "etcd-bootstrap-" + string(etcd.UID[:6]), Namespace: etcd.Namespace}, cm)).To(Succeed())

	logger.Info("Checking client service")
	svc := &corev1.Service{}
	ExpectWithOffset(2, cl.Get(ctx, client.ObjectKey{Name: etcd.Name + "-client", Namespace: etcd.Namespace}, svc)).To(Succeed())
}

func deleteAndCheckEtcd(ctx context.Context, cl client.Client, logger logr.Logger, etcd *v1alpha1.Etcd, timeout time.Duration) {
	ExpectWithOffset(1, cl.Delete(ctx, etcd, client.PropagationPolicy(metav1.DeletePropagationForeground))).To(Succeed())

	logger.Info("Checking if etcd is gone")
	EventuallyWithOffset(1, func() error {
		ctx, cancelFunc := context.WithTimeout(ctx, timeout)
		defer cancelFunc()
		return cl.Get(ctx, client.ObjectKeyFromObject(etcd), etcd)
	}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	logger.Info("Checking if statefulset is gone")
	ExpectWithOffset(1,
		cl.Get(
			ctx,
			client.ObjectKeyFromObject(etcd),
			&appsv1.StatefulSet{},
		),
	).Should(matchers.BeNotFoundError())

	logger.Info("Checking if configmap is gone")
	ExpectWithOffset(1,
		cl.Get(
			ctx,
			client.ObjectKey{Name: "etcd-bootstrap-" + string(etcd.UID[:6]), Namespace: etcd.Namespace},
			&corev1.ConfigMap{},
		),
	).Should(matchers.BeNotFoundError())

	logger.Info("Checking client service is gone")
	ExpectWithOffset(1,
		cl.Get(
			ctx,
			client.ObjectKey{Name: etcd.Name + "-client", Namespace: etcd.Namespace},
			&corev1.Service{},
		),
	).Should(matchers.BeNotFoundError())

	// removing ETCD statefulset's PVCs,
	// because sometimes k8s garbage collection is delayed to remove PVCs before starting next tests.
	purgeEtcdPVCs(ctx, cl, etcd.Name)
}

func purgeEtcdPVCs(ctx context.Context, cl client.Client, etcdName string) {
	r, _ := k8s_labels.NewRequirement("instance", selection.Equals, []string{etcdName})
	pvc := &corev1.PersistentVolumeClaim{}
	delOptions := client.DeleteOptions{}
	delOptions.ApplyOptions([]client.DeleteOption{client.PropagationPolicy(metav1.DeletePropagationForeground)})
	ExpectWithOffset(1, client.IgnoreNotFound(cl.DeleteAllOf(ctx, pvc, &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			Namespace:     namespace,
			LabelSelector: k8s_labels.NewSelector().Add(*r),
		},
		DeleteOptions: delOptions,
	}))).ShouldNot(HaveOccurred())
}
