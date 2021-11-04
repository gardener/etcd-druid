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

package etcd_test

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	. "github.com/gardener/etcd-druid/pkg/component/etcd"

	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	. "github.com/onsi/gomega/types"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Etcd", func() {
	var (
		ctx       context.Context
		c         client.Client
		namespace string
		name      string
		uid       types.UID
		replicas  int

		etcd Interface
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
		name = "etcd-test"
		replicas = 3
		uid = "123"

		c = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
		etcd = New(c, namespace, Values{
			BackupEnabled: true,
			EtcdName:      name,
			EtcdUID:       uid,
			Replicas:      replicas,
		})
	})

	Describe("Deploy", func() {
		Context("when deployment is new", func() {
			It("should successfully deploy", func() {
				Expect(etcd.Deploy(ctx)).To(Succeed())

				checkMemberLeases(ctx, c, namespace, name, uid, replicas)
				checkSnapshotLeases(ctx, c, namespace, name, uid)
			})
		})

		Context("when deployment is changed from 5 -> 3 replicas", func() {
			BeforeEach(func() {
				// Create existing leases
				for _, l := range []coordinationv1.Lease{
					{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%d", name, 0)}},
					{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%d", name, 1)}},
					{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%d", name, 2)}},
					{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%d", name, 3)}},
					{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%d", name, 4)}},
				} {
					lease := l
					Expect(c.Create(ctx, &lease)).To(Succeed())
				}
			})

			It("should successfully deploy", func() {
				Expect(etcd.Deploy(ctx)).To(Succeed())

				checkMemberLeases(ctx, c, namespace, name, uid, replicas)
				checkSnapshotLeases(ctx, c, namespace, name, uid)
			})
		})

		Context("when deployment is changed from 1 -> 3 replicas", func() {
			BeforeEach(func() {
				// Create existing leases
				for _, l := range []coordinationv1.Lease{
					{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%d", name, 0)}},
				} {
					lease := l
					Expect(c.Create(ctx, &lease)).To(Succeed())
				}
			})

			It("should successfully deploy", func() {
				Expect(etcd.Deploy(ctx)).To(Succeed())

				checkMemberLeases(ctx, c, namespace, name, uid, replicas)
				checkSnapshotLeases(ctx, c, namespace, name, uid)
			})
		})

		Context("when backup is disabled", func() {
			BeforeEach(func() {
				etcd = New(c, namespace, Values{
					BackupEnabled: false,
					EtcdName:      name,
					EtcdUID:       uid,
					Replicas:      replicas,
				})
			})

			It("should successfully deploy", func() {
				Expect(etcd.Deploy(ctx)).To(Succeed())

				checkMemberLeases(ctx, c, namespace, name, uid, replicas)

				_, err := getDeltaSnapshotLease(ctx, c, namespace, name)
				Expect(err).To(matchers.BeNotFoundError())
				_, err = getFullSnapshotLease(ctx, c, namespace, name)
				Expect(err).To(matchers.BeNotFoundError())
			})
		})
	})
})

func getDeltaSnapshotLease(ctx context.Context, c client.Client, namespace, name string) (*coordinationv1.Lease, error) {
	deltaLeaseName := fmt.Sprintf("delta-snapshot-%s", name)
	deltaLease := coordinationv1.Lease{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: deltaLeaseName}, &deltaLease); err != nil {
		return nil, err
	}
	return &deltaLease, nil
}

func getFullSnapshotLease(ctx context.Context, c client.Client, namespace, name string) (*coordinationv1.Lease, error) {
	fullLeaseName := fmt.Sprintf("full-snapshot-%s", name)
	fullLease := coordinationv1.Lease{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fullLeaseName}, &fullLease); err != nil {
		return nil, err
	}
	return &fullLease, nil
}

func checkSnapshotLeases(ctx context.Context, c client.Client, namespace, name string, etcdUID types.UID) {
	deltaLease, err := getDeltaSnapshotLease(ctx, c, namespace, name)
	Expect(err).NotTo(HaveOccurred())
	Expect(deltaLease).To(PointTo(matchLeaseElement(deltaLease.Name, name, etcdUID)))

	fullLease, err := getFullSnapshotLease(ctx, c, namespace, name)
	Expect(err).NotTo(HaveOccurred())
	Expect(fullLease).To(PointTo(matchLeaseElement(fullLease.Name, name, etcdUID)))
}

func checkMemberLeases(ctx context.Context, c client.Client, namespace, name string, etcdUID types.UID, replicas int) {
	leases := &coordinationv1.LeaseList{}
	Expect(c.List(ctx, leases, client.InNamespace(namespace), client.MatchingLabels(map[string]string{
		common.GardenerOwnedBy: name,
		common.GardenerPurpose: "etcd-member-lease",
	}))).To(Succeed())

	Expect(leases.Items).To(ConsistOf(memberLeases(name, etcdUID, replicas)))
}

func memberLeases(name string, etcdUID types.UID, replicas int) []interface{} {
	var elements []interface{}
	for i := 0; i < replicas; i++ {
		elements = append(elements, matchLeaseElement(fmt.Sprintf("%s-%d", name, i), name, etcdUID))
	}

	return elements
}

func matchLeaseElement(leasName, etcdName string, etcdUID types.UID) GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name": Equal(leasName),
			"OwnerReferences": ConsistOf(MatchFields(IgnoreExtras, Fields{
				"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
				"Kind":               Equal("etcd"),
				"Name":               Equal(etcdName),
				"UID":                Equal(etcdUID),
				"Controller":         PointTo(BeTrue()),
				"BlockOwnerDeletion": PointTo(BeTrue()),
			})),
		}),
	})
}
