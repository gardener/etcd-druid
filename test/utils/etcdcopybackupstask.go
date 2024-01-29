// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

// CreateEtcdCopyBackupsTask creates an instance of EtcdCopyBackupsTask for the given provider and optional fields boolean.
func CreateEtcdCopyBackupsTask(name, namespace string, provider druidv1alpha1.StorageProvider, withOptionalFields bool) *druidv1alpha1.EtcdCopyBackupsTask {
	var (
		maxBackupAge, maxBackups *uint32
		waitForFinalSnapshot     *druidv1alpha1.WaitForFinalSnapshotSpec
	)
	if withOptionalFields {
		maxBackupAge = pointer.Uint32(7)
		maxBackups = pointer.Uint32(42)
		waitForFinalSnapshot = &druidv1alpha1.WaitForFinalSnapshotSpec{
			Enabled: true,
			Timeout: &metav1.Duration{Duration: 10 * time.Minute},
		}
	}
	return &druidv1alpha1.EtcdCopyBackupsTask{
		TypeMeta: metav1.TypeMeta{
			APIVersion: druidv1alpha1.GroupVersion.String(),
			Kind:       "EtcdCopyBackupsTask",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: druidv1alpha1.EtcdCopyBackupsTaskSpec{
			SourceStore: druidv1alpha1.StoreSpec{
				Container: pointer.String("source-container"),
				Prefix:    "/tmp",
				Provider:  &provider,
				SecretRef: &corev1.SecretReference{
					Name:      "source-etcd-backup",
					Namespace: namespace,
				},
			},
			TargetStore: druidv1alpha1.StoreSpec{
				Container: pointer.String("target-container"),
				Prefix:    "/tmp",
				Provider:  &provider,
				SecretRef: &corev1.SecretReference{
					Name:      "target-etcd-backup",
					Namespace: namespace,
				},
			},
			MaxBackupAge:         maxBackupAge,
			MaxBackups:           maxBackups,
			WaitForFinalSnapshot: waitForFinalSnapshot,
		},
	}
}

// CreateEtcdCopyBackupsJob creates an instance of a Job owned by a EtcdCopyBackupsTask with the given name.
func CreateEtcdCopyBackupsJob(taskName, namespace string) *batchv1.Job {
	jobName := taskName + "-worker"
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels:    getLabels(taskName, jobName, false),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         druidv1alpha1.GroupVersion.String(),
					Kind:               "EtcdCopyBackupsTask",
					Name:               taskName,
					UID:                "",
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				},
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: getLabels(taskName, jobName, true),
				},
				Spec: corev1.PodSpec{
					Volumes: nil,
					Containers: []corev1.Container{
						{
							Name:            "copy-backups",
							Image:           "europe-docker.pkg.dev/gardener-project/public/gardener/etcdbrctl",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            []string{"copy"}, // since this is only used for testing the command here is not complete.
						},
					},
					RestartPolicy: "OnFailure",
				},
			},
		},
	}
}

func getLabels(taskName, jobName string, includeNetworkPolicyLabels bool) map[string]string {
	labels := map[string]string{
		druidv1alpha1.LabelPartOfKey:    taskName,
		druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
		druidv1alpha1.LabelComponentKey: common.EtcdCopyBackupTaskComponentName,
		druidv1alpha1.LabelAppNameKey:   jobName,
	}
	if includeNetworkPolicyLabels {
		labels[v1beta1constants.LabelNetworkPolicyToDNS] = v1beta1constants.LabelNetworkPolicyAllowed
		labels[v1beta1constants.LabelNetworkPolicyToPublicNetworks] = v1beta1constants.LabelNetworkPolicyAllowed
	}
	return labels
}
