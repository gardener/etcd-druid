// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"context"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type etcdToSecretMapper struct{}

func (m *etcdToSecretMapper) Map(_ context.Context, _ logr.Logger, _ client.Reader, obj client.Object) []reconcile.Request {
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
