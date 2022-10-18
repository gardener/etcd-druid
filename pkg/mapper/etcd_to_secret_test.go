// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package mapper_test

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/pkg/mapper"
	"github.com/go-logr/logr"

	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("EtcdToSecret", func() {
	var (
		ctx    = context.Background()
		m      mapper.Mapper
		etcd   *druidv1alpha1.Etcd
		logger logr.Logger

		namespace = "some-namespace"
	)

	BeforeEach(func() {
		m = EtcdToSecret()

		etcd = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
			},
		}
		logger = log.Log.WithName("Test")
	})

	It("should return empty list because Etcd is not referencing secrets", func() {
		Expect(m.Map(ctx, logger, nil, etcd)).To(BeEmpty())
	})

	It("should return four requests because Etcd is referencing secrets", func() {
		var (
			secretClientCATLS     = "client-url-ca-etcd"
			secretClientServerTLS = "client-url-etcd-server-tls"
			secretClientClientTLS = "client-url-etcd-client-tls"
			secretPeerCATLS       = "peer-url-ca-etcd"
			secretPeerServerTLS   = "peer-url-etcd-server-tls"
			secretBackupStore     = "backup-store"
		)

		etcd.Spec.Etcd.ClientUrlTLS = &druidv1alpha1.TLSConfig{
			TLSCASecretRef: druidv1alpha1.SecretReference{
				SecretReference: corev1.SecretReference{Name: secretClientCATLS},
			},
			ServerTLSSecretRef: corev1.SecretReference{Name: secretClientServerTLS},
			ClientTLSSecretRef: corev1.SecretReference{Name: secretClientClientTLS},
		}

		etcd.Spec.Etcd.PeerUrlTLS = &druidv1alpha1.TLSConfig{
			TLSCASecretRef: druidv1alpha1.SecretReference{
				SecretReference: corev1.SecretReference{Name: secretPeerCATLS},
			},
			ServerTLSSecretRef: corev1.SecretReference{Name: secretPeerServerTLS},
		}

		etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
			SecretRef: &corev1.SecretReference{Name: secretBackupStore},
		}

		Expect(m.Map(ctx, logger, nil, etcd)).To(ConsistOf(
			reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      secretClientCATLS,
				Namespace: namespace,
			}},
			reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      secretClientServerTLS,
				Namespace: namespace,
			}},
			reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      secretClientClientTLS,
				Namespace: namespace,
			}},
			reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      secretPeerCATLS,
				Namespace: namespace,
			}},
			reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      secretPeerServerTLS,
				Namespace: namespace,
			}},
			reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      secretBackupStore,
				Namespace: namespace,
			}},
		))
	})
})
