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

package condition_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/pkg/health/condition"
)

var _ = Describe("AllMembersReadyCheck", func() {
	Describe("#Check", func() {
		var readyMember, notReadyMember druidv1alpha1.EtcdMemberStatus

		BeforeEach(func() {
			readyMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusReady,
			}
			notReadyMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusNotReady,
			}
		})

		Context("when members in status", func() {
			It("should return that all members are ready", func() {
				etcd := druidv1alpha1.Etcd{
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							readyMember,
							readyMember,
						},
					},
				}
				check := AllMembersCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
			})

			It("should return that members are not ready", func() {
				etcd := druidv1alpha1.Etcd{
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							notReadyMember,
							readyMember,
						},
					},
				}
				check := AllMembersCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
			})
		})

		Context("when no members in status", func() {
			It("should return that readiness is unknown", func() {
				etcd := druidv1alpha1.Etcd{
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{},
					},
				}
				check := AllMembersCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
			})
		})
	})
})
