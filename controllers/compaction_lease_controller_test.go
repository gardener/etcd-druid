// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package controllers

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Lease Controller", func() {
	Context("when fields are not set in etcd.Spec", func() {
		var (
			err      error
			instance *druidv1alpha1.Etcd
			c        client.Client
			s        *appsv1.StatefulSet
			cm       *corev1.ConfigMap
			svc      *corev1.Service
		)
		BeforeEach(func() {
			instance = getEtcdWithDefault("foo333", "default")
			c = mgr.GetClient()
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}

			_, err = controllerutil.CreateOrUpdate(context.TODO(), c, &ns, func() error { return nil })
			Expect(err).To(Not(HaveOccurred()))

			err = c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			s = &appsv1.StatefulSet{}
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, s) }, timeout, pollingInterval).Should(BeNil())
			cm = &corev1.ConfigMap{}
			Eventually(func() error { return configMapIsCorrectlyReconciled(c, instance, cm) }, timeout, pollingInterval).Should(BeNil())
			svc = &corev1.Service{}
			Eventually(func() error { return clientServiceIsCorrectlyReconciled(c, instance, svc) }, timeout, pollingInterval).Should(BeNil())
		})

		AfterEach(func() {
			Expect(c.Delete(context.TODO(), instance)).To(Succeed())
			Eventually(func() error { return statefulSetRemoved(c, s) }, timeout, pollingInterval).Should(BeNil())
			Eventually(func() error { return etcdRemoved(c, instance) }, timeout, pollingInterval).Should(BeNil())
		})
	})

	// When an ETCD resource is created, check if the associated compaction job is created with validateETCDCmpctJob
	DescribeTable("when etcd resource is created",
		func(name string,
			generateEtcd func(string, string) *druidv1alpha1.Etcd,
			validateETCDCmpctJob func(*druidv1alpha1.Etcd, *batchv1.Job)) {
			var (
				err      error
				instance *druidv1alpha1.Etcd
				c        client.Client
				j        *batchv1.Job
			)

			instance = generateEtcd(name, "default")
			c = mgr.GetClient()
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}

			_, err = controllerutil.CreateOrUpdate(context.TODO(), c, &ns, func() error { return nil })
			Expect(err).To(Not(HaveOccurred()))

			if instance.Spec.Backup.Store != nil && instance.Spec.Backup.Store.SecretRef != nil {
				storeSecret := instance.Spec.Backup.Store.SecretRef.Name
				errors := createSecrets(c, instance.Namespace, storeSecret)
				Expect(len(errors)).Should(BeZero())
			}
			err = c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())

			// Verify if the job is created when difference between holder identities in delta-snapshot-revision and full-snapshot-revision is greater than 1M
			fullLease := &coordinationv1.Lease{}
			Eventually(func() error { return fullLeaseIsCorrectlyReconciled(c, instance, fullLease) }, timeout, pollingInterval).Should(BeNil())
			err = controllerutils.TryUpdate(context.TODO(), retry.DefaultBackoff, c, fullLease, func() error {
				fullLease.Spec.HolderIdentity = pointer.StringPtr("0")
				renewedTime := time.Now()
				fullLease.Spec.RenewTime = &metav1.MicroTime{Time: renewedTime}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			deltaLease := &coordinationv1.Lease{}
			Eventually(func() error { return deltaLeaseIsCorrectlyReconciled(c, instance, deltaLease) }, timeout, pollingInterval).Should(BeNil())
			err = controllerutils.TryUpdate(context.TODO(), retry.DefaultBackoff, c, deltaLease, func() error {
				deltaLease.Spec.HolderIdentity = pointer.StringPtr("1000000")
				renewedTime := time.Now()
				deltaLease.Spec.RenewTime = &metav1.MicroTime{Time: renewedTime}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			j = &batchv1.Job{}
			Eventually(func() error { return jobIsCorrectlyReconciled(c, instance, j) }, timeout, pollingInterval).Should(BeNil())

			validateETCDCmpctJob(instance, j)

			Expect(c.Delete(context.TODO(), instance)).To(Succeed())
			Eventually(func() error {
				return c.Get(context.TODO(), client.ObjectKeyFromObject(instance), &druidv1alpha1.Etcd{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
		},
		Entry("if fields are set in etcd.Spec and TLS enabled, the resources should reflect the spec changes", "foo71", getEtcdWithTLS, validateEtcdForCmpctJob),
		Entry("if the store is GCS, the statefulset and compaction job should reflect the spec changes", "foo72", getEtcdWithGCS, validateStoreGCPForCmpctJob),
		Entry("if the store is S3, the statefulset and compaction job should reflect the spec changes", "foo73", getEtcdWithS3, validateStoreAWSForCmpctJob),
		Entry("if the store is ABS, the statefulset and compaction job should reflect the spec changes", "foo74", getEtcdWithABS, validateStoreAzureForCmpctJob),
		Entry("if the store is Swift, the statefulset and compaction job should reflect the spec changes", "foo75", getEtcdWithSwift, validateStoreOpenstackForCmpctJob),
		Entry("if the store is OSS, the statefulset and compaction job should reflect the spec changes", "foo76", getEtcdWithOSS, validateStoreAlicloudForCmpctJob),
	)

	Context("when an existing job is already present", func() {
		var (
			err      error
			instance *druidv1alpha1.Etcd
			c        client.Client
			ns       corev1.Namespace
		)

		BeforeEach(func() {

		})

		It("should create a new job if the existing job is failed", func() {
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			instance = getEtcd("foo77", "default", false)
			c = mgr.GetClient()
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}

			_, err = controllerutil.CreateOrUpdate(context.TODO(), c, &ns, func() error { return nil })
			Expect(err).To(Not(HaveOccurred()))

			if instance.Spec.Backup.Store != nil && instance.Spec.Backup.Store.SecretRef != nil {
				storeSecret := instance.Spec.Backup.Store.SecretRef.Name
				errors := createSecrets(c, instance.Namespace, storeSecret)
				Expect(len(errors)).Should(BeZero())
			}
			err = c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())

			// Create Job
			j := createJob(instance)
			Expect(c.Create(ctx, j)).To(Succeed())

			Eventually(func() error { return jobIsCorrectlyReconciled(c, instance, j) }, timeout, pollingInterval).Should(BeNil())

			// Update job status as failed
			err = controllerutils.TryUpdateStatus(ctx, retry.DefaultBackoff, c, j, func() error {
				j.Status.Failed = 1
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			// Deliberately update the full lease
			fullLease := &coordinationv1.Lease{}
			Eventually(func() error { return fullLeaseIsCorrectlyReconciled(c, instance, fullLease) }, timeout, pollingInterval).Should(BeNil())
			err = controllerutils.TryUpdate(context.TODO(), retry.DefaultBackoff, c, fullLease, func() error {
				fullLease.Spec.HolderIdentity = pointer.StringPtr("0")
				renewedTime := time.Now()
				fullLease.Spec.RenewTime = &metav1.MicroTime{Time: renewedTime}
				return nil
			})

			// Deliberately update the delta lease
			deltaLease := &coordinationv1.Lease{}
			Eventually(func() error { return deltaLeaseIsCorrectlyReconciled(c, instance, deltaLease) }, timeout, pollingInterval).Should(BeNil())
			err = controllerutils.TryUpdate(context.TODO(), retry.DefaultBackoff, c, deltaLease, func() error {
				deltaLease.Spec.HolderIdentity = pointer.StringPtr("1000000")
				renewedTime := time.Now()
				deltaLease.Spec.RenewTime = &metav1.MicroTime{Time: renewedTime}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait until the job gets the "foregroundDeletion" finalizer and remove it
			Eventually(func() (*batchv1.Job, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(j), j); err != nil {
					return nil, err
				}
				return j, nil
			}, timeout, pollingInterval).Should(PointTo(matchFinalizer(metav1.FinalizerDeleteDependents)))
			Expect(controllerutils.PatchRemoveFinalizers(ctx, c, j, metav1.FinalizerDeleteDependents)).To(Succeed())

			// Wait until the job has been deleted
			Eventually(func() error {
				return c.Get(ctx, client.ObjectKeyFromObject(j), &batchv1.Job{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

			//Instead of failed one a new job should be created
			j = &batchv1.Job{}
			Eventually(func() error { return jobIsCorrectlyReconciled(c, instance, j) }, timeout, pollingInterval).Should(BeNil())
		})

		It("should delete the existing job if the job is succeeded", func() {
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			instance = getEtcd("foo78", "default", false)
			c = mgr.GetClient()
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}

			_, err = controllerutil.CreateOrUpdate(context.TODO(), c, &ns, func() error { return nil })
			Expect(err).To(Not(HaveOccurred()))

			if instance.Spec.Backup.Store != nil && instance.Spec.Backup.Store.SecretRef != nil {
				storeSecret := instance.Spec.Backup.Store.SecretRef.Name
				errors := createSecrets(c, instance.Namespace, storeSecret)
				Expect(len(errors)).Should(BeZero())
			}
			err = c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())

			// Create Job
			j := createJob(instance)
			Expect(c.Create(ctx, j)).To(Succeed())

			Eventually(func() error { return jobIsCorrectlyReconciled(c, instance, j) }, timeout, pollingInterval).Should(BeNil())

			// Update job status as succeeded
			err = controllerutils.TryUpdateStatus(ctx, retry.DefaultBackoff, c, j, func() error {
				j.Status.Succeeded = 1
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			// Deliberately update the full lease
			fullLease := &coordinationv1.Lease{}
			Eventually(func() error { return fullLeaseIsCorrectlyReconciled(c, instance, fullLease) }, timeout, pollingInterval).Should(BeNil())
			err = controllerutils.TryUpdate(context.TODO(), retry.DefaultBackoff, c, fullLease, func() error {
				fullLease.Spec.HolderIdentity = pointer.StringPtr("0")
				renewedTime := time.Now()
				fullLease.Spec.RenewTime = &metav1.MicroTime{Time: renewedTime}
				return nil
			})

			Eventually(func() error {
				if err := c.Get(ctx, client.ObjectKeyFromObject(fullLease), fullLease); err != nil {
					return err
				}
				if fullLease.Spec.HolderIdentity != nil {
					return nil
				}
				return fmt.Errorf("no HolderIdentity")
			}, 10*time.Second, pollingInterval)

			// Deliberately update the delta lease
			deltaLease := &coordinationv1.Lease{}
			Eventually(func() error { return deltaLeaseIsCorrectlyReconciled(c, instance, deltaLease) }, timeout, pollingInterval).Should(BeNil())
			err = controllerutils.TryUpdate(context.TODO(), retry.DefaultBackoff, c, deltaLease, func() error {
				deltaLease.Spec.HolderIdentity = pointer.StringPtr("1000000")
				renewedTime := time.Now()
				deltaLease.Spec.RenewTime = &metav1.MicroTime{Time: renewedTime}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait until the job gets the "foregroundDeletion" finalizer and remove it
			Eventually(func() (*batchv1.Job, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(j), j); err != nil {
					return nil, err
				}
				return j, nil
			}, timeout, pollingInterval).Should(PointTo(matchFinalizer(metav1.FinalizerDeleteDependents)))
			Expect(controllerutils.PatchRemoveFinalizers(ctx, c, j, metav1.FinalizerDeleteDependents)).To(Succeed())

			// Wait until the job has been deleted
			Eventually(func() error {
				return c.Get(ctx, client.ObjectKeyFromObject(j), &batchv1.Job{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
		})

		It("should let the existing job run if the job is active", func() {
			ctx, cancel := context.WithTimeout(context.TODO(), timeout)
			defer cancel()

			instance = getEtcd("foo79", "default", false)
			c = mgr.GetClient()
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}

			_, err = controllerutil.CreateOrUpdate(context.TODO(), c, &ns, func() error { return nil })
			Expect(err).To(Not(HaveOccurred()))

			if instance.Spec.Backup.Store != nil && instance.Spec.Backup.Store.SecretRef != nil {
				storeSecret := instance.Spec.Backup.Store.SecretRef.Name
				errors := createSecrets(c, instance.Namespace, storeSecret)
				Expect(len(errors)).Should(BeZero())
			}
			err = c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())

			// Create Job
			j := createJob(instance)
			Expect(c.Create(ctx, j)).To(Succeed())

			Eventually(func() error { return jobIsCorrectlyReconciled(c, instance, j) }, timeout, pollingInterval).Should(BeNil())

			// Update job status as active
			err = controllerutils.TryUpdateStatus(ctx, retry.DefaultBackoff, c, j, func() error {
				j.Status.Active = 1
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			// Deliberately update the delta lease
			deltaLease := &coordinationv1.Lease{}
			Eventually(func() error { return deltaLeaseIsCorrectlyReconciled(c, instance, deltaLease) }, timeout, pollingInterval).Should(BeNil())
			err = controllerutils.TryUpdate(context.TODO(), retry.DefaultBackoff, c, deltaLease, func() error {
				deltaLease.Spec.HolderIdentity = pointer.StringPtr("1000000")
				renewedTime := time.Now()
				deltaLease.Spec.RenewTime = &metav1.MicroTime{Time: renewedTime}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			// The active job should exist
			Eventually(func() error { return jobIsCorrectlyReconciled(c, instance, j) }, timeout, pollingInterval).Should(BeNil())
		})

		AfterEach(func() {
			Expect(c.Delete(context.TODO(), instance)).To(Succeed())
			Eventually(func() error { return etcdRemoved(c, instance) }, timeout, pollingInterval).Should(BeNil())
		})
	})
})

func validateEtcdForCmpctJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	store, err := utils.StorageProviderFromInfraProvider(instance.Spec.Backup.Store.Provider)
	Expect(err).NotTo(HaveOccurred())

	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(utils.GetJobName(instance)),
			"Namespace": Equal(instance.Namespace),
			"OwnerReferences": MatchElements(ownerRefIterator, IgnoreExtras, Elements{
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
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--data-dir=/var/etcd/data":                     Equal("--data-dir=/var/etcd/data"),
								"--snapstore-temp-directory=/var/etcd/data/tmp": Equal("--snapstore-temp-directory=/var/etcd/data/tmp"),
								"--enable-snapshot-lease-renewal=true":          Equal("--enable-snapshot-lease-renewal=true"),
								fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", utils.GetFullSnapshotLeaseName(instance)):                Equal(fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", utils.GetFullSnapshotLeaseName(instance))),
								fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", utils.GetDeltaSnapshotLeaseName(instance)):              Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", utils.GetDeltaSnapshotLeaseName(instance))),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):                                   Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--storage-provider", store):                                                           Equal(fmt.Sprintf("%s=%s", "--storage-provider", store)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container):                            Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value())):                      Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value()))),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String()): Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String()):       Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String())),
							}),
							"Image":           Equal(*instance.Spec.Backup.Image),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
								"etcd-workspace-dir": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-workspace-dir"),
									"MountPath": Equal("/var/etcd/data"),
								}),
							}),
							"Env": MatchElements(envIterator, IgnoreExtras, Elements{
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
					"Volumes": MatchAllElements(volumeIterator, Elements{
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

func validateStoreGCPForCmpctJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=GCS": Equal("--storage-provider=GCS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
								"etcd-backup": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-backup"),
									"MountPath": Equal("/root/.gcp/"),
								}),
							}),
							"Env": MatchAllElements(envIterator, Elements{
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
					"Volumes": MatchElements(volumeIterator, IgnoreExtras, Elements{
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

func validateStoreAWSForCmpctJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=S3": Equal("--storage-provider=S3"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"Env": MatchAllElements(envIterator, Elements{
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
					"Volumes": MatchElements(volumeIterator, IgnoreExtras, Elements{
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

func validateStoreAzureForCmpctJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=ABS": Equal("--storage-provider=ABS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"Env": MatchAllElements(envIterator, Elements{
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
					"Volumes": MatchElements(volumeIterator, IgnoreExtras, Elements{
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

func validateStoreOpenstackForCmpctJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=Swift": Equal("--storage-provider=Swift"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"Env": MatchAllElements(envIterator, Elements{
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
					"Volumes": MatchElements(volumeIterator, IgnoreExtras, Elements{
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

func validateStoreAlicloudForCmpctJob(instance *druidv1alpha1.Etcd, j *batchv1.Job) {
	Expect(*j).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						"compact-backup": MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=OSS": Equal("--storage-provider=OSS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Env": MatchAllElements(envIterator, Elements{
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
					"Volumes": MatchElements(volumeIterator, IgnoreExtras, Elements{
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
		Name:      utils.GetJobName(instance),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, job); err != nil {
		return err
	}

	if job.Status.Failed > 0 {
		return fmt.Errorf("Job is running but it's failed")
	}

	return nil
}

func createJob(instance *druidv1alpha1.Etcd) *batchv1.Job {
	j := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.GetJobName(instance),
			Namespace: instance.Namespace,
			Labels:    instance.Labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: pointer.Int32Ptr(0),
			Completions:  pointer.Int32Ptr(1),
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

func fullLeaseIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, lease *coordinationv1.Lease) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      utils.GetFullSnapshotLeaseName(instance),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, lease); err != nil {
		return err
	}

	if !checkEtcdOwnerReference(lease.GetOwnerReferences(), instance) {
		return fmt.Errorf("ownerReference does not exists for lease")
	}
	return nil
}

func deltaLeaseIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, lease *coordinationv1.Lease) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      utils.GetDeltaSnapshotLeaseName(instance),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, lease); err != nil {
		return err
	}

	if !checkEtcdOwnerReference(lease.GetOwnerReferences(), instance) {
		return fmt.Errorf("ownerReference does not exists for lease")
	}
	return nil
}
