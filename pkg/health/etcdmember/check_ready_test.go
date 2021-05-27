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

package etcdmember_test

import (
	"time"

	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/config"
	. "github.com/gardener/etcd-druid/pkg/health/etcdmember"
)

var _ = Describe("ReadyCheck", func() {
	Describe("#Check", func() {
		var (
			threshold time.Duration
			now       time.Time
			check     Checker
		)
		BeforeEach(func() {
			threshold = 300 * time.Second
			now, _ = time.Parse(time.RFC3339, "2021-06-01T00:00:00Z")
			check = ReadyCheck(config.EtcdCustodianController{
				EtcdStaleMemberThreshold: threshold,
			})
		})

		Context("when condition is outdated", func() {
			It("should set the affected condition to UNKNOWN", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()
				status := druidv1alpha1.EtcdStatus{
					Members: []druidv1alpha1.EtcdMemberStatus{
						{
							Name:               "member1",
							ID:                 "1",
							Role:               druidv1alpha1.EtcdRoleMember,
							Status:             druidv1alpha1.EtcdMemeberStatusReady,
							Reason:             "foo reason",
							LastTransitionTime: metav1.Now(),
							LastUpdateTime:     metav1.NewTime(now.Add(-301 * time.Second)),
						},
						{
							Name:               "member2",
							ID:                 "2",
							Role:               druidv1alpha1.EtcdRoleMember,
							Status:             druidv1alpha1.EtcdMemeberStatusReady,
							Reason:             "bar reason",
							LastTransitionTime: metav1.Now(),
							LastUpdateTime:     metav1.NewTime(now.Add(-1 * threshold)),
						},
					},
				}

				check := ReadyCheck(config.EtcdCustodianController{
					EtcdStaleMemberThreshold: threshold,
				})

				results := check.Check(status)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemeberStatusUnknown))
				Expect(results[0].ID()).To(Equal("1"))
			})
		})
		Context("when condition is not outdated", func() {
			It("should not return any results", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()
				status := druidv1alpha1.EtcdStatus{
					Members: []druidv1alpha1.EtcdMemberStatus{
						{
							Name:               "member1",
							ID:                 "1",
							Role:               druidv1alpha1.EtcdRoleMember,
							Status:             druidv1alpha1.EtcdMemeberStatusReady,
							Reason:             "foo reason",
							LastTransitionTime: metav1.Now(),
							LastUpdateTime:     metav1.Now(),
						},
						{
							Name:               "member2",
							ID:                 "2",
							Role:               druidv1alpha1.EtcdRoleMember,
							Status:             druidv1alpha1.EtcdMemeberStatusReady,
							Reason:             "bar reason",
							LastTransitionTime: metav1.Now(),
							LastUpdateTime:     metav1.NewTime(now.Add(-1 * threshold)),
						},
					},
				}

				results := check.Check(status)

				Expect(results).To(BeEmpty())
			})
		})
	})
})
