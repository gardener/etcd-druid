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
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/test/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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
				task = ensureEtcdCopyBackupsTaskCreation(ctx, testTaskName, testNamespace, fakeClient)
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
				Expect(addDeletionTimestampToTask(ctx, task, time.Now(), fakeClient)).To(Succeed())

				_, err := r.delete(ctx, task)
				Expect(err).To(BeNil())
				Eventually(func() error {
					return fakeClient.Get(ctx, client.ObjectKeyFromObject(task), task)
				}).Should(BeNotFoundError())
			})

			It("should delete job but not the task for which the deletion timestamp, finalizer is set and job is present", func() {
				job := utils.CreateEtcdCopyBackupsJob(testTaskName, testNamespace)
				Expect(fakeClient.Create(ctx, job)).To(Succeed())
				Expect(controllerutils.AddFinalizers(ctx, fakeClient, task, common.FinalizerName)).To(Succeed())
				Expect(addDeletionTimestampToTask(ctx, task, time.Now(), fakeClient)).To(Succeed())
				_, err := r.delete(ctx, task)
				Expect(err).To(BeNil())
				Eventually(func() error {
					return fakeClient.Get(ctx, client.ObjectKeyFromObject(job), job)
				}).Should(BeNotFoundError())
				Eventually(func() error {
					return fakeClient.Get(ctx, client.ObjectKeyFromObject(task), task)
				}).Should(BeNil())
			})

		})

	})
})

func ensureEtcdCopyBackupsTaskCreation(ctx context.Context, name, namespace string, fakeClient client.WithWatch) *druidv1alpha1.EtcdCopyBackupsTask {
	task := utils.CreateEtcdCopyBackupsTask(name, namespace, "aws", false)
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
