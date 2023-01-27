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
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("EtcdCopyBackupsTaskController", func() {
	Describe("getConditions", func() {
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
})
