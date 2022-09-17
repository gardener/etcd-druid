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

package lease_test

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/utils/pointer"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	. "github.com/gardener/etcd-druid/pkg/component/etcd/lease"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Lease", func() {
	var (
		ctx context.Context
		c   client.Client

		etcd      *druidv1alpha1.Etcd
		namespace string
		name      string
		uid       types.UID
		replicas  int32

		values        Values
		leaseDeployer component.Deployer
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
		name = "etcd-test"
		replicas = 3
		uid = "123"

		etcd = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       uid,
			},
			Spec: druidv1alpha1.EtcdSpec{
				Backup: druidv1alpha1.BackupSpec{
					Store: new(druidv1alpha1.StoreSpec),
				},
				Replicas: replicas,
			},
		}

		c = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()

		values = GenerateValues(etcd)
		leaseDeployer = New(c, logr.Discard(), namespace, values)
	})

	Describe("#Deploy", func() {
		Context("when deployment is new", func() {
			It("should successfully deploy", func() {
				Expect(leaseDeployer.Deploy(ctx)).To(Succeed())

				checkMemberLeases(ctx, c, etcd)
				checkSnapshotLeases(ctx, c, etcd, values)
			})
		})

		Context("when deployment is changed from 5 -> 3 replicas", func() {
			var etcd2 *druidv1alpha1.Etcd

			BeforeEach(func() {
				etcd2 = etcd.DeepCopy()
				etcd2.Namespace = "kube-system"
				etcd2.Spec.Replicas = 5

				// Create existing leases
				for _, l := range []coordinationv1.Lease{
					memberLease(etcd, 0, false),
					memberLease(etcd, 1, false),
					memberLease(etcd, 2, false),
					memberLease(etcd, 3, false),
					memberLease(etcd, 4, false),
					// Add leases for another etcd in a different namespace which is not scaled down.
					memberLease(etcd2, 0, true),
					memberLease(etcd2, 1, true),
					memberLease(etcd2, 2, true),
					memberLease(etcd2, 3, true),
					memberLease(etcd2, 4, true),
				} {
					lease := l
					Expect(c.Create(ctx, &lease)).To(Succeed())
				}
			})

			It("should successfully deploy", func() {
				Expect(leaseDeployer.Deploy(ctx)).To(Succeed())

				checkMemberLeases(ctx, c, etcd)
				checkMemberLeases(ctx, c, etcd2)
				checkSnapshotLeases(ctx, c, etcd, values)
			})
		})

		Context("when deployment is changed from 1 -> 3 replicas", func() {
			BeforeEach(func() {
				// Create existing leases
				for _, l := range []coordinationv1.Lease{
					memberLease(etcd, 0, false),
				} {
					lease := l
					Expect(c.Create(ctx, &lease)).To(Succeed())
				}
			})

			It("should successfully deploy", func() {
				Expect(leaseDeployer.Deploy(ctx)).To(Succeed())

				checkMemberLeases(ctx, c, etcd)
				checkSnapshotLeases(ctx, c, etcd, values)
			})
		})

		Context("when backup is disabled", func() {
			BeforeEach(func() {
				leaseDeployer = New(c,
					logr.Discard(),
					namespace, Values{
						BackupEnabled: false,
						EtcdName:      name,
						EtcdUID:       uid,
						Replicas:      replicas,
					})
			})

			It("should successfully deploy", func() {
				// Snapshot leases might exist before, so create them here.
				for _, l := range []coordinationv1.Lease{
					{ObjectMeta: metav1.ObjectMeta{Name: values.DeltaSnapshotLeaseName}},
					{ObjectMeta: metav1.ObjectMeta{Name: values.FullSnapshotLeaseName}},
				} {
					lease := l
					Expect(c.Create(ctx, &lease)).To(Succeed())
				}

				Expect(leaseDeployer.Deploy(ctx)).To(Succeed())

				checkMemberLeases(ctx, c, etcd)

				_, err := getSnapshotLease(ctx, c, namespace, values.DeltaSnapshotLeaseName)
				Expect(err).To(matchers.BeNotFoundError())
				_, err = getSnapshotLease(ctx, c, namespace, values.FullSnapshotLeaseName)
				Expect(err).To(matchers.BeNotFoundError())
			})
		})
	})

	Describe("#Destroy", func() {
		Context("when no leases exist", func() {
			It("should destroy without errors", func() {
				Expect(leaseDeployer.Destroy(ctx)).To(Succeed())
			})
		})

		Context("when leases exist", func() {
			It("should destroy without errors", func() {
				for _, l := range []coordinationv1.Lease{
					memberLease(etcd, 0, false),
					memberLease(etcd, 1, false),
					memberLease(etcd, 2, false),
					{ObjectMeta: metav1.ObjectMeta{Name: values.DeltaSnapshotLeaseName}},
					{ObjectMeta: metav1.ObjectMeta{Name: values.FullSnapshotLeaseName}},
				} {
					lease := l
					Expect(c.Create(ctx, &lease)).To(Succeed())
				}

				Expect(leaseDeployer.Destroy(ctx)).To(Succeed())

				_, err := getSnapshotLease(ctx, c, namespace, values.DeltaSnapshotLeaseName)
				Expect(err).To(matchers.BeNotFoundError())
				_, err = getSnapshotLease(ctx, c, namespace, values.FullSnapshotLeaseName)
				Expect(err).To(matchers.BeNotFoundError())

				leases := &coordinationv1.LeaseList{}
				Expect(c.List(ctx, leases, client.InNamespace(etcd.Namespace), client.MatchingLabels(map[string]string{
					common.GardenerOwnedBy:           etcd.Name,
					v1beta1constants.GardenerPurpose: "etcd-member-lease",
				}))).To(Succeed())

				Expect(leases.Items).To(BeEmpty())
			})
		})
	})
})

func getSnapshotLease(ctx context.Context, c client.Client, namespace, name string) (*coordinationv1.Lease, error) {
	snapshotLease := coordinationv1.Lease{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &snapshotLease); err != nil {
		return nil, err
	}
	return &snapshotLease, nil
}

func checkSnapshotLeases(ctx context.Context, c client.Client, etcd *druidv1alpha1.Etcd, val Values) {
	deltaLease, err := getSnapshotLease(ctx, c, etcd.Namespace, val.DeltaSnapshotLeaseName)
	Expect(err).NotTo(HaveOccurred())
	Expect(deltaLease).To(PointTo(matchLeaseElement(deltaLease.Name, etcd.Name, etcd.UID)))

	fullLease, err := getSnapshotLease(ctx, c, etcd.Namespace, val.FullSnapshotLeaseName)
	Expect(err).NotTo(HaveOccurred())
	Expect(fullLease).To(PointTo(matchLeaseElement(fullLease.Name, etcd.Name, etcd.UID)))
}

func checkMemberLeases(ctx context.Context, c client.Client, etcd *druidv1alpha1.Etcd) {
	leases := &coordinationv1.LeaseList{}
	Expect(c.List(ctx, leases, client.InNamespace(etcd.Namespace), client.MatchingLabels(map[string]string{
		common.GardenerOwnedBy:           etcd.Name,
		v1beta1constants.GardenerPurpose: "etcd-member-lease",
	}))).To(Succeed())

	Expect(leases.Items).To(ConsistOf(memberLeases(etcd.Name, etcd.UID, etcd.Spec.Replicas)))
}

func memberLeases(name string, etcdUID types.UID, replicas int32) []interface{} {
	var elements []interface{}
	for i := 0; i < int(replicas); i++ {
		elements = append(elements, matchLeaseElement(fmt.Sprintf("%s-%d", name, i), name, etcdUID))
	}

	return elements
}

func matchLeaseElement(leaseName, etcdName string, etcdUID types.UID) gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name": Equal(leaseName),
			"OwnerReferences": ConsistOf(MatchFields(IgnoreExtras, Fields{
				"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
				"Kind":               Equal("Etcd"),
				"Name":               Equal(etcdName),
				"UID":                Equal(etcdUID),
				"Controller":         PointTo(BeTrue()),
				"BlockOwnerDeletion": PointTo(BeTrue()),
			})),
		}),
	})
}

func memberLease(etcd *druidv1alpha1.Etcd, replica int, withOwnerRef bool) coordinationv1.Lease {
	lease := coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", etcd.Name, replica),
			Namespace: etcd.Namespace,
			Labels: map[string]string{
				common.GardenerOwnedBy:           etcd.Name,
				v1beta1constants.GardenerPurpose: "etcd-member-lease",
			},
		},
	}

	if withOwnerRef {
		lease.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         druidv1alpha1.GroupVersion.String(),
				Kind:               "Etcd",
				Name:               etcd.Name,
				UID:                etcd.UID,
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			},
		}
	}

	return lease
}
