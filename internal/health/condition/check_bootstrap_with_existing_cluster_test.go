// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition_test

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	. "github.com/gardener/etcd-druid/internal/health/condition"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BootstrapWithExistingClusterCheck", func() {
	Describe("#Check", func() {
		var (
			readyMember    druidv1alpha1.EtcdMemberStatus
			notReadyMember druidv1alpha1.EtcdMemberStatus
			sourceMembers  []druidv1alpha1.BootstrapExistingMember
			joinedMembers  []druidv1alpha1.BootstrapJoinedMember
		)

		BeforeEach(func() {
			readyMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusReady,
			}
			notReadyMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusNotReady,
			}
			sourceMembers = []druidv1alpha1.BootstrapExistingMember{
				{Name: "src-0", PeerURLs: []string{"https://src-0.peer.src-ns.svc:2380"}},
				{Name: "src-1", PeerURLs: []string{"https://src-1.peer.src-ns.svc:2380"}},
				{Name: "src-2", PeerURLs: []string{"https://src-2.peer.src-ns.svc:2380"}},
			}
			joinedMembers = []druidv1alpha1.BootstrapJoinedMember{
				{Name: "src-0", PeerURLs: []string{"https://src-0.peer.src-ns.svc:2380"}, JoinedAt: metav1.Now()},
				{Name: "src-1", PeerURLs: []string{"https://src-1.peer.src-ns.svc:2380"}, JoinedAt: metav1.Now()},
				{Name: "src-2", PeerURLs: []string{"https://src-2.peer.src-ns.svc:2380"}, JoinedAt: metav1.Now()},
			}
		})

		Context("when bootstrapWithExistingCluster is not configured", func() {
			It("should return nil so no condition is reported", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{readyMember, readyMember, readyMember},
					},
				}
				check := BootstrapWithExistingClusterCheck(nil)

				Expect(check.Check(context.TODO(), etcd)).To(BeNil())
			})

			It("should return nil when members slice is explicitly empty", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
						Etcd: druidv1alpha1.EtcdConfig{
							BootstrapWithExistingCluster: &druidv1alpha1.BootstrapWithExistingCluster{
								Members: nil,
							},
						},
					},
				}
				check := BootstrapWithExistingClusterCheck(nil)

				Expect(check.Check(context.TODO(), etcd)).To(BeNil())
			})
		})

		Context("when bootstrapWithExistingCluster is configured but bootstrap is in progress", func() {
			It("should return False/BootstrapInProgress when fewer members have registered than spec.replicas", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
						Etcd: druidv1alpha1.EtcdConfig{
							BootstrapWithExistingCluster: &druidv1alpha1.BootstrapWithExistingCluster{
								Members: sourceMembers,
							},
						},
					},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{readyMember, readyMember},
					},
				}
				check := BootstrapWithExistingClusterCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result).NotTo(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBootstrapWithExistingCluster))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal("BootstrapInProgress"))
				Expect(result.Message()).To(Equal("Not all members have joined the cluster yet"))
			})

			It("should return False/BootstrapInProgress when at least one member is not ready", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
						Etcd: druidv1alpha1.EtcdConfig{
							BootstrapWithExistingCluster: &druidv1alpha1.BootstrapWithExistingCluster{
								Members: sourceMembers,
							},
						},
					},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{readyMember, notReadyMember, readyMember},
					},
				}
				check := BootstrapWithExistingClusterCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result).NotTo(BeNil())
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal("BootstrapInProgress"))
				Expect(result.Message()).To(Equal("Not all members are ready"))
			})
		})

		Context("when bootstrap has just completed in this reconcile pass", func() {
			It("should return True/BootstrapSucceeded when all members are ready and status is not yet populated", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
						Etcd: druidv1alpha1.EtcdConfig{
							BootstrapWithExistingCluster: &druidv1alpha1.BootstrapWithExistingCluster{
								Members: sourceMembers,
							},
						},
					},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{readyMember, readyMember, readyMember},
						// BootstrapWithExistingClusterMembers populated by
						// reconcile_status only after this condition first
						// reaches True; on this pass it's still empty.
					},
				}
				check := BootstrapWithExistingClusterCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result).NotTo(BeNil())
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal("BootstrapSucceeded"))
				Expect(result.Message()).To(Equal("All members have successfully joined the existing cluster"))
			})
		})

		Context("when bootstrap has succeeded on a previous reconcile pass (sticky)", func() {
			It("should stay True even when a member becomes not ready post-bootstrap", func() {
				// Regression test for the review comment that the condition
				// must not flip back to BootstrapInProgress on a transient
				// member outage post-bootstrap. Bootstrap is one-shot.
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
						Etcd: druidv1alpha1.EtcdConfig{
							BootstrapWithExistingCluster: &druidv1alpha1.BootstrapWithExistingCluster{
								Members: sourceMembers,
							},
						},
					},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{readyMember, notReadyMember, readyMember},
						BootstrapWithExistingClusterMembers: joinedMembers,
					},
				}
				check := BootstrapWithExistingClusterCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result).NotTo(BeNil())
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal("BootstrapSucceeded"))
			})

			It("should stay True even when fewer members have registered than spec.replicas", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
						Etcd: druidv1alpha1.EtcdConfig{
							BootstrapWithExistingCluster: &druidv1alpha1.BootstrapWithExistingCluster{
								Members: sourceMembers,
							},
						},
					},
					Status: druidv1alpha1.EtcdStatus{
						Members:                             []druidv1alpha1.EtcdMemberStatus{readyMember, readyMember},
						BootstrapWithExistingClusterMembers: joinedMembers,
					},
				}
				check := BootstrapWithExistingClusterCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result).NotTo(BeNil())
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal("BootstrapSucceeded"))
			})

			It("should stay True after spec.bootstrapWithExistingCluster has been cleared (member-removal trigger)", func() {
				// Member-removal trigger contract: when spec is cleared but
				// status still records joined members, the future
				// member-removal flow runs. Throughout that window the
				// condition must remain True until removal completes and
				// status is wiped.
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
						// BootstrapWithExistingCluster intentionally nil.
					},
					Status: druidv1alpha1.EtcdStatus{
						Members:                             []druidv1alpha1.EtcdMemberStatus{readyMember, readyMember, readyMember},
						BootstrapWithExistingClusterMembers: joinedMembers,
					},
				}
				check := BootstrapWithExistingClusterCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result).NotTo(BeNil())
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal("BootstrapSucceeded"))
			})
		})
	})
})
