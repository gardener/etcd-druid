// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druidstore "github.com/gardener/etcd-druid/internal/store"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"
	testutils "github.com/gardener/etcd-druid/test/utils"

	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var (
	timeout         = time.Minute * 2
	pollingInterval = time.Second * 2
)

var _ = Describe("Compaction Controller", func() {

	// When an ETCD resource is created, check if the associated compaction job is created with validateETCDCompactionJob
	DescribeTable("when etcd resource is created",
		func(name string,
			provider druidv1alpha1.StorageProvider,
			validateETCDCompactionJob func(*druidv1alpha1.Etcd, *batchv1.Job)) {
			var (
				instance       *druidv1alpha1.Etcd
				fullSnapLease  *coordinationv1.Lease
				deltaSnapLease *coordinationv1.Lease
				j              *batchv1.Job
			)

			instance = testutils.EtcdBuilderWithDefaults(name, namespace).WithPeerTLS().WithClientTLS().WithStorageProvider(provider).Build()
			createEtcdAndWait(k8sClient, instance)

			// manually create full and delta snapshot leases since etcd controller is not running
			fullSnapLease, deltaSnapLease = createEtcdSnapshotLeasesAndWait(k8sClient, instance)

			// manually update full lease spec since there is no running instance of etcd-backup-restore
			By("update full snapshot lease")
			fullSnapLease.Spec.HolderIdentity = ptr.To("0")
			fullSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), fullSnapLease)).To(Succeed())

			// manually update delta lease spec since there is no running instance of etcd-backup-restore
			By("update delta snapshot lease")
			deltaSnapLease.Spec.HolderIdentity = ptr.To("101")
			deltaSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), deltaSnapLease)).To(Succeed())

			// Verify if the job is created when difference between holder identities in delta-snapshot-revision and full-snapshot-revision is greater than 100
			j = &batchv1.Job{}
			Eventually(func() error {
				return jobIsCorrectlyReconciled(k8sClient, instance, j)
			}, timeout, pollingInterval).Should(BeNil())

			validateETCDCompactionJob(instance, j)

			deleteEtcdAndWait(k8sClient, instance)

			deleteEtcdSnapshotLeasesAndWait(k8sClient, instance)
		},
		Entry("if fields are set in etcd.Spec and TLS enabled, the resources should reflect the spec changes", "foo71", druidv1alpha1.StorageProvider("local"), validateEtcdForCompactionJob),
		Entry("if the store is S3, the statefulset and compaction job should reflect the spec changes", "foo72", druidv1alpha1.StorageProvider("aws"), validateStoreAWSForCompactionJob),
		Entry("if the store is ABS, the statefulset and compaction job should reflect the spec changes", "foo73", druidv1alpha1.StorageProvider("azure"), validateStoreAzureForCompactionJob),
		Entry("if the store is GCS, the statefulset and compaction job should reflect the spec changes", "foo74", druidv1alpha1.StorageProvider("gcp"), validateStoreGCPForCompactionJob),
		Entry("if the store is Swift, the statefulset and compaction job should reflect the spec changes", "foo75", druidv1alpha1.StorageProvider("openstack"), validateStoreOpenstackForCompactionJob),
		Entry("if the store is OSS, the statefulset and compaction job should reflect the spec changes", "foo76", druidv1alpha1.StorageProvider("alicloud"), validateStoreAlicloudForCompactionJob),
	)

	Context("when an existing job is already present", func() {
		var (
			instance       *druidv1alpha1.Etcd
			fullSnapLease  *coordinationv1.Lease
			deltaSnapLease *coordinationv1.Lease
			j              *batchv1.Job
		)

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), instance)).To(Succeed())
			Eventually(func() error { return testutils.IsEtcdRemoved(k8sClient, instance.Name, instance.Namespace, timeout) }, timeout, pollingInterval).Should(BeNil())
		})

		It("should create a new job if the existing job is failed", func() {
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			instance = testutils.EtcdBuilderWithDefaults("foo77", namespace).WithProviderLocal().Build()
			createEtcdAndWait(k8sClient, instance)

			// manually create full and delta snapshot leases since etcd controller is not running
			fullSnapLease, deltaSnapLease = createEtcdSnapshotLeasesAndWait(k8sClient, instance)

			// manually create compaction job
			// j = createCompactionJob(instance)
			// Expect(k8sClient.Create(ctx, j)).To(Succeed())

			// Deliberately update the full lease
			fullSnapLease.Spec.HolderIdentity = ptr.To("0")
			fullSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), fullSnapLease)).To(Succeed())

			// Deliberately update the delta lease
			deltaSnapLease.Spec.HolderIdentity = ptr.To("101")
			deltaSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), deltaSnapLease)).To(Succeed())

			j = &batchv1.Job{}
			Eventually(func() error {
				return jobIsCorrectlyReconciled(k8sClient, instance, j)
			}, timeout, pollingInterval).Should(BeNil())

			// Update job status as failed
			j.Status = batchv1.JobStatus{
				Failed: 1,
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionTrue,
					},
				},
			}
			j.Status.StartTime = &metav1.Time{Time: time.Now()}
			Expect(k8sClient.Status().Update(ctx, j)).To(Succeed())

			// Wait until the job gets the "foregroundDeletion" finalizer and remove it
			Eventually(func() (*batchv1.Job, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(j), j); err != nil {
					return nil, err
				}
				return j, nil
			}, timeout, pollingInterval).Should(PointTo(testutils.MatchFinalizer(metav1.FinalizerDeleteDependents)))
			Expect(kubernetes.RemoveFinalizers(ctx, k8sClient, j, metav1.FinalizerDeleteDependents)).To(Succeed())

			// Wait until the job has been deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(j), &batchv1.Job{})
			}, timeout, pollingInterval).Should(testutils.BeNotFoundError())

			// A new job should be created
			j = &batchv1.Job{}
			Eventually(func() error { return jobIsCorrectlyReconciled(k8sClient, instance, j) }, timeout, pollingInterval).Should(BeNil())
		})

		It("should delete the existing job if the job is succeeded", func() {
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			instance = testutils.EtcdBuilderWithDefaults("foo78", "default").WithProviderLocal().Build()
			createEtcdAndWait(k8sClient, instance)

			// manually create full and delta snapshot leases since etcd controller is not running
			fullSnapLease, deltaSnapLease = createEtcdSnapshotLeasesAndWait(k8sClient, instance)

			// Deliberately update the full lease
			fullSnapLease.Spec.HolderIdentity = ptr.To("0")
			fullSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), fullSnapLease)).To(Succeed())

			// Deliberately update the delta lease
			deltaSnapLease.Spec.HolderIdentity = ptr.To("101")
			deltaSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), deltaSnapLease)).To(Succeed())

			j = &batchv1.Job{}
			Eventually(func() error {
				return jobIsCorrectlyReconciled(k8sClient, instance, j)
			}, timeout, pollingInterval).Should(BeNil())

			// Update job status as succeeded
			j.Status = batchv1.JobStatus{
				Succeeded: 1,
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), j)).To(Succeed())

			// Wait until the job gets the "foregroundDeletion" finalizer and remove it
			Eventually(func() (*batchv1.Job, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(j), j); err != nil {
					return nil, err
				}
				return j, nil
			}, timeout, pollingInterval).Should(PointTo(testutils.MatchFinalizer(metav1.FinalizerDeleteDependents)))
			Expect(kubernetes.RemoveFinalizers(ctx, k8sClient, j, metav1.FinalizerDeleteDependents)).To(Succeed())

			// Wait until the job has been deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(j), &batchv1.Job{})
			}, timeout, pollingInterval).Should(testutils.BeNotFoundError())
		})

		It("should let the existing job run if the job is active", func() {
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			instance = testutils.EtcdBuilderWithDefaults("foo79", "default").WithProviderLocal().Build()
			createEtcdAndWait(k8sClient, instance)

			// manually create full and delta snapshot leases since etcd controller is not running
			fullSnapLease, deltaSnapLease = createEtcdSnapshotLeasesAndWait(k8sClient, instance)

			// Deliberately update the full lease
			fullSnapLease.Spec.HolderIdentity = ptr.To("0")
			fullSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), fullSnapLease)).To(Succeed())

			// Deliberately update the delta lease
			deltaSnapLease.Spec.HolderIdentity = ptr.To("101")
			deltaSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), deltaSnapLease)).To(Succeed())

			j = &batchv1.Job{}
			Eventually(func() error {
				return jobIsCorrectlyReconciled(k8sClient, instance, j)
			}, timeout, pollingInterval).Should(BeNil())

			// Update job status as active
			j.Status.Active = 1
			Expect(k8sClient.Status().Update(ctx, j)).To(Succeed())

			// The active job should exist
			Eventually(func() error {
				if err := jobIsCorrectlyReconciled(k8sClient, instance, j); err != nil {
					return err
				}
				if j.Status.Active != 1 {
					return fmt.Errorf("compaction job is not currently active")
				}
				return nil
			}, timeout, pollingInterval).Should(BeNil())
		})

		It("should let the existing job run even without any lease update", func() {
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			instance = testutils.EtcdBuilderWithDefaults("foo80", "default").WithProviderLocal().Build()
			createEtcdAndWait(k8sClient, instance)

			// manually create full and delta snapshot leases since etcd controller is not running
			fullSnapLease, deltaSnapLease = createEtcdSnapshotLeasesAndWait(k8sClient, instance)

			// Deliberately update the full lease
			fullSnapLease.Spec.HolderIdentity = ptr.To("0")
			fullSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), fullSnapLease)).To(Succeed())

			// Deliberately update the delta lease
			deltaSnapLease.Spec.HolderIdentity = ptr.To("101")
			deltaSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), deltaSnapLease)).To(Succeed())

			j = &batchv1.Job{}
			Eventually(func() error {
				return jobIsCorrectlyReconciled(k8sClient, instance, j)
			}, timeout, pollingInterval).Should(BeNil())

			// Update job status as active
			j.Status.Active = 1
			Expect(k8sClient.Status().Update(ctx, j)).To(Succeed())

			// The active job should exist
			Eventually(func() error {
				if err := jobIsCorrectlyReconciled(k8sClient, instance, j); err != nil {
					return err
				}
				if j.Status.Active != 1 {
					return fmt.Errorf("compaction job is not currently active")
				}
				return nil
			}, timeout, pollingInterval).Should(BeNil())
		})
	})
})

func validateEtcdForCompactionJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	store, err := druidstore.StorageProviderFromInfraProvider(instance.Spec.Backup.Store.Provider)
	Expect(err).NotTo(HaveOccurred())

	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(druidv1alpha1.GetCompactionJobName(instance.ObjectMeta)),
			"Namespace": Equal(instance.Namespace),
			"OwnerReferences": MatchElements(testutils.OwnerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal(druidv1alpha1.SchemeGroupVersion.String()),
					"Kind":               Equal("Etcd"),
					"Name":               Equal(instance.Name),
					"UID":                Equal(instance.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"BackoffLimit": PointTo(Equal(int32(0))),
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"RestartPolicy": Equal(corev1.RestartPolicyNever),
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--data-dir=/var/etcd/data/compaction.etcd":                                                                       Equal("--data-dir=/var/etcd/data/compaction.etcd"),
								"--restoration-temp-snapshots-dir=/var/etcd/data/compaction.restoration.temp":                                     Equal("--restoration-temp-snapshots-dir=/var/etcd/data/compaction.restoration.temp"),
								"--snapstore-temp-directory=/var/etcd/data/tmp":                                                                   Equal("--snapstore-temp-directory=/var/etcd/data/tmp"),
								"--metrics-scrape-wait-duration=1m0s":                                                                             Equal("--metrics-scrape-wait-duration=1m0s"),
								"--enable-snapshot-lease-renewal=true":                                                                            Equal("--enable-snapshot-lease-renewal=true"),
								fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", druidv1alpha1.GetFullSnapshotLeaseName(instance.ObjectMeta)):   Equal(fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", druidv1alpha1.GetFullSnapshotLeaseName(instance.ObjectMeta))),
								fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", druidv1alpha1.GetDeltaSnapshotLeaseName(instance.ObjectMeta)): Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", druidv1alpha1.GetDeltaSnapshotLeaseName(instance.ObjectMeta))),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):                                         Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--storage-provider", store):                                                                 Equal(fmt.Sprintf("%s=%s", "--storage-provider", store)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container):                                  Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", instance.Spec.Etcd.Quota.Value()):                                   Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", instance.Spec.Etcd.Quota.Value())),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String()):       Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String()):             Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String())),
							}),
							"Image":           Equal(*instance.Spec.Backup.Image),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"VolumeMounts": MatchElements(testutils.VolumeMountIterator, IgnoreExtras, Elements{
								"etcd-workspace-dir": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-workspace-dir"),
									"MountPath": Equal("/var/etcd/data"),
								}),
							}),
							"Env": MatchElements(testutils.EnvIterator, IgnoreExtras, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
							}),
						}),
					}),
					"Volumes": MatchAllElements(testutils.VolumeIterator, Elements{
						"etcd-workspace-dir": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-workspace-dir"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"HostPath": BeNil(),
								"EmptyDir": PointTo(MatchFields(IgnoreExtras, Fields{
									"SizeLimit": BeNil(),
								})),
							}),
						}),
						"host-storage": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("host-storage"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"HostPath": PointTo(MatchFields(IgnoreExtras, Fields{
									"Path": Equal("/etc/gardener/local-backupbuckets/default.bkp"),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreGCPForCompactionJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=GCS": Equal("--storage-provider=GCS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"VolumeMounts": MatchElements(testutils.VolumeMountIterator, IgnoreExtras, Elements{
								common.VolumeNameProviderBackupSecret: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(common.VolumeNameProviderBackupSecret),
									"MountPath": Equal(common.VolumeMountPathGCSBackupSecret),
								}),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvGoogleApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvGoogleApplicationCredentials),
									"Value": Equal("/var/.gcp/serviceaccount.json"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						common.VolumeNameProviderBackupSecret: MatchFields(IgnoreExtras, Fields{
							"Name": Equal(common.VolumeNameProviderBackupSecret),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Backup.Store.SecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAWSForCompactionJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=S3": Equal("--storage-provider=S3"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvAWSApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvAWSApplicationCredentials),
									"Value": Equal(common.VolumeMountPathNonGCSProviderBackupSecret),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						common.VolumeNameProviderBackupSecret: MatchFields(IgnoreExtras, Fields{
							"Name": Equal(common.VolumeNameProviderBackupSecret),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Backup.Store.SecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAzureForCompactionJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=ABS": Equal("--storage-provider=ABS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvAzureApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvAzureApplicationCredentials),
									"Value": Equal(common.VolumeMountPathNonGCSProviderBackupSecret),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						common.VolumeNameProviderBackupSecret: MatchFields(IgnoreExtras, Fields{
							"Name": Equal(common.VolumeNameProviderBackupSecret),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Backup.Store.SecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreOpenstackForCompactionJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=Swift": Equal("--storage-provider=Swift"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvOpenstackApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvOpenstackApplicationCredentials),
									"Value": Equal(common.VolumeMountPathNonGCSProviderBackupSecret),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						common.VolumeNameProviderBackupSecret: MatchFields(IgnoreExtras, Fields{
							"Name": Equal(common.VolumeNameProviderBackupSecret),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Backup.Store.SecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAlicloudForCompactionJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=OSS": Equal("--storage-provider=OSS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvAlicloudApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvAlicloudApplicationCredentials),
									"Value": Equal(common.VolumeMountPathNonGCSProviderBackupSecret),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						common.VolumeNameProviderBackupSecret: MatchFields(IgnoreExtras, Fields{
							"Name": Equal(common.VolumeNameProviderBackupSecret),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Backup.Store.SecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func jobIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, job *batchv1.Job) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	req := types.NamespacedName{
		Name:      druidv1alpha1.GetCompactionJobName(instance.ObjectMeta),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, job); err != nil {
		return err
	}

	if job.Status.Failed > 0 {
		return fmt.Errorf("job is running but it is in failed state")
	}

	return nil
}

func etcdSnapshotLeaseIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, lease *coordinationv1.Lease) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	if err := c.Get(ctx, client.ObjectKeyFromObject(lease), lease); err != nil {
		return err
	}

	if !testutils.CheckEtcdOwnerReference(lease.GetOwnerReferences(), instance.UID) {
		return fmt.Errorf("ownerReference does not exists for lease")
	}
	return nil
}

func createEtcdAndWait(c client.Client, etcd *druidv1alpha1.Etcd) {
	By("create etcd instance")
	Expect(c.Create(context.TODO(), etcd)).To(Succeed())

	// wait for etcd to be created
	Eventually(func() error {
		return c.Get(context.TODO(), client.ObjectKeyFromObject(etcd), etcd)
	}, timeout, pollingInterval).Should(BeNil())
}

func deleteEtcdAndWait(c client.Client, etcd *druidv1alpha1.Etcd) {
	By("delete etcd instance")
	Expect(c.Delete(context.TODO(), etcd)).To(Succeed())

	// wait for etcd to be deleted
	Eventually(func() error {
		return testutils.IsEtcdRemoved(c, etcd.Name, etcd.Namespace, timeout)
	}, timeout, pollingInterval).Should(BeNil())
}

func createEtcdSnapshotLeasesAndWait(c client.Client, etcd *druidv1alpha1.Etcd) (*coordinationv1.Lease, *coordinationv1.Lease) {
	By("create full snapshot lease")
	fullSnapLease := testutils.CreateLease(druidv1alpha1.GetFullSnapshotLeaseName(etcd.ObjectMeta), etcd.Namespace, etcd.Name, etcd.UID, "etcd-snapshot-lease")
	Expect(c.Create(context.TODO(), fullSnapLease)).To(Succeed())

	By("create delta snapshot lease")
	deltaSnapLease := testutils.CreateLease(druidv1alpha1.GetDeltaSnapshotLeaseName(etcd.ObjectMeta), etcd.Namespace, etcd.Name, etcd.UID, "etcd-snapshot-lease")
	Expect(c.Create(context.TODO(), deltaSnapLease)).To(Succeed())

	// wait for full snapshot lease to be created
	Eventually(func() error { return etcdSnapshotLeaseIsCorrectlyReconciled(c, etcd, fullSnapLease) }, timeout, pollingInterval).Should(BeNil())

	// wait for delta snapshot lease to be created
	Eventually(func() error { return etcdSnapshotLeaseIsCorrectlyReconciled(c, etcd, deltaSnapLease) }, timeout, pollingInterval).Should(BeNil())

	return fullSnapLease, deltaSnapLease
}

func deleteEtcdSnapshotLeasesAndWait(c client.Client, etcd *druidv1alpha1.Etcd) {
	By("delete full snapshot lease")
	fullSnapLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      druidv1alpha1.GetFullSnapshotLeaseName(etcd.ObjectMeta),
			Namespace: etcd.Namespace,
		},
	}
	Expect(c.Delete(context.TODO(), fullSnapLease)).To(Succeed())

	By("delete delta snapshot lease")
	deltaSnapLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      druidv1alpha1.GetDeltaSnapshotLeaseName(etcd.ObjectMeta),
			Namespace: etcd.Namespace,
		},
	}
	Expect(c.Delete(context.TODO(), deltaSnapLease)).To(Succeed())

	// wait for full snapshot lease to be created
	Eventually(func() error {
		return testutils.IsLeaseRemoved(c, fullSnapLease.Name, fullSnapLease.Namespace, timeout)
	}, timeout, pollingInterval).Should(BeNil())

	// wait for delta snapshot lease to be created
	Eventually(func() error {
		return testutils.IsLeaseRemoved(c, deltaSnapLease.Name, deltaSnapLease.Namespace, timeout)
	}, timeout, pollingInterval).Should(BeNil())
}
