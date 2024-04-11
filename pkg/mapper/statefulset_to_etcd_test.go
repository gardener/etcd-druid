// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mapper_test

import (
	"context"
	"fmt"

	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	. "github.com/gardener/etcd-druid/pkg/mapper"
	mockclient "github.com/gardener/etcd-druid/pkg/mock/controller-runtime/client"
)

var _ = Describe("Druid Mapper", func() {
	var (
		ctx    = context.Background()
		ctrl   *gomock.Controller
		c      *mockclient.MockClient
		logger logr.Logger

		name, namespace, key string
		statefulset          *appsv1.StatefulSet
		mapper               mapper.Mapper
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		c = mockclient.NewMockClient(ctrl)
		logger = log.Log.WithName("Test")

		name = "etcd-test"
		namespace = "test"
		key = fmt.Sprintf("%s/%s", namespace, name)
		statefulset = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
		mapper = StatefulSetToEtcd(ctx, c)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("#StatefulSetToEtcd", func() {
		It("should find related Etcd object", func() {
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			}

			c.EXPECT().Get(ctx, kutil.Key(namespace, name), gomock.AssignableToTypeOf(etcd), gomock.Any()).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, obj *druidv1alpha1.Etcd, _ ...client.GetOption) error {
					*obj = *etcd
					return nil
				},
			)

			kutil.SetMetaDataAnnotation(statefulset, common.GardenerOwnedBy, key)

			etcds := mapper.Map(ctx, logger, nil, statefulset)

			Expect(etcds).To(ConsistOf(
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: namespace,
						Name:      name,
					},
				},
			))
		})

		It("should not find related Etcd object because an error occurred during retrieval", func() {
			c.EXPECT().Get(ctx, kutil.Key(namespace, name), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(fmt.Errorf("foo error"))

			kutil.SetMetaDataAnnotation(statefulset, common.GardenerOwnedBy, key)

			etcds := mapper.Map(ctx, logger, nil, statefulset)

			Expect(etcds).To(BeEmpty())
		})

		It("should not find related Etcd object because owner annotation is not present", func() {
			etcds := mapper.Map(ctx, logger, nil, statefulset)

			Expect(etcds).To(BeEmpty())
		})

		It("should not find related Etcd object because map is called with wrong object", func() {
			etcds := mapper.Map(ctx, logger, nil, nil)

			Expect(etcds).To(BeEmpty())
		})
	})
})
