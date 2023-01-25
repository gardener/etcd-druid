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
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

// CreateEtcdCopyBackupsTask creates an instance of EtcdCopyBackupsTask for the given provider and optional fields boolean.
func CreateEtcdCopyBackupsTask(provider druidv1alpha1.StorageProvider, withOptionalFields bool) *druidv1alpha1.EtcdCopyBackupsTask {
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
func uint32Ptr(v uint32) *uint32 { return &v }
