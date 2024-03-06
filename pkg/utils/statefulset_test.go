// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/test/utils"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("tests for statefulset utility functions", func() {

	var (
		ctx        context.Context
		fakeClient = fake.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
	)

	Describe("#IsStatefulSetReady", func() {
		const (
			stsName      = "etcd-test-0"
			stsNamespace = "test-ns"
		)
		It("statefulset has less number of ready replicas as compared to configured etcd replicas", func() {
			sts := utils.CreateStatefulSet(stsName, stsNamespace, uuid.NewUUID(), 2)
			sts.Generation = 1
			sts.Status.ObservedGeneration = 1
			sts.Status.Replicas = 2
			sts.Status.ReadyReplicas = 2
			ready, reasonMsg := IsStatefulSetReady(3, sts)
			Expect(ready).To(BeFalse())
			Expect(reasonMsg).ToNot(BeNil())
		})

		It("statefulset has equal number of replicas as defined in etcd but observed generation is outdated", func() {
			sts := utils.CreateStatefulSet(stsName, stsNamespace, uuid.NewUUID(), 3)
			sts.Generation = 2
			sts.Status.ObservedGeneration = 1
			sts.Status.Replicas = 3
			sts.Status.ReadyReplicas = 3
			ready, reasonMsg := IsStatefulSetReady(3, sts)
			Expect(ready).To(BeFalse())
			Expect(reasonMsg).ToNot(BeNil())
		})

		It("statefulset has equal number of replicas as defined in etcd and observed generation = generation", func() {
			sts := utils.CreateStatefulSet(stsName, stsNamespace, uuid.NewUUID(), 3)
			utils.SetStatefulSetReady(sts)
			ready, reasonMsg := IsStatefulSetReady(3, sts)
			Expect(ready).To(BeTrue())
			Expect(len(reasonMsg)).To(BeZero())
		})
	})

	Describe("#GetStatefulSet", func() {
		var (
			etcd             *druidv1alpha1.Etcd
			stsListToCleanup *appsv1.StatefulSetList
		)
		const (
			testEtcdName  = "etcd-test-0"
			testNamespace = "test-ns"
		)

		BeforeEach(func() {
			ctx = context.TODO()
			stsListToCleanup = &appsv1.StatefulSetList{}
			etcd = utils.EtcdBuilderWithDefaults(testEtcdName, testNamespace).Build()
		})

		AfterEach(func() {
			for _, sts := range stsListToCleanup.Items {
				Expect(fakeClient.Delete(ctx, &sts)).To(Succeed())
			}
		})

		It("no statefulset is found irrespective of ownership", func() {
			sts, err := GetStatefulSet(ctx, fakeClient, etcd)
			Expect(err).To(BeNil())
			Expect(sts).To(BeNil())
		})

		It("statefulset is present but it is not owned by etcd", func() {
			sts := utils.CreateStatefulSet(etcd.Name, etcd.Namespace, uuid.NewUUID(), 3)
			Expect(fakeClient.Create(ctx, sts)).To(Succeed())
			stsListToCleanup.Items = append(stsListToCleanup.Items, *sts)
			foundSts, err := GetStatefulSet(ctx, fakeClient, etcd)
			Expect(err).To(BeNil())
			Expect(foundSts).To(BeNil())
		})

		It("found statefulset owned by etcd", func() {
			sts := utils.CreateStatefulSet(etcd.Name, etcd.Namespace, etcd.UID, 3)
			Expect(fakeClient.Create(ctx, sts)).To(Succeed())
			stsListToCleanup.Items = append(stsListToCleanup.Items, *sts)
			foundSts, err := GetStatefulSet(ctx, fakeClient, etcd)
			Expect(err).To(BeNil())
			Expect(foundSts).ToNot(BeNil())
			Expect(foundSts.UID).To(Equal(sts.UID))
		})
	})
})
