// Copyright 2023 SAP SE or an SAP affiliate company
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

package utils

import (
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
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
		maxBackupAge = uint32Ptr(7)
		maxBackups = uint32Ptr(42)
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
func uint32Ptr(v uint32) *uint32 { return &v }

// CreateEtcdCopyBackupsJob creates an instance of a Job owned by a EtcdCopyBackupsTask with the given name.
func CreateEtcdCopyBackupsJob(taskName, namespace string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        taskName + "-worker",
			Namespace:   namespace,
			Labels:      nil,
			Annotations: createJobAnnotations(taskName, namespace),
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
				Spec: corev1.PodSpec{
					Volumes: nil,
					Containers: []corev1.Container{
						{
							Name:            "copy-backups",
							Image:           "eu.gcr.io/gardener-project/gardener/etcdbrctl",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"etcdbrctl", "copy"}, // since this is only used for testing the command here is not complete.
						},
					},
					RestartPolicy: "OnFailure",
				},
			},
		},
	}
}

func createJobAnnotations(taskName, namespace string) map[string]string {
	annotations := make(map[string]string, 2)
	annotations[common.GardenerOwnedBy] = fmt.Sprintf("%s/%s", namespace, taskName)
	annotations[common.GardenerOwnerType] = "etcdcopybackupstask"
	return annotations
}
