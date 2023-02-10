// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package compaction

import (
	"context"
	"fmt"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	timeout         = time.Minute * 2
	pollingInterval = time.Second * 2
)

var _ = Describe("Lease Controller", func() {

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

			instance = testutils.EtcdBuilderWithDefaults(name, namespace).WithTLS().WithStorageProvider(provider).Build()
			createEtcdAndWait(k8sClient, instance)

			// manually create full and delta snapshot leases since etcd controller is not running
			fullSnapLease, deltaSnapLease = createEtcdSnapshotLeasesAndWait(k8sClient, instance)

			// manually update full lease spec since there is no running instance of etcd-backup-restore
			By("update full snapshot lease")
			fullSnapLease.Spec.HolderIdentity = pointer.String("0")
			fullSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), fullSnapLease)).To(Succeed())

			// manually update delta lease spec since there is no running instance of etcd-backup-restore
			By("update delta snapshot lease")
			deltaSnapLease.Spec.HolderIdentity = pointer.String("101")
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
		Entry("if fields are set in etcd.Spec and TLS enabled, the resources should reflect the spec changes", "foo71", druidv1alpha1.StorageProvider("Local"), validateEtcdForCompactionJob),
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

			instance = testutils.EtcdBuilderWithDefaults("foo77", namespace).Build()
			createEtcdAndWait(k8sClient, instance)

			// manually create full and delta snapshot leases since etcd controller is not running
			fullSnapLease, deltaSnapLease = createEtcdSnapshotLeasesAndWait(k8sClient, instance)

			// manually create compaction job
			j = createCompactionJob(instance)
			Expect(k8sClient.Create(ctx, j)).To(Succeed())

			Eventually(func() error { return jobIsCorrectlyReconciled(k8sClient, instance, j) }, timeout, pollingInterval).Should(BeNil())

			// Update job status as failed
			j.Status.Failed = 1
			Expect(k8sClient.Status().Update(ctx, j)).To(Succeed())

			// Deliberately update the full lease
			fullSnapLease.Spec.HolderIdentity = pointer.String("0")
			fullSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), fullSnapLease)).To(Succeed())

			// Deliberately update the delta lease
			deltaSnapLease.Spec.HolderIdentity = pointer.String("101")
			deltaSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), deltaSnapLease)).To(Succeed())

			// Wait until the job gets the "foregroundDeletion" finalizer and remove it
			Eventually(func() (*batchv1.Job, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(j), j); err != nil {
					return nil, err
				}
				return j, nil
			}, timeout, pollingInterval).Should(PointTo(testutils.MatchFinalizer(metav1.FinalizerDeleteDependents)))
			Expect(controllerutils.RemoveFinalizers(ctx, k8sClient, j, metav1.FinalizerDeleteDependents)).To(Succeed())

			// Wait until the job has been deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(j), &batchv1.Job{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

			// A new job should be created
			j = &batchv1.Job{}
			Eventually(func() error { return jobIsCorrectlyReconciled(k8sClient, instance, j) }, timeout, pollingInterval).Should(BeNil())
		})

		It("should delete the existing job if the job is succeeded", func() {
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			instance = testutils.EtcdBuilderWithDefaults("foo78", "default").Build()
			createEtcdAndWait(k8sClient, instance)

			// manually create full and delta snapshot leases since etcd controller is not running
			fullSnapLease, deltaSnapLease = createEtcdSnapshotLeasesAndWait(k8sClient, instance)

			// create compaction job
			j := createCompactionJob(instance)
			Expect(k8sClient.Create(ctx, j)).To(Succeed())

			Eventually(func() error { return jobIsCorrectlyReconciled(k8sClient, instance, j) }, timeout, pollingInterval).Should(BeNil())

			// Update job status as succeeded
			j.Status.Succeeded = 1
			Expect(k8sClient.Status().Update(context.TODO(), j)).To(Succeed())

			// Deliberately update the full lease
			fullSnapLease.Spec.HolderIdentity = pointer.String("0")
			fullSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), fullSnapLease)).To(Succeed())

			// Deliberately update the delta lease
			deltaSnapLease.Spec.HolderIdentity = pointer.String("101")
			deltaSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), deltaSnapLease)).To(Succeed())

			// Wait until the job gets the "foregroundDeletion" finalizer and remove it
			Eventually(func() (*batchv1.Job, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(j), j); err != nil {
					return nil, err
				}
				return j, nil
			}, timeout, pollingInterval).Should(PointTo(testutils.MatchFinalizer(metav1.FinalizerDeleteDependents)))
			Expect(controllerutils.RemoveFinalizers(ctx, k8sClient, j, metav1.FinalizerDeleteDependents)).To(Succeed())

			// Wait until the job has been deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(j), &batchv1.Job{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
		})

		It("should let the existing job run if the job is active", func() {
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			instance = testutils.EtcdBuilderWithDefaults("foo79", "default").Build()
			createEtcdAndWait(k8sClient, instance)

			// manually create full and delta snapshot leases since etcd controller is not running
			fullSnapLease, deltaSnapLease = createEtcdSnapshotLeasesAndWait(k8sClient, instance)

			// create compaction job
			j := createCompactionJob(instance)
			Expect(k8sClient.Create(ctx, j)).To(Succeed())

			Eventually(func() error { return jobIsCorrectlyReconciled(k8sClient, instance, j) }, timeout, pollingInterval).Should(BeNil())

			// Update job status as active
			j.Status.Active = 1
			Expect(k8sClient.Status().Update(ctx, j)).To(Succeed())

			// Deliberately update the delta lease
			deltaSnapLease.Spec.HolderIdentity = pointer.String("100")
			deltaSnapLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			Expect(k8sClient.Update(context.TODO(), deltaSnapLease)).To(Succeed())

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
	store, err := utils.StorageProviderFromInfraProvider(instance.Spec.Backup.Store.Provider)
	Expect(err).NotTo(HaveOccurred())

	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(instance.GetCompactionJobName()),
			"Namespace": Equal(instance.Namespace),
			"OwnerReferences": MatchElements(testutils.OwnerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal("druid.gardener.cloud/v1alpha1"),
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
							"Command": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--data-dir=/var/etcd/data":                                                                                 Equal("--data-dir=/var/etcd/data"),
								"--snapstore-temp-directory=/var/etcd/data/tmp":                                                             Equal("--snapstore-temp-directory=/var/etcd/data/tmp"),
								"--enable-snapshot-lease-renewal=true":                                                                      Equal("--enable-snapshot-lease-renewal=true"),
								fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", instance.GetFullSnapshotLeaseName()):                     Equal(fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", instance.GetFullSnapshotLeaseName())),
								fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", instance.GetDeltaSnapshotLeaseName()):                   Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", instance.GetDeltaSnapshotLeaseName())),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):                                   Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--storage-provider", store):                                                           Equal(fmt.Sprintf("%s=%s", "--storage-provider", store)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container):                            Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", instance.Spec.Etcd.Quota.Value()):                             Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", instance.Spec.Etcd.Quota.Value())),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String()): Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String()):       Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String())),
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
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"POD_NAMESPACE": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("POD_NAMESPACE"),
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
							"Command": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=GCS": Equal("--storage-provider=GCS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"VolumeMounts": MatchElements(testutils.VolumeMountIterator, IgnoreExtras, Elements{
								"etcd-backup": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-backup"),
									"MountPath": Equal("/root/.gcp/"),
								}),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"POD_NAMESPACE": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("POD_NAMESPACE"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								"GOOGLE_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("GOOGLE_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/.gcp/serviceaccount.json"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
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
							"Command": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=S3": Equal("--storage-provider=S3"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"POD_NAMESPACE": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("POD_NAMESPACE"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								"AWS_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("AWS_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/etcd-backup"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
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
							"Command": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=ABS": Equal("--storage-provider=ABS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"POD_NAMESPACE": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("POD_NAMESPACE"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								"AZURE_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("AZURE_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/etcd-backup"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
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
							"Command": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=Swift": Equal("--storage-provider=Swift"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"POD_NAMESPACE": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("POD_NAMESPACE"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								"OPENSTACK_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("OPENSTACK_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/etcd-backup"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
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
							"Command": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=OSS": Equal("--storage-provider=OSS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"POD_NAMESPACE": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("POD_NAMESPACE"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								"ALICLOUD_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("ALICLOUD_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/etcd-backup"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
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
		Name:      instance.GetCompactionJobName(),
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

func createCompactionJob(instance *druidv1alpha1.Etcd) *batchv1.Job {
	j := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.GetCompactionJobName(),
			Namespace: instance.Namespace,
			Labels:    instance.Labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: pointer.Int32(0),
			Completions:  pointer.Int32(1),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: instance.Labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: "Never",
					Containers: []corev1.Container{
						{
							Name:    "compact-backup",
							Image:   "eu.gcr.io/gardener-project/alpine:3.14",
							Command: []string{"sh", "-c", "tail -f /dev/null"},
						},
					},
				},
			},
		},
	}
	return &j
}

func etcdSnapshotLeaseIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, lease *coordinationv1.Lease) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	if err := c.Get(ctx, client.ObjectKeyFromObject(lease), lease); err != nil {
		return err
	}

	if !testutils.CheckEtcdOwnerReference(lease.GetOwnerReferences(), instance) {
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
	fullSnapLease := testutils.CreateLease(etcd.GetFullSnapshotLeaseName(), etcd.Namespace, etcd.Name, etcd.UID)
	Expect(c.Create(context.TODO(), fullSnapLease)).To(Succeed())

	By("create delta snapshot lease")
	deltaSnapLease := testutils.CreateLease(etcd.GetDeltaSnapshotLeaseName(), etcd.Namespace, etcd.Name, etcd.UID)
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
			Name:      etcd.GetFullSnapshotLeaseName(),
			Namespace: etcd.Namespace,
		},
	}
	Expect(c.Delete(context.TODO(), fullSnapLease)).To(Succeed())

	By("delete delta snapshot lease")
	deltaSnapLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.GetDeltaSnapshotLeaseName(),
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
