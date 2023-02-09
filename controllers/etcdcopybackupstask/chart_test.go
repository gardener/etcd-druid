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

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/common"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EtcdCopyBackupsTaskController Chart", func() {
	var (
		ctx        context.Context
		reconciler *Reconciler
		task       *druidv1alpha1.EtcdCopyBackupsTask
	)
	const (
		testTaskName  = "test-task"
		testNamespace = "test-ns"
	)

	BeforeEach(func() {
		ctx = context.Background()
		revertFunc := testutils.SwitchDirectory("../..")
		defer revertFunc()
		imageVector, err := utils.CreateImageVector()
		Expect(err).ToNot(HaveOccurred())
		reconciler = &Reconciler{
			imageVector: imageVector,
		}
		task = testutils.CreateEtcdCopyBackupsTask(testTaskName, testNamespace, "aws", true)
	})

	Describe("getChartValues", func() {
		var (
			expectedValues     map[string]interface{}
			backupRestoreImage string
		)

		BeforeEach(func() {
			images, err := imagevector.FindImages(reconciler.imageVector, []string{common.BackupRestore})
			Expect(err).ToNot(HaveOccurred())
			val, ok := images[common.BackupRestore]
			Expect(ok).To(BeTrue())
			backupRestoreImage = val.String()

			expectedValues = map[string]interface{}{
				"name":      task.Name + "-worker",
				"ownerName": task.Name,
				"ownerUID":  task.UID,
				"sourceStore": map[string]interface{}{
					"storePrefix":      task.Spec.SourceStore.Prefix,
					"storageProvider":  "S3",
					"storageContainer": task.Spec.SourceStore.Container,
					"storeSecret":      task.Spec.SourceStore.SecretRef.Name,
				},
				"targetStore": map[string]interface{}{
					"storePrefix":      task.Spec.TargetStore.Prefix,
					"storageProvider":  "S3",
					"storageContainer": task.Spec.TargetStore.Container,
					"storeSecret":      task.Spec.TargetStore.SecretRef.Name,
				},
				"maxBackupAge":                *task.Spec.MaxBackupAge,
				"maxBackups":                  *task.Spec.MaxBackups,
				"waitForFinalSnapshot":        task.Spec.WaitForFinalSnapshot.Enabled,
				"waitForFinalSnapshotTimeout": task.Spec.WaitForFinalSnapshot.Timeout,
				"image":                       backupRestoreImage,
			}
		})

		It("should return correct chart values", func() {
			values, err := reconciler.getChartValues(ctx, task)
			Expect(err).ToNot(HaveOccurred())
			Expect(values).To(Equal(expectedValues))
		})
	})
})
