// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/gardener/pkg/controllerutils/mapper"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/etcd-druid/pkg/common"
)

type statefulSetToEtcdMapper struct {
	ctx context.Context
	cl  client.Client
}

func (m *statefulSetToEtcdMapper) Map(_ context.Context, _ logr.Logger, _ client.Reader, obj client.Object) []reconcile.Request {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return nil
	}

	ownerKey, ok := sts.Annotations[common.GardenerOwnedBy]
	if !ok {
		return nil
	}

	name, namespace, err := cache.SplitMetaNamespaceKey(ownerKey)
	if err != nil {
		return nil
	}

	etcd := &druidv1alpha1.Etcd{}
	if err := m.cl.Get(m.ctx, kutil.Key(name, namespace), etcd); err != nil {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: etcd.Namespace,
				Name:      etcd.Name,
			},
		},
	}
}

// StatefulSetToEtcd returns a mapper that returns a request for the Etcd resource
// that owns the StatefulSet for which an event happened.
func StatefulSetToEtcd(ctx context.Context, cl client.Client) mapper.Mapper {
	return &statefulSetToEtcdMapper{
		ctx: ctx,
		cl:  cl,
	}
}
