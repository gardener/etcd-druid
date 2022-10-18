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

func (m *statefulSetToEtcdMapper) Map(ctx context.Context, log logr.Logger, reader client.Reader, obj client.Object) []reconcile.Request {
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
