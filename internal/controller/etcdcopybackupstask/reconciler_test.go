// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdcopybackupstask

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/common"
	druidstore "github.com/gardener/etcd-druid/internal/store"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	gomegatypes "github.com/onsi/gomega/types"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("EtcdCopyBackupsTaskController", func() {

	Describe("#getConditions", func() {
		var (
			jobConditions []batchv1.JobCondition
		)

		BeforeEach(func() {
			jobConditions = []batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   batchv1.JobFailed,
					Status: corev1.ConditionFalse,
				},
			}
		})

		It("should get the correct conditions from the job", func() {
			conditions := getConditions(jobConditions)
			Expect(len(conditions)).To(Equal(len(jobConditions)))
			for i, condition := range conditions {
				if condition.Type == druidv1alpha1.EtcdCopyBackupsTaskSucceeded {
					Expect(jobConditions[i].Type).To(Equal(batchv1.JobComplete))
				} else if condition.Type == druidv1alpha1.EtcdCopyBackupsTaskFailed {
					Expect(jobConditions[i].Type).To(Equal(batchv1.JobFailed))
				} else {
					Fail("got unexpected condition type")
				}
				Expect(condition.Status).To(Equal(druidv1alpha1.ConditionStatus(jobConditions[i].Status)))
			}
		})
	})

	Describe("#delete", func() {
		var (
			ctx        = context.Background()
			task       *druidv1alpha1.EtcdCopyBackupsTask
			fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
			r          = &Reconciler{
				Client: fakeClient,
				logger: logr.Discard(),
			}
		)
		const (
			testTaskName  = "test-etcd-backup-copy-task"
			testNamespace = "test-ns"
		)

		Context("delete EtcdCopyBackupsTask object tests when it exists", func() {
			BeforeEach(func() {
				task = testutils.CreateEtcdCopyBackupsTask(testTaskName, testNamespace, "aws", false)

				By("Create fake client with task object")
				fakeClient = fakeclient.NewClientBuilder().
					WithScheme(kubernetes.Scheme).
					WithObjects(task).
					WithStatusSubresource(task).
					Build()

				By("Ensure that copy backups task is created")
				Eventually(func() error {
					return fakeClient.Get(ctx, client.ObjectKeyFromObject(task), task)
				}).Should(Succeed())

				r = &Reconciler{
					Client: fakeClient,
					logger: logr.Discard(),
				}
			})
			AfterEach(func() {
				ensureEtcdCopyBackupsTaskRemoval(ctx, testTaskName, testNamespace, fakeClient)
			})

			It("should not delete if there is no deletion timestamp set and no finalizer set", func() {
				_, err := r.delete(ctx, task)
				Expect(err).To(BeNil())
				foundTask := &druidv1alpha1.EtcdCopyBackupsTask{}
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(task), foundTask)).To(Succeed())
				Expect(client.ObjectKeyFromObject(foundTask)).To(Equal(client.ObjectKeyFromObject(task)))
			})

			It("should remove finalizer for task which does not have a corresponding job", func() {
				Expect(controllerutils.AddFinalizers(ctx, fakeClient, task, common.FinalizerName)).To(Succeed())
				// use fakeClient.Delete() to simply add deletionTimestamp to `task` object,
				// due to https://github.com/kubernetes-sigs/controller-runtime/pull/2316
				Expect(fakeClient.Delete(ctx, task)).To(Succeed())
				// get the updated object after deletionTimestamp has been added by fakeClient.Delete() call
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(task), task)).To(Succeed())

				_, err := r.delete(ctx, task)
				Expect(err).To(BeNil())
				Eventually(func() error {
					return fakeClient.Get(ctx, client.ObjectKeyFromObject(task), task)
				}).Should(BeNotFoundError())
			})

			It("should delete job but not the task for which the deletion timestamp, finalizer is set and job is present", func() {
				job := testutils.CreateEtcdCopyBackupsJob(testTaskName, testNamespace)
				Expect(fakeClient.Create(ctx, job)).To(Succeed())
				Expect(controllerutils.AddFinalizers(ctx, fakeClient, task, common.FinalizerName)).To(Succeed())
				Expect(fakeClient.Delete(ctx, task)).To(Succeed())
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(task), task)).To(Succeed())

				_, err := r.delete(ctx, task)
				Expect(err).To(BeNil())
				Eventually(func() error { return fakeClient.Get(ctx, client.ObjectKeyFromObject(job), job) }).Should(BeNotFoundError())
				Eventually(func() error { return fakeClient.Get(ctx, client.ObjectKeyFromObject(task), task) }).Should(BeNil())
			})
		})
	})

	Describe("#createJobObject", func() {
		var (
			reconciler      *Reconciler
			ctx             = context.Background()
			fakeClient      = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
			namespace       = "test-ns"
			unknownProvider = druidv1alpha1.StorageProvider("unknown")
		)

		BeforeEach(func() {
			reconciler = &Reconciler{
				Client: fakeClient,
				logger: logr.Discard(),
				imageVector: imagevector.ImageVector{
					&imagevector.ImageSource{
						Name:       common.ImageKeyEtcdBackupRestore,
						Repository: ptr.To("test-repo"),
						Tag:        ptr.To("etcd-test-tag"),
					},
					&imagevector.ImageSource{
						Name:       common.ImageKeyAlpine,
						Repository: ptr.To("test-repo"),
						Tag:        ptr.To("init-container-test-tag"),
					},
				},
				Config: &Config{
					FeatureGates: make(map[featuregate.Feature]bool),
				},
			}
		})

		DescribeTable("should create the expected job object with correct metadata, pod template, and containers for a valid input task",
			func(taskName string, provider druidv1alpha1.StorageProvider, withOptionalFields bool) {
				task := testutils.CreateEtcdCopyBackupsTask(taskName, namespace, provider, withOptionalFields)
				errors := testutils.CreateSecrets(ctx, fakeClient, task.Namespace, task.Spec.SourceStore.SecretRef.Name, task.Spec.TargetStore.SecretRef.Name)
				Expect(errors).Should(BeNil())

				job, err := reconciler.createJobObject(ctx, task)
				Expect(err).NotTo(HaveOccurred())
				Expect(job).Should(PointTo(matchJob(task, reconciler.imageVector)))
			},
			Entry("with #Local provider, without optional fields",
				"foo01", druidv1alpha1.StorageProvider("Local"), false),
			Entry("with #Local provider, with optional fields",
				"foo02", druidv1alpha1.StorageProvider("Local"), true),
			Entry("with #S3 storage provider, without optional fields",
				"foo03", druidv1alpha1.StorageProvider("aws"), false),
			Entry("with #S3 storage provider, with optional fields",
				"foo04", druidv1alpha1.StorageProvider("aws"), true),
			Entry("with #AZURE storage provider, without optional fields",
				"foo05", druidv1alpha1.StorageProvider("azure"), false),
			Entry("with #AZURE storage provider, with optional fields",
				"foo06", druidv1alpha1.StorageProvider("azure"), true),
			Entry("with #GCP storage provider, without optional fields",
				"foo07", druidv1alpha1.StorageProvider("gcp"), false),
			Entry("with #GCP storage provider, with optional fields",
				"foo08", druidv1alpha1.StorageProvider("gcp"), true),
			Entry("with #OPENSTACK storage provider, without optional fields",
				"foo09", druidv1alpha1.StorageProvider("openstack"), false),
			Entry("with #OPENSTACK storage provider, with optional fields",
				"foo10", druidv1alpha1.StorageProvider("openstack"), true),
			Entry("with #ALICLOUD storage provider, without optional fields",
				"foo11", druidv1alpha1.StorageProvider("alicloud"), false),
			Entry("with #ALICLOUD storage provider, with optional fields",
				"foo12", druidv1alpha1.StorageProvider("alicloud"), true),
		)

		Context("when etcd-backup image is not found", func() {
			It("should return error", func() {
				reconciler.imageVector = nil
				task := testutils.CreateEtcdCopyBackupsTask("test", namespace, "Local", true)
				job, err := reconciler.createJobObject(ctx, task)

				Expect(job).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when target store provider is unknown", func() {
			It("should return error", func() {
				task := testutils.CreateEtcdCopyBackupsTask("test", namespace, "Local", true)
				task.Spec.TargetStore.Provider = &unknownProvider
				job, err := reconciler.createJobObject(ctx, task)

				Expect(job).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when source store provider is unknown", func() {
			It("should return error", func() {
				task := testutils.CreateEtcdCopyBackupsTask("test", namespace, "Local", true)
				task.Spec.SourceStore.Provider = &unknownProvider
				job, err := reconciler.createJobObject(ctx, task)

				Expect(job).To(BeNil())
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("#createJobArgumentFromStore", func() {
		Context("when given a nil store", func() {
			It("returns an empty argument slice", func() {
				Expect(createJobArgumentFromStore(nil, "provider", "prefix")).To(BeEmpty())
			})
		})

		Context("when given a empty provider", func() {
			It("returns an empty argument slice", func() {
				Expect(createJobArgumentFromStore(nil, "", "prefix")).To(BeEmpty())
			})
		})

		Context("when given a non-nil store", func() {
			var (
				store    druidv1alpha1.StoreSpec
				provider string
				prefix   string
			)

			BeforeEach(func() {
				store = druidv1alpha1.StoreSpec{
					Prefix:    "store_prefix",
					Container: ptr.To("store_container"),
				}
				provider = "storage_provider"
				prefix = "prefix"
			})

			It("returns a argument slice with provider, prefix, and container information", func() {
				expected := []string{
					"--prefixstorage-provider=storage_provider",
					"--prefixstore-prefix=store_prefix",
					"--prefixstore-container=store_container",
				}
				Expect(createJobArgumentFromStore(&store, provider, prefix)).To(Equal(expected))
			})

			It("should return a argument slice with provider and prefix information only when StoreSpec.Container is nil", func() {
				expected := []string{
					"--prefixstorage-provider=storage_provider",
					"--prefixstore-prefix=store_prefix",
				}
				store.Container = nil
				Expect(createJobArgumentFromStore(&store, provider, prefix)).To(Equal(expected))
			})

			It("should return a argument slice with provider and container information only when StoreSpec.Prefix is empty", func() {
				expected := []string{
					"--prefixstorage-provider=storage_provider",
					"--prefixstore-container=store_container",
				}
				store.Prefix = ""
				Expect(createJobArgumentFromStore(&store, provider, prefix)).To(Equal(expected))
			})

			It("returns an empty argument slice when StoreSpec.Provider is empty", func() {
				provider = ""
				Expect(createJobArgumentFromStore(&store, provider, prefix)).To(BeEmpty())
			})
		})
	})

	Describe("#createJobArguments", func() {
		var (
			providerLocal = druidv1alpha1.StorageProvider(druidstore.Local)
			providerS3    = druidv1alpha1.StorageProvider(druidstore.S3)
			task          *druidv1alpha1.EtcdCopyBackupsTask
			expected      = []string{
				"copy",
				"--snapstore-temp-directory=/home/nonroot/data/tmp",
				"--storage-provider=S3",
				"--store-prefix=/target",
				"--store-container=target-container",
				"--source-storage-provider=Local",
				"--source-store-prefix=/source",
				"--source-store-container=source-container",
			}
		)

		BeforeEach(func() {
			task = &druidv1alpha1.EtcdCopyBackupsTask{
				Spec: druidv1alpha1.EtcdCopyBackupsTaskSpec{
					SourceStore: druidv1alpha1.StoreSpec{
						Prefix:    "/source",
						Container: ptr.To("source-container"),
						Provider:  &providerLocal,
					},
					TargetStore: druidv1alpha1.StoreSpec{
						Prefix:    "/target",
						Container: ptr.To("target-container"),
						Provider:  &providerS3,
						SecretRef: &corev1.SecretReference{
							Name: "test-secret",
						},
					},
				},
			}
		})

		It("should create the correct arguments", func() {
			arguments := createJobArgs(task, druidstore.Local, druidstore.S3)
			Expect(arguments).To(Equal(expected))
		})

		It("should include the max backup age in the arguments", func() {
			task.Spec.MaxBackupAge = ptr.To[uint32](10)
			arguments := createJobArgs(task, druidstore.Local, druidstore.S3)
			Expect(arguments).To(Equal(append(expected, "--max-backup-age=10")))
		})

		It("should include the max number of backups in the arguments", func() {
			task.Spec.MaxBackups = ptr.To[uint32](5)
			arguments := createJobArgs(task, druidstore.Local, druidstore.S3)
			Expect(arguments).To(Equal(append(expected, "--max-backups-to-copy=5")))
		})

		It("should include the wait for final snapshot in the arguments", func() {
			task.Spec.WaitForFinalSnapshot = &druidv1alpha1.WaitForFinalSnapshotSpec{
				Enabled: true,
			}
			arguments := createJobArgs(task, druidstore.Local, druidstore.S3)
			Expect(arguments).To(Equal(append(expected, "--wait-for-final-snapshot=true")))
		})

		It("should include the wait for final snapshot and timeout in the arguments", func() {
			task.Spec.WaitForFinalSnapshot = &druidv1alpha1.WaitForFinalSnapshotSpec{
				Enabled: true,
				Timeout: &metav1.Duration{Duration: time.Minute},
			}
			arguments := createJobArgs(task, druidstore.Local, druidstore.S3)
			Expect(arguments).To(Equal(append(expected, "--wait-for-final-snapshot=true", "--wait-for-final-snapshot-timeout=1m0s")))
		})
	})

	Describe("#createEnvVarsFromStore", func() {
		var (
			envKeyPrefix = "SOURCE_"
			volumePrefix = "source-"
			container    = "source-container"
			storeSpec    *druidv1alpha1.StoreSpec
		)
		// Loop through different storage providers to test with
		for _, p := range []string{
			druidstore.ABS,
			druidstore.GCS,
			druidstore.S3,
			druidstore.Swift,
			druidstore.OSS,
			druidstore.OCS,
		} {
			Context(fmt.Sprintf("with provider #%s", p), func() {
				provider := p
				BeforeEach(func() {
					storageProvider := druidv1alpha1.StorageProvider(provider)
					storeSpec = &druidv1alpha1.StoreSpec{
						Container: &container,
						Provider:  &storageProvider,
					}
				})

				It("should create the correct env vars", func() {
					envVars := createEnvVarsFromStore(storeSpec, provider, envKeyPrefix, volumePrefix)
					checkEnvVars(envVars, provider, container, envKeyPrefix, volumePrefix)

				})
			})
		}
		Context("with provider #Local", func() {
			BeforeEach(func() {
				storageProvider := druidv1alpha1.StorageProvider(druidstore.Local)
				storeSpec = &druidv1alpha1.StoreSpec{
					Container: &container,
					Provider:  &storageProvider,
				}
			})

			It("should create the correct env vars", func() {
				envVars := createEnvVarsFromStore(storeSpec, druidstore.Local, envKeyPrefix, volumePrefix)
				checkEnvVars(envVars, druidstore.Local, container, envKeyPrefix, volumePrefix)

			})
		})
	})

	Describe("#createVolumeMountsFromStore", func() {
		var (
			volumeMountPrefix = "source-"
			storeSpec         *druidv1alpha1.StoreSpec
		)
		// Loop through different storage providers to test with
		for _, p := range []string{
			druidstore.Local,
			druidstore.ABS,
			druidstore.GCS,
			druidstore.S3,
			druidstore.Swift,
			druidstore.OSS,
			druidstore.OCS,
		} {
			Context(fmt.Sprintf("with provider #%s", p), func() {
				provider := p
				BeforeEach(func() {
					storageProvider := druidv1alpha1.StorageProvider(provider)
					storeSpec = &druidv1alpha1.StoreSpec{
						Container: ptr.To("source-container"),
						Provider:  &storageProvider,
					}
				})

				It("should create the correct volume mounts", func() {
					volumeMounts := createVolumeMountsFromStore(storeSpec, provider, volumeMountPrefix)
					Expect(volumeMounts).To(HaveLen(1))

					expectedMountPath := ""
					expectedMountName := ""

					switch provider {
					case druidstore.Local:
						expectedMountName = volumeMountPrefix + "host-storage"
						expectedMountPath = "/home/nonroot/" + *storeSpec.Container
					case druidstore.GCS:
						expectedMountName = volumeMountPrefix + common.VolumeNameProviderBackupSecret
						expectedMountPath = getGCSSecretVolumeMountPathWithPrefixAndSuffix(volumeMountPrefix, "/")
					case druidstore.S3, druidstore.ABS, druidstore.Swift, druidstore.OCS, druidstore.OSS:
						expectedMountName = volumeMountPrefix + common.VolumeNameProviderBackupSecret
						expectedMountPath = getNonGCSSecretVolumeMountPathWithPrefixAndSuffix(volumeMountPrefix, "/")
					default:
						Fail(fmt.Sprintf("Unknown provider: %s", provider))
					}

					Expect(volumeMounts[0].Name).To(Equal(expectedMountName))
					Expect(volumeMounts[0].MountPath).To(Equal(expectedMountPath))
				})
			})
		}
	})

	Describe("#createVolumesFromStore", func() {
		Context("with provider #Local", func() {
			var (
				fakeClient    = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
				ctx           = context.Background()
				secret        *corev1.Secret
				providerLocal = druidv1alpha1.StorageProvider("Local")
				namespace     = "test-ns"
				reconciler    = &Reconciler{
					Client: fakeClient,
					logger: logr.Discard(),
				}

				store = &druidv1alpha1.StoreSpec{
					Container: ptr.To("source-container"),
					Prefix:    "/tmp",
					Provider:  &providerLocal,
				}
			)

			BeforeEach(func() {
				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: namespace,
					},
				}
			})

			AfterEach(func() {
				Expect(fakeClient.Delete(ctx, secret)).To(Succeed())
			})

			It("should create the correct volumes when secret data hostPath is set", func() {
				secret.Data = map[string][]byte{
					druidstore.EtcdBackupSecretHostPath: []byte("/test/hostPath"),
				}
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				store.SecretRef = &corev1.SecretReference{Name: secret.Name}
				volumes, err := reconciler.createVolumesFromStore(ctx, store, namespace, string(providerLocal), "source-")
				Expect(err).NotTo(HaveOccurred())
				Expect(volumes).To(HaveLen(1))
				Expect(volumes[0].Name).To(Equal("source-host-storage"))

				hostPathVolumeSource := volumes[0].VolumeSource.HostPath
				Expect(hostPathVolumeSource).NotTo(BeNil())
				Expect(hostPathVolumeSource.Path).To(Equal("/test/hostPath/" + *store.Container))
				Expect(*hostPathVolumeSource.Type).To(Equal(corev1.HostPathDirectory))
			})

			It("should create the correct volumes when secret data hostPath is not set", func() {
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				store.SecretRef = &corev1.SecretReference{Name: secret.Name}
				volumes, err := reconciler.createVolumesFromStore(ctx, store, namespace, string(providerLocal), "source-")
				Expect(err).NotTo(HaveOccurred())
				Expect(volumes).To(HaveLen(1))
				Expect(volumes[0].Name).To(Equal("source-host-storage"))

				hostPathVolumeSource := volumes[0].VolumeSource.HostPath
				Expect(hostPathVolumeSource).NotTo(BeNil())
				Expect(hostPathVolumeSource.Path).To(Equal(druidstore.LocalProviderDefaultMountPath + "/" + *store.Container))
				Expect(*hostPathVolumeSource.Type).To(Equal(corev1.HostPathDirectory))
			})

			It("should create the correct volumes when store.SecretRef is not referred", func() {
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				store.SecretRef = &corev1.SecretReference{Name: secret.Name}
				volumes, err := reconciler.createVolumesFromStore(ctx, store, namespace, string(providerLocal), "source-")
				Expect(err).NotTo(HaveOccurred())
				Expect(volumes).To(HaveLen(1))
				Expect(volumes[0].Name).To(Equal("source-host-storage"))

				hostPathVolumeSource := volumes[0].VolumeSource.HostPath
				Expect(hostPathVolumeSource).NotTo(BeNil())
				Expect(hostPathVolumeSource.Path).To(Equal(druidstore.LocalProviderDefaultMountPath + "/" + *store.Container))
				Expect(*hostPathVolumeSource.Type).To(Equal(corev1.HostPathDirectory))
			})
		})

		Context("with provider", func() {
			var (
				storageProvider druidv1alpha1.StorageProvider
				fakeClient      = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
				ctx             = context.Background()
				secret          *corev1.Secret
				store           *druidv1alpha1.StoreSpec
				namespace       = "test-ns"
				reconciler      = &Reconciler{
					Client: fakeClient,
					logger: logr.Discard(),
				}
			)

			// Loop through different storage providers to test with
			for _, p := range []string{
				druidstore.ABS,
				druidstore.GCS,
				druidstore.S3,
				druidstore.Swift,
				druidstore.OSS,
				druidstore.OCS,
			} {
				Context(fmt.Sprintf("#%s", p), func() {
					BeforeEach(func() {
						provider := p
						// Set up test variables and create necessary secrets
						storageProvider = druidv1alpha1.StorageProvider(provider)
						store = &druidv1alpha1.StoreSpec{
							Container: ptr.To("source-container"),
							Provider:  &storageProvider,
						}
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-secret-" + provider,
								Namespace: namespace,
							},
						}
						Expect(fakeClient.Create(ctx, secret)).To(Succeed())
					})

					AfterEach(func() {
						// Clean up secret after each test case
						Expect(fakeClient.Delete(ctx, secret)).To(Succeed())
					})

					It("should create the correct volumes", func() {
						// Call the function being tested with a valid secret reference
						store.SecretRef = &corev1.SecretReference{Name: secret.Name}
						volumes, err := reconciler.createVolumesFromStore(ctx, store, namespace, string(storageProvider), "source-")
						Expect(err).NotTo(HaveOccurred())
						Expect(volumes).To(HaveLen(1))
						Expect(volumes[0].Name).To(Equal("source-etcd-backup-secret"))

						// Assert that the volume is created correctly with the expected secret
						volumeSource := volumes[0].VolumeSource
						Expect(volumeSource).NotTo(BeNil())
						Expect(volumeSource.Secret).NotTo(BeNil())
						Expect(*volumeSource.Secret).To(Equal(corev1.SecretVolumeSource{
							SecretName:  store.SecretRef.Name,
							DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
						}))
					})

					It("should return an error when secret reference is invalid", func() {
						// Call the function being tested with an invalid secret reference
						volumes, err := reconciler.createVolumesFromStore(ctx, store, namespace, string(storageProvider), "source-")

						// Assert that an error is returned and no volumes are created
						Expect(err.Error()).To(Equal("no secretRef is configured for backup source-store"))
						Expect(volumes).To(HaveLen(0))
					})
				})
			}
		})
	})

})

func ensureEtcdCopyBackupsTaskCreation(ctx context.Context, name, namespace string, fakeClient client.WithWatch) *druidv1alpha1.EtcdCopyBackupsTask {
	task := testutils.CreateEtcdCopyBackupsTask(name, namespace, "aws", false)
	By("create task")
	Expect(fakeClient.Create(ctx, task)).To(Succeed())

	By("Ensure that copy backups task is created")
	Eventually(func() error {
		return fakeClient.Get(ctx, client.ObjectKeyFromObject(task), task)
	}).Should(Succeed())

	return task
}

func ensureEtcdCopyBackupsTaskRemoval(ctx context.Context, name, namespace string, fakeClient client.WithWatch) {
	task := &druidv1alpha1.EtcdCopyBackupsTask{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, task); err != nil {
		Expect(err).To(BeNotFoundError())
		return
	}

	By("Remove any existing finalizers on EtcdCopyBackupsTask")
	Expect(controllerutils.RemoveAllFinalizers(ctx, fakeClient, task)).To(Succeed())

	By("Delete EtcdCopyBackupsTask")
	err := fakeClient.Delete(ctx, task)
	if err != nil {
		Expect(err).Should(BeNotFoundError())
	}

	By("Ensure EtcdCopyBackupsTask is deleted")
	Eventually(func() error {
		return fakeClient.Get(ctx, client.ObjectKeyFromObject(task), task)
	}).Should(BeNotFoundError())
}

func addDeletionTimestampToTask(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask, deletionTime time.Time, fakeClient client.WithWatch) error {
	patch := client.MergeFrom(task.DeepCopy())
	task.DeletionTimestamp = &metav1.Time{Time: deletionTime}
	return fakeClient.Patch(ctx, task, patch)
}

func checkEnvVars(envVars []corev1.EnvVar, storeProvider, container, envKeyPrefix, volumePrefix string) {
	expected := []corev1.EnvVar{
		{
			Name:  envKeyPrefix + common.EnvStorageContainer,
			Value: container,
		}}
	mapToEnvVarKey := map[string]string{
		druidstore.S3:    envKeyPrefix + common.EnvAWSApplicationCredentials,
		druidstore.ABS:   envKeyPrefix + common.EnvAzureApplicationCredentials,
		druidstore.GCS:   envKeyPrefix + common.EnvGoogleApplicationCredentials,
		druidstore.Swift: envKeyPrefix + common.EnvOpenstackApplicationCredentials,
		druidstore.OCS:   envKeyPrefix + common.EnvOpenshiftApplicationCredentials,
		druidstore.OSS:   envKeyPrefix + common.EnvAlicloudApplicationCredentials,
	}
	switch storeProvider {
	case druidstore.S3, druidstore.ABS, druidstore.Swift, druidstore.OCS, druidstore.OSS:
		expected = append(expected, corev1.EnvVar{
			Name:  mapToEnvVarKey[storeProvider],
			Value: getNonGCSSecretVolumeMountPathWithPrefixAndSuffix(volumePrefix, ""),
		})
	case druidstore.GCS:
		expected = append(expected, corev1.EnvVar{
			Name:  mapToEnvVarKey[storeProvider],
			Value: getGCSSecretVolumeMountPathWithPrefixAndSuffix(volumePrefix, "/serviceaccount.json"),
		})
	}
	Expect(envVars).To(Equal(expected))
}

func matchJob(task *druidv1alpha1.EtcdCopyBackupsTask, imageVector imagevector.ImageVector) gomegatypes.GomegaMatcher {
	sourceProvider, err := druidstore.StorageProviderFromInfraProvider(task.Spec.SourceStore.Provider)
	Expect(err).NotTo(HaveOccurred())
	targetProvider, err := druidstore.StorageProviderFromInfraProvider(task.Spec.TargetStore.Provider)
	Expect(err).NotTo(HaveOccurred())

	images, err := imagevector.FindImages(imageVector, []string{common.ImageKeyEtcdBackupRestore})
	Expect(err).NotTo(HaveOccurred())
	backupRestoreImage := images[common.ImageKeyEtcdBackupRestore]

	matcher := MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(task.Name + "-worker"),
			"Namespace": Equal(task.Namespace),
			"Labels": MatchKeys(IgnoreExtras, Keys{
				druidv1alpha1.LabelComponentKey: Equal(common.ComponentNameEtcdCopyBackupsJob),
				druidv1alpha1.LabelPartOfKey:    Equal(task.Name),
				druidv1alpha1.LabelManagedByKey: Equal(druidv1alpha1.LabelManagedByValue),
				druidv1alpha1.LabelAppNameKey:   Equal(task.GetJobName()),
			}),
			"OwnerReferences": MatchAllElements(testutils.OwnerRefIterator, Elements{
				task.Name: MatchAllFields(Fields{
					"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
					"Kind":               Equal("EtcdCopyBackupsTask"),
					"Name":               Equal(task.Name),
					"UID":                Equal(task.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"ObjectMeta": MatchFields(IgnoreExtras, Fields{
					"Labels": MatchKeys(IgnoreExtras, Keys{
						druidv1alpha1.LabelComponentKey:                Equal(common.ComponentNameEtcdCopyBackupsJob),
						druidv1alpha1.LabelPartOfKey:                   Equal(task.Name),
						druidv1alpha1.LabelManagedByKey:                Equal(druidv1alpha1.LabelManagedByValue),
						druidv1alpha1.LabelAppNameKey:                  Equal(task.GetJobName()),
						"networking.gardener.cloud/to-dns":             Equal("allowed"),
						"networking.gardener.cloud/to-public-networks": Equal("allowed"),
					}),
				}),
				"Spec": MatchFields(IgnoreExtras, Fields{
					"RestartPolicy": Equal(corev1.RestartPolicyOnFailure),
					"Containers": MatchAllElements(testutils.ContainerIterator, Elements{
						"copy-backups": MatchFields(IgnoreExtras, Fields{
							"Name":            Equal("copy-backups"),
							"Image":           Equal(fmt.Sprintf("%s:%s", *backupRestoreImage.Repository, *backupRestoreImage.Tag)),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Args":            MatchAllElements(testutils.CmdIterator, getArgElements(task, sourceProvider, targetProvider)),
							"Env":             MatchElements(testutils.EnvIterator, IgnoreExtras, getEnvElements(task)),
						}),
					}),
				}),
			}),
		}),
	})

	return And(matcher, matchJobWithProviders(task, sourceProvider, targetProvider))
}

func getArgElements(task *druidv1alpha1.EtcdCopyBackupsTask, sourceProvider, targetProvider string) Elements {
	elements := Elements{
		"copy": Equal("copy"),
		"--snapstore-temp-directory=/home/nonroot/data/tmp": Equal("--snapstore-temp-directory=/home/nonroot/data/tmp"),
	}
	if targetProvider != "" {
		addEqual(elements, fmt.Sprintf("%s=%s", "--storage-provider", targetProvider))
	}
	if task.Spec.TargetStore.Prefix != "" {
		addEqual(elements, fmt.Sprintf("%s=%s", "--store-prefix", task.Spec.TargetStore.Prefix))
	}
	if task.Spec.TargetStore.Container != nil && *task.Spec.TargetStore.Container != "" {
		addEqual(elements, fmt.Sprintf("%s=%s", "--store-container", *task.Spec.TargetStore.Container))
	}
	if sourceProvider != "" {
		addEqual(elements, fmt.Sprintf("%s=%s", "--source-storage-provider", sourceProvider))
	}
	if task.Spec.SourceStore.Prefix != "" {
		addEqual(elements, fmt.Sprintf("%s=%s", "--source-store-prefix", task.Spec.SourceStore.Prefix))
	}
	if task.Spec.SourceStore.Container != nil && *task.Spec.SourceStore.Container != "" {
		addEqual(elements, fmt.Sprintf("%s=%s", "--source-store-container", *task.Spec.SourceStore.Container))
	}
	if task.Spec.MaxBackupAge != nil && *task.Spec.MaxBackupAge != 0 {
		addEqual(elements, fmt.Sprintf("%s=%d", "--max-backup-age", *task.Spec.MaxBackupAge))
	}
	if task.Spec.MaxBackups != nil && *task.Spec.MaxBackups != 0 {
		addEqual(elements, fmt.Sprintf("%s=%d", "--max-backups-to-copy", *task.Spec.MaxBackups))
	}
	if task.Spec.WaitForFinalSnapshot != nil && task.Spec.WaitForFinalSnapshot.Enabled {
		addEqual(elements, fmt.Sprintf("%s=%t", "--wait-for-final-snapshot", task.Spec.WaitForFinalSnapshot.Enabled))
		if task.Spec.WaitForFinalSnapshot.Timeout != nil && task.Spec.WaitForFinalSnapshot.Timeout.Duration != 0 {
			addEqual(elements, fmt.Sprintf("%s=%s", "--wait-for-final-snapshot-timeout", task.Spec.WaitForFinalSnapshot.Timeout.Duration.String()))
		}
	}
	return elements
}

func getEnvElements(task *druidv1alpha1.EtcdCopyBackupsTask) Elements {
	elements := Elements{}
	if task.Spec.TargetStore.Container != nil && *task.Spec.TargetStore.Container != "" {
		elements[common.EnvStorageContainer] = MatchFields(IgnoreExtras, Fields{
			"Name":  Equal(common.EnvStorageContainer),
			"Value": Equal(*task.Spec.TargetStore.Container),
		})
	}
	if task.Spec.SourceStore.Container != nil && *task.Spec.SourceStore.Container != "" {
		elements[common.EnvSourceStorageContainer] = MatchFields(IgnoreExtras, Fields{
			"Name":  Equal(common.EnvSourceStorageContainer),
			"Value": Equal(*task.Spec.SourceStore.Container),
		})
	}
	return elements
}

func matchJobWithProviders(task *druidv1alpha1.EtcdCopyBackupsTask, sourceProvider, targetProvider string) gomegatypes.GomegaMatcher {
	matcher := MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchAllElements(testutils.ContainerIterator, Elements{
						"copy-backups": MatchFields(IgnoreExtras, Fields{
							"Env": And(
								MatchElements(testutils.EnvIterator, IgnoreExtras, getProviderEnvElements(targetProvider, "", "")),
								MatchElements(testutils.EnvIterator, IgnoreExtras, getProviderEnvElements(sourceProvider, "SOURCE_", "source-")),
							),
						}),
					}),
				}),
			}),
		}),
	})
	if sourceProvider == "GCS" || targetProvider == "GCS" {
		volumeMatcher := MatchFields(IgnoreExtras, Fields{
			"Spec": MatchFields(IgnoreExtras, Fields{
				"Template": MatchFields(IgnoreExtras, Fields{
					"Spec": MatchFields(IgnoreExtras, Fields{
						"Containers": MatchAllElements(testutils.ContainerIterator, Elements{
							"copy-backups": MatchFields(IgnoreExtras, Fields{
								"VolumeMounts": And(
									MatchElements(testutils.VolumeMountIterator, IgnoreExtras, getVolumeMountsElements(targetProvider, "")),
									MatchElements(testutils.VolumeMountIterator, IgnoreExtras, getVolumeMountsElements(sourceProvider, "source-")),
								),
							}),
						}),
						"Volumes": And(
							MatchElements(testutils.VolumeIterator, IgnoreExtras, getVolumesElements("", &task.Spec.TargetStore)),
							MatchElements(testutils.VolumeIterator, IgnoreExtras, getVolumesElements("source-", &task.Spec.SourceStore)),
						),
					}),
				}),
			}),
		})
		return And(matcher, volumeMatcher)
	}
	return matcher
}

func getProviderEnvElements(storeProvider, prefix, volumePrefix string) Elements {
	switch storeProvider {
	case "S3":
		return Elements{
			prefix + common.EnvAWSApplicationCredentials: MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + common.EnvAWSApplicationCredentials),
				"Value": Equal(fmt.Sprintf("/var/%setcd-backup", volumePrefix)),
			}),
		}
	case "ABS":
		return Elements{
			prefix + common.EnvAzureApplicationCredentials: MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + common.EnvAzureApplicationCredentials),
				"Value": Equal(fmt.Sprintf("/var/%setcd-backup", volumePrefix)),
			}),
		}
	case "GCS":
		return Elements{
			prefix + common.EnvGoogleApplicationCredentials: MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + common.EnvGoogleApplicationCredentials),
				"Value": Equal(getGCSSecretVolumeMountPathWithPrefixAndSuffix(volumePrefix, "/serviceaccount.json")),
			}),
		}
	case "Swift":
		return Elements{
			prefix + common.EnvOpenstackApplicationCredentials: MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + common.EnvOpenstackApplicationCredentials),
				"Value": Equal(fmt.Sprintf("/var/%setcd-backup", volumePrefix)),
			}),
		}
	case "OSS":
		return Elements{
			prefix + common.EnvAlicloudApplicationCredentials: MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + common.EnvAlicloudApplicationCredentials),
				"Value": Equal(fmt.Sprintf("/var/%setcd-backup", volumePrefix)),
			}),
		}
	case "OCS":
		return Elements{
			prefix + common.EnvOpenshiftApplicationCredentials: MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + common.EnvOpenshiftApplicationCredentials),
				"Value": Equal(fmt.Sprintf("/var/%setcd-backup", volumePrefix)),
			}),
		}
	default:
		return nil
	}
}

func getVolumeMountsElements(storeProvider, volumePrefix string) Elements {
	switch storeProvider {
	case "GCS":
		return Elements{
			volumePrefix + common.VolumeNameProviderBackupSecret: MatchFields(IgnoreExtras, Fields{
				"Name":      Equal(getVolumeNamePrefix(volumePrefix) + common.VolumeNameProviderBackupSecret),
				"MountPath": Equal(fmt.Sprintf("/var/.%sgcp/", volumePrefix)),
			}),
		}
	default:
		return Elements{
			volumePrefix + common.VolumeNameProviderBackupSecret: MatchFields(IgnoreExtras, Fields{
				"Name":      Equal(volumePrefix + common.VolumeNameProviderBackupSecret),
				"MountPath": Equal(fmt.Sprintf("/var/%setcd-backup", volumePrefix)),
			}),
		}
	}
}

func getVolumesElements(volumePrefix string, store *druidv1alpha1.StoreSpec) Elements {
	return Elements{
		volumePrefix + common.VolumeNameProviderBackupSecret: MatchAllFields(Fields{
			"Name": Equal(volumePrefix + common.VolumeNameProviderBackupSecret),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(store.SecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
				})),
			}),
		}),
	}
}

func addEqual(elements Elements, s string) {
	elements[s] = Equal(s)
}
