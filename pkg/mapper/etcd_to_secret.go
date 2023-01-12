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

package mapper

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/go-logr/logr"

	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type etcdToSecretMapper struct{}

func (m *etcdToSecretMapper) Map(ctx context.Context, log logr.Logger, reader client.Reader, obj client.Object) []reconcile.Request {
	etcd, ok := obj.(*druidv1alpha1.Etcd)
	if !ok {
		return nil
	}

	var requests []reconcile.Request

	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name,
		}})

		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name,
		}})

		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name,
		}})
	}

	if etcd.Spec.Etcd.PeerUrlTLS != nil {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name,
		}})

		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name,
		}})
	}

	if etcd.Spec.Backup.Store != nil && etcd.Spec.Backup.Store.SecretRef != nil {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Backup.Store.SecretRef.Name,
		}})
	}

	return requests
}

// EtcdToSecret returns a mapper that returns a request for the Secret resources
// referenced in the Etcd for which an event happened.
func EtcdToSecret() mapper.Mapper {
	return &etcdToSecretMapper{}
}
