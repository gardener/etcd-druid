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

	"k8s.io/apimachinery/pkg/util/sets"

	"gopkg.in/yaml.v3"

	brtypes "github.com/gardener/etcd-backup-restore/pkg/types"
	"github.com/gardener/etcd-druid/api/v1alpha1"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
)

var _ = Describe("Druid", func() {
	var entries []TableEntry
	providers, err := getProviders()
	Expect(err).ToNot(HaveOccurred())

	Context("with druid running with ignore-annotation=false", func() {
		// druid is already running with ignore-annotation=false
		// so no extra steps to be taken here

		entries = getEntries(providers, "should successfully create etcd and corresponding resources")
		DescribeTable("creating etcd resource", func(etcdName string, provider Provider, providerName string) {
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

		entries = getCloudProviderEntries(providers, "should upload snapshot when etcd is populated")
		DescribeTable("putting keys into etcd", func(etcdName string, _ Provider, providerName string) {
			var (
				latestSnapshotBeforePopulate *brtypes.Snapshot
				latestSnapshotAfterPopulate  *brtypes.Snapshot
				podName                      = fmt.Sprintf("%s-0", etcdName)
				err                          error
			)

			latestSnapshotsBeforePopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, podName, "backup-restore", 8080)
			Expect(err).ShouldNot(HaveOccurred())

			if len(latestSnapshotsBeforePopulate.DeltaSnapshots) == 0 {
				latestSnapshotBeforePopulate = latestSnapshotsBeforePopulate.FullSnapshot
			} else {
				latestSnapshotBeforePopulate = latestSnapshotsBeforePopulate.DeltaSnapshots[len(latestSnapshotsBeforePopulate.DeltaSnapshots)-1]
			}
			Expect(latestSnapshotBeforePopulate).To(Not(BeNil()))

			logger.Info(fmt.Sprintf("populating etcd with %s-1 to %s-10", etcdKeyPrefix, etcdKeyPrefix))
			// populate 10 keys in etcd, finishing in 10 seconds
			err = populateEtcdWithCount(logger, kubeconfigPath, namespace, etcdName, podName, "etcd", etcdKeyPrefix, etcdValuePrefix, 1, 10, time.Second*1)
			Expect(err).ShouldNot(HaveOccurred())

			// allow 5 second buffer to upload full/delta snapshot
			time.Sleep(time.Second * 5)

			latestSnapshotsAfterPopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, podName, "backup-restore", 8080)
			Expect(err).ShouldNot(HaveOccurred())

			if len(latestSnapshotsAfterPopulate.DeltaSnapshots) == 0 {
				latestSnapshotAfterPopulate = latestSnapshotsAfterPopulate.FullSnapshot
			} else {
				latestSnapshotAfterPopulate = latestSnapshotsAfterPopulate.DeltaSnapshots[len(latestSnapshotsAfterPopulate.DeltaSnapshots)-1]
			}
			Expect(latestSnapshotsAfterPopulate).To(Not(BeNil()))

			Expect(latestSnapshotAfterPopulate.CreatedOn.After(latestSnapshotBeforePopulate.CreatedOn)).To(BeTrue())

		}, entries...)

		entries = getCloudProviderEntries(providers, "should upload full snapshot when triggered on-demand")
		DescribeTable("triggering on-demand full snapshot", func(etcdName string, _ Provider, providerName string) {
			var (
				prevFullSnapshot *brtypes.Snapshot
				fullSnapshot     *brtypes.Snapshot
				podName          = fmt.Sprintf("%s-0", etcdName)
				err              error
			)

			latestSnapshotsBeforePopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, podName, "backup-restore", 8080)
			Expect(err).ShouldNot(HaveOccurred())
			prevFullSnapshot = latestSnapshotsBeforePopulate.FullSnapshot
			Expect(prevFullSnapshot).To(Not(BeNil()))

			logger.Info(fmt.Sprintf("populating etcd with %s-11 to %s-20", etcdKeyPrefix, etcdKeyPrefix))
			// populate 10 keys in etcd, finishing in 1 second
			err = populateEtcdWithCount(logger, kubeconfigPath, namespace, etcdName, podName, "etcd", etcdKeyPrefix, etcdValuePrefix, 11, 20, time.Millisecond*100)
			Expect(err).ShouldNot(HaveOccurred())

			fullSnapshot, err = triggerOnDemandSnapshot(kubeconfigPath, namespace, podName, "backup-restore", 8080, brtypes.SnapshotKindFull)
			Expect(err).ShouldNot(HaveOccurred())

			if fullSnapshot == nil {
				// check if a full snapshot was taken just before
				// triggering the on-demand full snapshot
				latestSnapshotsAfterPopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, podName, "backup-restore", 8080)
				Expect(err).ShouldNot(HaveOccurred())
				latestFullSnapshot := latestSnapshotsAfterPopulate.FullSnapshot
				Expect(latestFullSnapshot).To(Not(BeNil()))
				Expect(latestFullSnapshot.LastRevision).To(Equal(10 + prevFullSnapshot.LastRevision))
			}

			Expect(fullSnapshot.CreatedOn.After(prevFullSnapshot.CreatedOn)).To(BeTrue())

		}, entries...)

		entries = getCloudProviderEntries(providers, "should upload delta snapshot when triggered on-demand")
		DescribeTable("triggering on-demand delta snapshot", func(etcdName string, _ Provider, providerName string) {
			var (
				prevDeltaSnapshotTimestamp time.Time
				deltaSnapshot              *brtypes.Snapshot
				prevSnapshot               *brtypes.Snapshot
				podName                    = fmt.Sprintf("%s-0", etcdName)
				err                        error
			)

			latestSnapshotsBeforePopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, podName, "backup-restore", 8080)
			Expect(err).ShouldNot(HaveOccurred())
			if len(latestSnapshotsBeforePopulate.DeltaSnapshots) > 0 {
				prevDeltaSnapshot := latestSnapshotsBeforePopulate.DeltaSnapshots[len(latestSnapshotsBeforePopulate.DeltaSnapshots)-1]
				Expect(prevDeltaSnapshot).To(Not(BeNil()))
				prevDeltaSnapshotTimestamp = prevDeltaSnapshot.CreatedOn
				prevSnapshot = prevDeltaSnapshot
			} else {
				prevDeltaSnapshotTimestamp = time.Now()
				prevSnapshot = latestSnapshotsBeforePopulate.FullSnapshot
			}
			Expect(prevSnapshot).ToNot(BeNil())

			logger.Info(fmt.Sprintf("populating etcd with %s-21 to %s-30", etcdKeyPrefix, etcdKeyPrefix))
			// populate 10 keys in etcd, finishing in 1 second
			err = populateEtcdWithCount(logger, kubeconfigPath, namespace, etcdName, podName, "etcd", etcdKeyPrefix, etcdValuePrefix, 21, 30, time.Millisecond*100)
			Expect(err).ShouldNot(HaveOccurred())

			deltaSnapshot, err = triggerOnDemandSnapshot(kubeconfigPath, namespace, podName, "backup-restore", 8080, brtypes.SnapshotKindDelta)
			Expect(err).ShouldNot(HaveOccurred())

			if deltaSnapshot == nil {
				// check if a delta snapshot was taken just before
				// triggering the on-demand delta snapshot
				latestSnapshotsAfterPopulate, err := getLatestSnapshots(kubeconfigPath, namespace, etcdName, podName, "backup-restore", 8080)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(len(latestSnapshotsBeforePopulate.DeltaSnapshots)).To(BeNumerically(">", 0))

				latestDeltaSnapshot := latestSnapshotsAfterPopulate.DeltaSnapshots[len(latestSnapshotsAfterPopulate.DeltaSnapshots)-1]
				Expect(latestDeltaSnapshot).To(Not(BeNil()))
				Expect(latestDeltaSnapshot.LastRevision).To(Equal(10 + prevSnapshot.LastRevision))
			}

			Expect(deltaSnapshot.CreatedOn.After(prevDeltaSnapshotTimestamp)).To(BeTrue())

		}, entries...)

		entries = getCloudProviderEntries(providers, "should restore etcd member directory on deletion")
		DescribeTable("deleting etcd member directory", func(etcdName string, _ Provider, providerName string) {
			var (
				podName   = fmt.Sprintf("%s-0", etcdName)
				stsClient = typedClient.AppsV1().StatefulSets(namespace)
				err       error
			)

			err = deleteDir(kubeconfigPath, namespace, podName, "backup-restore", "/var/etcd/data/new.etcd/member")
			Expect(err).ShouldNot(HaveOccurred())

			logger.Info("waiting for sts to become unready", "statefulSetName", etcdName)
			Eventually(func() error {
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()

				etcdSts, err := stsClient.Get(ctx, etcdName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if etcdSts.Status.ReadyReplicas == *etcdSts.Spec.Replicas {
					return fmt.Errorf("sts %s is still in ready state", etcdName)
				}
				return nil
			}, timeout*3, pollingInterval).Should(BeNil())
			logger.Info("sts is unready", "statefulSetName", etcdName)

			logger.Info("waiting for sts to become ready again", "statefulSetName", etcdName)
			Eventually(func() error {
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()

				etcdSts, err := stsClient.Get(ctx, etcdName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if etcdSts.Status.ReadyReplicas != *etcdSts.Spec.Replicas {
					return fmt.Errorf("sts %s unready", etcdName)
				}
				return nil
			}, timeout*3, pollingInterval).Should(BeNil())
			logger.Info("sts is ready", "statefulSetName", etcdName)

			// verify existence and correctness of keys 1 to 30
			logger.Info("fetching etcd key-value pairs")
			keyValueMap, err := getEtcdKeys(logger, kubeconfigPath, namespace, etcdName, podName, "etcd", etcdKeyPrefix, 1, 30, 10)
			Expect(err).ShouldNot(HaveOccurred())

			for i := 1; i <= 30; i++ {
				if i%10 == 0 {
					continue
				}
				Expect(keyValueMap[fmt.Sprintf("%s-%d", etcdKeyPrefix, i)]).To(Equal(fmt.Sprintf("%s-%d", etcdValuePrefix, i)))
			}

		}, entries...)

		entries = getEntries(providers, "should trigger reconciliation on adding reconcile annotation")
		DescribeTable("adding reconcile annotation", func(etcdName string, provider Provider, providerName string) {
			var (
				etcd                = getEmptyEtcd(etcdName, namespace)
				druidManagedField   metav1.ManagedFieldsEntry
				prevResourceVersion string
				prevTimestamp       time.Time
			)
			client, err := getKubernetesClient(kubeconfigPath)
			Expect(err).ShouldNot(HaveOccurred())

			logger.Info("waiting for etcd to become ready", "etcdName", etcdName)
			Eventually(func() error {
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

			prevResourceVersion = etcd.ResourceVersion
			managedFields := etcd.ManagedFields
			for _, f := range managedFields {
				if f.Manager == "etcd-druid" {
					druidManagedField = f
				}
			}
			prevTimestamp = druidManagedField.Time.Time

			// annotate etcd to trigger reconciliation
			ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
			defer cancelFunc()

			_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
				annotations := etcd.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
				etcd.SetAnnotations(annotations)
				return nil
			})
			Expect(err).ShouldNot(HaveOccurred())

			logger.Info("waiting for etcd to be reconciled", "etcdName", etcdName)
			Eventually(func() error {
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()

				err := client.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: namespace}, etcd)
				if err != nil || apierrors.IsNotFound(err) {
					return err
				}

				managedFields = etcd.ManagedFields
				for _, f := range managedFields {
					if f.Manager == "etcd-druid" {
						druidManagedField = f
					}
				}
				timestamp := druidManagedField.Time.Time

				if !timestamp.After(prevTimestamp) {
					return fmt.Errorf("etcd %s not reconciled", etcdName)
				}

				return nil
			}, timeout, pollingInterval).Should(BeNil())
			logger.Info("etcd has been reconciled", "etcdName", etcdName)

			managedFields = etcd.ManagedFields
			for _, f := range managedFields {
				if f.Manager == "etcd-druid" {
					druidManagedField = f
				}
			}
			operation := druidManagedField.Operation
			resourceVersion := etcd.ResourceVersion

			Expect(resourceVersion).To(Not(Equal(prevResourceVersion)))
			Expect(operation).To(Equal(metav1.ManagedFieldsOperationUpdate))

		}, entries...)

		entries = getEntries(providers, "should trigger reconciliation on spec change")
		DescribeTable("modifying etcd spec", func(etcdName string, provider Provider, providerName string) {
			var (
				etcd              = getEmptyEtcd(etcdName, namespace)
				druidManagedField metav1.ManagedFieldsEntry
				prevTimestamp     time.Time
				metricsExtensive  = v1alpha1.Extensive
				etcdConfYAMLName  = "etcd.conf.yaml"
				metricsField      = "metrics"
			)
			client, err := getKubernetesClient(kubeconfigPath)
			Expect(err).ShouldNot(HaveOccurred())

			logger.Info("waiting for etcd to become ready", "etcdName", etcdName)
			Eventually(func() error {
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

			managedFields := etcd.ManagedFields
			for _, f := range managedFields {
				if f.Manager == "etcd-druid" {
					druidManagedField = f
				}
			}
			prevTimestamp = druidManagedField.Time.Time

			// modify etcd spec
			ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
			defer cancelFunc()

			_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
				metav1.SetMetaDataAnnotation(&etcd.ObjectMeta, v1beta1constants.GardenerOperation, v1beta1constants.GardenerOperationReconcile)
				etcd.Spec.Etcd.Metrics = &metricsExtensive
				return nil
			})
			Expect(err).ShouldNot(HaveOccurred())

			logger.Info("waiting for etcd to be reconciled", "etcdName", etcdName)
			Eventually(func() error {
				ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
				defer cancelFunc()

				err := client.Get(ctx, types.NamespacedName{Name: etcdName, Namespace: namespace}, etcd)
				if err != nil || apierrors.IsNotFound(err) {
					return err
				}

				managedFields = etcd.ManagedFields
				for _, f := range managedFields {
					if f.Manager == "etcd-druid" {
						druidManagedField = f
					}
				}
				timestamp := druidManagedField.Time.Time

				if !timestamp.After(prevTimestamp) {
					return fmt.Errorf("etcd %s not reconciled", etcdName)
				}

				return nil
			}, timeout, pollingInterval).Should(BeNil())
			logger.Info("etcd has been reconciled", "etcdName", etcdName)

			stsClient := typedClient.AppsV1().StatefulSets(namespace)
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

			cmClient := typedClient.CoreV1().ConfigMaps(namespace)
			ctx, cancelFunc = context.WithTimeout(context.TODO(), timeout)
			defer cancelFunc()

			cm, err := cmClient.Get(ctx, cmName, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			etcdConfYAML, etcdConfYAMLExists := cm.Data[etcdConfYAMLName]
			Expect(etcdConfYAMLExists).To(BeTrue())

			yamlData := make(map[interface{}]interface{})
			err = yaml.Unmarshal([]byte(etcdConfYAML), &yamlData)
			Expect(err).ShouldNot(HaveOccurred())

			metricsValue, metricsFieldExists := yamlData[metricsField]
			Expect(metricsFieldExists).To(BeTrue())
			Expect(metricsValue).To(Equal(fmt.Sprintf("%v", metricsExtensive)))

		}, entries...)

		entries = getEntries(providers, "should trigger reconciliation and throw error on invalid spec change")
		DescribeTable("making invalid change to etcd spec", func(etcdName string, provider Provider, providerName string) {
			var (
				etcd                                      = getEmptyEtcd(etcdName, namespace)
				invalidMetricsLevel v1alpha1.MetricsLevel = "invalid"
			)
			client, err := getKubernetesClient(kubeconfigPath)
			Expect(err).ShouldNot(HaveOccurred())

			logger.Info("waiting for etcd to become ready", "etcdName", etcdName)
			Eventually(func() error {
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

			// modify etcd spec with invalid value
			ctx, cancelFunc := context.WithTimeout(context.TODO(), timeout)
			defer cancelFunc()

			_, err = controllerutil.CreateOrUpdate(ctx, client, etcd, func() error {
				etcd.Spec.Etcd.Metrics = &invalidMetricsLevel
				return nil
			})
			Expect(err).Should(HaveOccurred())

		}, entries...)

		entries = getEntries(providers, "should successfully delete etcd and corresponding resources")
		DescribeTable("deleting etcd resource", func(etcdName string, provider Provider, providerName string) {
			etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
			client, err := getKubernetesClient(kubeconfigPath)
			Expect(err).ShouldNot(HaveOccurred())
			typedClient, err := getKubernetesTypedClient(kubeconfigPath)
			Expect(err).ShouldNot(HaveOccurred())

			ctx, stsCancelFunc := context.WithTimeout(context.TODO(), timeout)
			defer stsCancelFunc()

			stsClient := typedClient.AppsV1().StatefulSets(namespace)
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

		entries = getEntries(providers, "should fail to spin up the etcd")
		DescribeTable("creating etcd resource with invalid etcd image version", func(etcdName string, provider Provider, providerName string) {
			var (
				etcd                                      = getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
				invalidMetricsLevel v1alpha1.MetricsLevel = "invalid"
			)

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

				etcd.Spec.Etcd.Metrics = &invalidMetricsLevel

				return nil
			})
			Expect(err).Should(HaveOccurred())

		}, entries...)

		entries = getEntries(providers, "should successfully delete etcd for invalid etcd spec")
		DescribeTable("deleting etcd resource with invalid etcd spec", func(etcdName string, provider Provider, providerName string) {
			etcd := getDefaultEtcd(etcdName, namespace, storageContainer, storePrefix, provider)
			client, err := getKubernetesClient(kubeconfigPath)
			Expect(err).ShouldNot(HaveOccurred())

			ctx, etcdCancelFunc := context.WithTimeout(context.TODO(), timeout)
			defer etcdCancelFunc()

			err = kutil.DeleteObjects(ctx, client, etcd)
			Expect(err).ShouldNot(HaveOccurred())
			logger.Info("issued delete call to etcd", "etcdName", etcdName)

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

func getEntries(providers map[string]Provider, description string, exclusions ...string) []TableEntry {
	excl := sets.NewString(exclusions...)
	entries := make([]TableEntry, 0, len(providers))

	for providerName, provider := range providers {
		if excl.Has(providerName) {
			continue
		}
		providerSuffix := provider.Suffix
		etcdName := fmt.Sprintf("%s-%s", etcdPrefix, providerSuffix)
		entry := Entry(fmt.Sprintf("%s for %s", description, etcdName), etcdName, provider, providerName)
		entries = append(entries, entry)
	}

	return entries
}

func getCloudProviderEntries(providers map[string]Provider, description string) []TableEntry {
	return getEntries(providers, description, providerLocal)
}
