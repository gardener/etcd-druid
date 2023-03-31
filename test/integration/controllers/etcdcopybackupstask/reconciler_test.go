// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package etcdcopybackupstask

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/pkg/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	timeout         = 1 * time.Minute
	pollingInterval = 2 * time.Second
)

var _ = Describe("EtcdCopyBackupsTask Controller", func() {
	var (
		ctx = context.Background()
	)

	DescribeTable("when creating and deleting etcdcopybackupstask",
		func(taskName string, provider druidv1alpha1.StorageProvider, withOptionalFields bool, jobStatus *batchv1.JobStatus) {
			task := testutils.CreateEtcdCopyBackupsTask(taskName, namespace, provider, withOptionalFields)

			// Create secrets
			errors := testutils.CreateSecrets(ctx, k8sClient, task.Namespace, task.Spec.SourceStore.SecretRef.Name, task.Spec.TargetStore.SecretRef.Name)
			Expect(errors).Should(BeNil())

			// Create task
			Expect(k8sClient.Create(ctx, task)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(task), task)
			}).Should(Not(HaveOccurred()))

			// Wait until the job has been created
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      task.Name + "-worker",
					Namespace: task.Namespace,
				},
			}
			Eventually(func() (*batchv1.Job, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
					return nil, err
				}
				return job, nil
			}, timeout, pollingInterval).Should(PointTo(matchJob(task, imageVector)))

			// Update job status
			job.Status = *jobStatus
			err := k8sClient.Status().Update(ctx, job)
			Expect(err).NotTo(HaveOccurred())

			// Wait until the task status has been updated
			Eventually(func() (*druidv1alpha1.EtcdCopyBackupsTask, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(task), task); err != nil {
					return nil, err
				}
				return task, nil
			}, timeout, pollingInterval).Should(PointTo(matchTaskStatus(&job.Status)))

			// Delete task
			Expect(k8sClient.Delete(ctx, task)).To(Succeed())

			// Wait until the job gets the "foregroundDeletion" finalizer and remove it
			Eventually(func() (*batchv1.Job, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
					return nil, err
				}
				return job, nil
			}, timeout, pollingInterval).Should(PointTo(testutils.MatchFinalizer(metav1.FinalizerDeleteDependents)))
			Expect(controllerutils.RemoveFinalizers(ctx, k8sClient, job, metav1.FinalizerDeleteDependents)).To(Succeed())

			// Wait until the job has been deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), &batchv1.Job{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

			// Wait until the task has been deleted
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(task), &druidv1alpha1.EtcdCopyBackupsTask{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
		},
		Entry("should create the job, update the task status, and delete the job if the job completed",
			"foo01", druidv1alpha1.StorageProvider("Local"), true, getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job failed",
			"foo02", druidv1alpha1.StorageProvider("Local"), false, getJobStatus(batchv1.JobFailed, "test reason", "test message")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for aws",
			"foo03", druidv1alpha1.StorageProvider("aws"), false, getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for azure",
			"foo04", druidv1alpha1.StorageProvider("azure"), false, getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for gcp",
			"foo05", druidv1alpha1.StorageProvider("gcp"), false, getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for openstack",
			"foo06", druidv1alpha1.StorageProvider("openstack"), false, getJobStatus(batchv1.JobComplete, "", "")),
		Entry("should create the job, update the task status, and delete the job if the job completed, for alicloud",
			"foo07", druidv1alpha1.StorageProvider("alicloud"), false, getJobStatus(batchv1.JobComplete, "", "")),
		// ref https://github.com/gardener/etcd-druid/issues/532
		Entry("should correctly handle ownerReferences for numeric names with leading 0",
			"01234", druidv1alpha1.StorageProvider("Local"), true, getJobStatus(batchv1.JobComplete, "", "")),
	)
})

func matchJob(task *druidv1alpha1.EtcdCopyBackupsTask, imageVector imagevector.ImageVector) gomegatypes.GomegaMatcher {
	sourceProvider, err := utils.StorageProviderFromInfraProvider(task.Spec.SourceStore.Provider)
	Expect(err).NotTo(HaveOccurred())
	targetProvider, err := utils.StorageProviderFromInfraProvider(task.Spec.TargetStore.Provider)
	Expect(err).NotTo(HaveOccurred())

	images, err := imagevector.FindImages(imageVector, []string{common.BackupRestore})
	Expect(err).NotTo(HaveOccurred())
	backupRestoreImage := images[common.BackupRestore]

	matcher := MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(task.Name + "-worker"),
			"Namespace": Equal(task.Namespace),
			"Annotations": MatchKeys(IgnoreExtras, Keys{
				"gardener.cloud/owned-by":   Equal(fmt.Sprintf("%s/%s", task.Namespace, task.Name)),
				"gardener.cloud/owner-type": Equal("etcdcopybackupstask"),
			}),
			"OwnerReferences": MatchAllElements(testutils.OwnerRefIterator, Elements{
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
					"Containers": MatchAllElements(testutils.ContainerIterator, Elements{
						"copy-backups": MatchFields(IgnoreExtras, Fields{
							"Name":            Equal("copy-backups"),
							"Image":           Equal(fmt.Sprintf("%s:%s", backupRestoreImage.Repository, *backupRestoreImage.Tag)),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Command":         MatchAllElements(testutils.CmdIterator, getCmdElements(task, sourceProvider, targetProvider)),
							"Env":             MatchElements(testutils.EnvIterator, IgnoreExtras, getEnvElements(task)),
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
			prefix + "AWS_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + "AWS_APPLICATION_CREDENTIALS"),
				"Value": Equal(fmt.Sprintf("/root/%setcd-backup", volumePrefix)),
			}),
		}
	case "ABS":
		return Elements{
			prefix + "AZURE_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + "AZURE_APPLICATION_CREDENTIALS"),
				"Value": Equal(fmt.Sprintf("/root/%setcd-backup", volumePrefix)),
			}),
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
			prefix + "OPENSTACK_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + "OPENSTACK_APPLICATION_CREDENTIALS"),
				"Value": Equal(fmt.Sprintf("/root/%setcd-backup", volumePrefix)),
			}),
		}
	case "OSS":
		return Elements{
			prefix + "ALICLOUD_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + "ALICLOUD_APPLICATION_CREDENTIALS"),
				"Value": Equal(fmt.Sprintf("/root/%setcd-backup", volumePrefix)),
			}),
		}
	case "OCS":
		return Elements{
			prefix + "OPENSHIFT_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
				"Name":  Equal(prefix + "OPENSHIFT_APPLICATION_CREDENTIALS"),
				"Value": Equal(fmt.Sprintf("/root/%setcd-backup", volumePrefix)),
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
			volumePrefix + "etcd-backup": MatchFields(IgnoreExtras, Fields{
				"Name":      Equal(volumePrefix + "etcd-backup"),
				"MountPath": Equal(fmt.Sprintf("/root/.%sgcp/", volumePrefix)),
			}),
		}
	default:
		return Elements{
			volumePrefix + "etcd-backup": MatchFields(IgnoreExtras, Fields{
				"Name":      Equal(volumePrefix + "etcd-backup"),
				"MountPath": Equal(fmt.Sprintf("/root/%setcd-backup", volumePrefix)),
			}),
		}
	}
}

func getVolumesElements(volumePrefix string, store *druidv1alpha1.StoreSpec) Elements {

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

func matchTaskStatus(jobStatus *batchv1.JobStatus) gomegatypes.GomegaMatcher {
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
			"ObservedGeneration": Equal(pointer.Int64(1)),
		}),
	})
}

func addEqual(elements Elements, s string) {
	elements[s] = Equal(s)
}

func conditionIdentifier(element interface{}) string {
	return string((element.(druidv1alpha1.Condition)).Type)
}
