// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
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
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/pkg/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	. "github.com/onsi/gomega/types"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("EtcdCopyBackupsTask Controller", func() {
	var (
		c   client.Client
		ctx context.Context
	)

	BeforeEach(func() {
		c = mgr.GetClient()
		ctx = context.TODO()
	})

	DescribeTable("when creating and deleting etcdcopybackupstask",
		func(task *druidv1alpha1.EtcdCopyBackupsTask, jobStatus *batchv1.JobStatus) {
			// Create namespace
			_, err := controllerutil.CreateOrUpdate(ctx, c, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: task.Namespace}}, func() error { return nil })
			Expect(err).To(Not(HaveOccurred()))

			// Create secrets
			errors := createSecrets(c, task.Namespace, task.Spec.SourceStore.SecretRef.Name, task.Spec.TargetStore.SecretRef.Name)
			Expect(len(errors)).Should(BeZero())

			// Create task
			Expect(c.Create(ctx, task)).To(Succeed())

			// Wait until the job has been created
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      task.Name + workerSuffix,
					Namespace: task.Namespace,
				},
			}
			Eventually(func() (*batchv1.Job, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
					return nil, err
				}
				return job, nil
			}, timeout, pollingInterval).Should(PointTo(matchJob(task)))

			// Update job status
			job.Status = *jobStatus
			err = c.Status().Update(ctx, job)
			Expect(err).NotTo(HaveOccurred())

			// Wait until the task status has been updated
			Eventually(func() (*druidv1alpha1.EtcdCopyBackupsTask, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(task), task); err != nil {
					return nil, err
				}
				return task, nil
			}, timeout, pollingInterval).Should(PointTo(matchTaskStatus(&job.Status)))

			// Delete task
			Expect(c.Delete(ctx, task)).To(Succeed())

			// Wait until the job gets the "foregroundDeletion" finalizer and remove it
			Eventually(func() (*batchv1.Job, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
					return nil, err
				}
				return job, nil
			}, timeout, pollingInterval).Should(PointTo(matchFinalizer(metav1.FinalizerDeleteDependents)))
			Expect(controllerutils.PatchRemoveFinalizers(ctx, c, job, metav1.FinalizerDeleteDependents)).To(Succeed())

			// Wait until the job has been deleted
			Eventually(func() error {
				return c.Get(ctx, client.ObjectKeyFromObject(job), &batchv1.Job{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

			// Wait until the task has been deleted
			Eventually(func() error {
				return c.Get(ctx, client.ObjectKeyFromObject(task), &druidv1alpha1.EtcdCopyBackupsTask{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
		},
		Entry("should create the job, update the task status, and delete the job if the job completed",
			getEtcdCopyBackupsTask("Local", true), getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job failed",
			getEtcdCopyBackupsTask("Local", false), getJobStatus(batchv1.JobFailed, "test reason", "test message")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for aws",
			getEtcdCopyBackupsTask("aws", false), getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for azure",
			getEtcdCopyBackupsTask("azure", false), getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for gcp",
			getEtcdCopyBackupsTask("gcp", false), getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for openstack",
			getEtcdCopyBackupsTask("openstack", false), getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for alicloud",
			getEtcdCopyBackupsTask("alicloud", false), getJobStatus(batchv1.JobComplete, "", "")),
	)
})

func getEtcdCopyBackupsTask(provider druidv1alpha1.StorageProvider, withOptionalFields bool) *druidv1alpha1.EtcdCopyBackupsTask {
	var (
		maxBackupAge, maxBackups *uint32
		waitForFinalSnapshot     *druidv1alpha1.WaitForFinalSnapshotSpec
	)
	if withOptionalFields {
		maxBackupAge = uint32Ptr(7)
		maxBackups = uint32Ptr(42)
		waitForFinalSnapshot = &druidv1alpha1.WaitForFinalSnapshotSpec{
			Enabled: true,
			Timeout: &metav1.Duration{Duration: 10 * time.Minute},
		}
	}
	return &druidv1alpha1.EtcdCopyBackupsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: druidv1alpha1.EtcdCopyBackupsTaskSpec{
			SourceStore: druidv1alpha1.StoreSpec{
				Container: pointer.StringPtr("source-container"),
				Prefix:    "/tmp",
				Provider:  &provider,
				SecretRef: &corev1.SecretReference{
					Name: "source-etcd-backup",
				},
			},
			TargetStore: druidv1alpha1.StoreSpec{
				Container: pointer.StringPtr("target-container"),
				Prefix:    "/tmp",
				Provider:  &provider,
				SecretRef: &corev1.SecretReference{
					Name: "target-etcd-backup",
				},
			},
			MaxBackupAge:         maxBackupAge,
			MaxBackups:           maxBackups,
			WaitForFinalSnapshot: waitForFinalSnapshot,
		},
	}
}

func matchJob(task *druidv1alpha1.EtcdCopyBackupsTask) GomegaMatcher {
	sourceProvider, err := utils.StorageProviderFromInfraProvider(task.Spec.SourceStore.Provider)
	Expect(err).NotTo(HaveOccurred())
	targetProvider, err := utils.StorageProviderFromInfraProvider(task.Spec.TargetStore.Provider)
	Expect(err).NotTo(HaveOccurred())

	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getImageYAMLPath())
	Expect(err).NotTo(HaveOccurred())
	images, err := imagevector.FindImages(imageVector, imageNames)
	Expect(err).NotTo(HaveOccurred())

	matcher := MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(task.Name + workerSuffix),
			"Namespace": Equal(task.Namespace),
			"Annotations": MatchKeys(IgnoreExtras, Keys{
				"gardener.cloud/owned-by":   Equal(fmt.Sprintf("%s/%s", task.Namespace, task.Name)),
				"gardener.cloud/owner-type": Equal("etcdcopybackupstask"),
			}),
			"OwnerReferences": MatchAllElements(ownerRefIterator, Elements{
				task.Name: MatchAllFields(Fields{
					"APIVersion":         Equal("druid.gardener.cloud/v1alpha1"),
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
						"networking.gardener.cloud/to-dns":             Equal("allowed"),
						"networking.gardener.cloud/to-public-networks": Equal("allowed"),
					}),
				}),
				"Spec": MatchFields(IgnoreExtras, Fields{
					"RestartPolicy": Equal(corev1.RestartPolicyOnFailure),
					"Containers": MatchAllElements(containerIterator, Elements{
						"copy-backups": MatchFields(IgnoreExtras, Fields{
							"Name":            Equal("copy-backups"),
							"Image":           Equal(fmt.Sprintf("%s:%s", images[common.BackupRestore].Repository, *images[common.BackupRestore].Tag)),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Command":         MatchAllElements(cmdIterator, getCmdElements(task, sourceProvider, targetProvider)),
							"Env":             MatchElements(envIterator, IgnoreExtras, getEnvElements(task)),
						}),
					}),
				}),
			}),
		}),
	})

	return And(matcher, matchJobWithProviders(task, sourceProvider, targetProvider))
}

func getCmdElements(task *druidv1alpha1.EtcdCopyBackupsTask, sourceProvider, targetProvider string) Elements {
	elements := Elements{
		"etcdbrctl": Equal("etcdbrctl"),
		"copy":      Equal("copy"),
		"--snapstore-temp-directory=/var/etcd/data/tmp": Equal("--snapstore-temp-directory=/var/etcd/data/tmp"),
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
		elements["STORAGE_CONTAINER"] = MatchFields(IgnoreExtras, Fields{
			"Name":  Equal("STORAGE_CONTAINER"),
			"Value": Equal(*task.Spec.TargetStore.Container),
		})
	}
	if task.Spec.SourceStore.Container != nil && *task.Spec.SourceStore.Container != "" {
		elements["SOURCE_STORAGE_CONTAINER"] = MatchFields(IgnoreExtras, Fields{
			"Name":  Equal("SOURCE_STORAGE_CONTAINER"),
			"Value": Equal(*task.Spec.SourceStore.Container),
		})
	}
	return elements
}

func matchJobWithProviders(task *druidv1alpha1.EtcdCopyBackupsTask, sourceProvider, targetProvider string) GomegaMatcher {
	matcher := MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchAllElements(containerIterator, Elements{
						"copy-backups": MatchFields(IgnoreExtras, Fields{
							"Env": And(
								MatchElements(envIterator, IgnoreExtras, getProviderEnvElements(targetProvider, "", "", &task.Spec.TargetStore)),
								MatchElements(envIterator, IgnoreExtras, getProviderEnvElements(sourceProvider, "SOURCE_", "source-", &task.Spec.SourceStore)),
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
						"Containers": MatchAllElements(containerIterator, Elements{
							"copy-backups": MatchFields(IgnoreExtras, Fields{
								"VolumeMounts": And(
									MatchElements(volumeMountIterator, IgnoreExtras, getVolumeMountsElements(targetProvider, "")),
									MatchElements(volumeMountIterator, IgnoreExtras, getVolumeMountsElements(sourceProvider, "source-")),
								),
							}),
						}),
						"Volumes": And(
							MatchElements(volumeIterator, IgnoreExtras, getVolumesElements(targetProvider, "", &task.Spec.TargetStore)),
							MatchElements(volumeIterator, IgnoreExtras, getVolumesElements(sourceProvider, "source-", &task.Spec.SourceStore)),
						),
					}),
				}),
			}),
		})
		return And(matcher, volumeMatcher)
	}
	return matcher
}

func getProviderEnvElements(storeProvider, prefix, volumePrefix string, store *druidv1alpha1.StoreSpec) Elements {
	switch storeProvider {
	case "S3":
		return Elements{
			prefix + "AWS_REGION":            matchEnvValueFrom(prefix+"AWS_REGION", store, "region"),
			prefix + "AWS_SECRET_ACCESS_KEY": matchEnvValueFrom(prefix+"AWS_SECRET_ACCESS_KEY", store, "secretAccessKey"),
			prefix + "AWS_ACCESS_KEY_ID":     matchEnvValueFrom(prefix+"AWS_ACCESS_KEY_ID", store, "accessKeyID"),
		}
	case "ABS":
		return Elements{
			prefix + "STORAGE_ACCOUNT": matchEnvValueFrom(prefix+"STORAGE_ACCOUNT", store, "storageAccount"),
			prefix + "STORAGE_KEY":     matchEnvValueFrom(prefix+"STORAGE_KEY", store, "storageKey"),
		}
	case "GCS":
		return Elements{
			prefix + "GOOGLE_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + "GOOGLE_APPLICATION_CREDENTIALS"),
				"Value": Equal(fmt.Sprintf("/root/.%sgcp/serviceaccount.json", volumePrefix)),
			}),
		}
	case "Swift":
		return Elements{
			prefix + "OS_AUTH_URL":    matchEnvValueFrom(prefix+"OS_AUTH_URL", store, "authURL"),
			prefix + "OS_DOMAIN_NAME": matchEnvValueFrom(prefix+"OS_DOMAIN_NAME", store, "domainName"),
			prefix + "OS_USERNAME":    matchEnvValueFrom(prefix+"OS_USERNAME", store, "username"),
			prefix + "OS_PASSWORD":    matchEnvValueFrom(prefix+"OS_PASSWORD", store, "password"),
			prefix + "OS_TENANT_NAME": matchEnvValueFrom(prefix+"OS_TENANT_NAME", store, "tenantName"),
		}
	case "OSS":
		return Elements{
			prefix + "ALICLOUD_ENDPOINT":          matchEnvValueFrom(prefix+"ALICLOUD_ENDPOINT", store, "storageEndpoint"),
			prefix + "ALICLOUD_ACCESS_KEY_SECRET": matchEnvValueFrom(prefix+"ALICLOUD_ACCESS_KEY_SECRET", store, "accessKeySecret"),
			prefix + "ALICLOUD_ACCESS_KEY_ID":     matchEnvValueFrom(prefix+"ALICLOUD_ACCESS_KEY_ID", store, "accessKeyID"),
		}
	default:
		return nil
	}
}

func getVolumeMountsElements(storeProvider, volumePrefix string) Elements {
	switch storeProvider {
	case "GCS":
		return Elements{
			volumePrefix + "etcd-backup": MatchFields(IgnoreExtras, Fields{
				"Name":      Equal(volumePrefix + "etcd-backup"),
				"MountPath": Equal(fmt.Sprintf("/root/.%sgcp/", volumePrefix)),
			}),
		}
	default:
		return nil
	}
}

func getVolumesElements(storeProvider, volumePrefix string, store *druidv1alpha1.StoreSpec) Elements {
	switch storeProvider {
	case "GCS":
		return Elements{
			volumePrefix + "etcd-backup": MatchAllFields(Fields{
				"Name": Equal(volumePrefix + "etcd-backup"),
				"VolumeSource": MatchFields(IgnoreExtras, Fields{
					"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
						"SecretName": Equal(store.SecretRef.Name),
					})),
				}),
			}),
		}
	default:
		return nil
	}
}

func matchEnvValueFrom(name string, store *druidv1alpha1.StoreSpec, key string) GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"Name": Equal(name),
		"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
			"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
				"LocalObjectReference": MatchAllFields(Fields{
					"Name": Equal(store.SecretRef.Name),
				}),
				"Key": Equal(key),
			})),
		})),
	})
}

func getJobStatus(conditionType batchv1.JobConditionType, reason, message string) *batchv1.JobStatus {
	now := metav1.Now()
	return &batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{
			{
				Type:               conditionType,
				Status:             corev1.ConditionTrue,
				LastProbeTime:      now,
				LastTransitionTime: now,
				Reason:             reason,
				Message:            message,
			},
		},
	}
}

func matchTaskStatus(jobStatus *batchv1.JobStatus) GomegaMatcher {
	conditionElements := Elements{}
	for _, jobCondition := range jobStatus.Conditions {
		var conditionType druidv1alpha1.ConditionType
		switch jobCondition.Type {
		case batchv1.JobComplete:
			conditionType = druidv1alpha1.EtcdCopyBackupsTaskSucceeded
		case batchv1.JobFailed:
			conditionType = druidv1alpha1.EtcdCopyBackupsTaskFailed
		}
		if conditionType != "" {
			conditionElements[string(conditionType)] = MatchFields(IgnoreExtras, Fields{
				"Type":               Equal(conditionType),
				"Status":             Equal(druidv1alpha1.ConditionStatus(jobCondition.Status)),
				"LastUpdateTime":     Equal(jobCondition.LastProbeTime),
				"LastTransitionTime": Equal(jobCondition.LastTransitionTime),
				"Reason":             Equal(jobCondition.Reason),
				"Message":            Equal(jobCondition.Message),
			})
		}
	}
	return MatchFields(IgnoreExtras, Fields{
		"Status": MatchFields(IgnoreExtras, Fields{
			"Conditions":         MatchAllElements(conditionIdentifier, conditionElements),
			"ObservedGeneration": Equal(pointer.Int64Ptr(1)),
		}),
	})
}

func matchFinalizer(finalizer string) GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Finalizers": MatchAllElements(stringIdentifier, Elements{
				finalizer: Equal(finalizer),
			}),
		}),
	})
}

func addEqual(elements Elements, s string) {
	elements[s] = Equal(s)
}

func conditionIdentifier(element interface{}) string {
	return string((element.(druidv1alpha1.Condition)).Type)
}

func stringIdentifier(element interface{}) string {
	return element.(string)
}

func uint32Ptr(v uint32) *uint32 { return &v }
