// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	predicateutils "github.com/gardener/gardener/pkg/controllerutils/predicate"
	contextutil "github.com/gardener/gardener/pkg/utils/context"
)

type clusterToObjectMapper struct {
	ctx            context.Context
	client         client.Client
	newObjListFunc func() client.ObjectList
	predicates     []predicate.Predicate
}

func (m *clusterToObjectMapper) InjectClient(c client.Client) error {
	m.client = c
	return nil
}

func (m *clusterToObjectMapper) InjectStopChannel(stopCh <-chan struct{}) error {
	m.ctx = contextutil.FromStopChannel(stopCh)
	return nil
}

func (m *clusterToObjectMapper) InjectFunc(f inject.Func) error {
	for _, p := range m.predicates {
		if err := f(p); err != nil {
			return err
		}
	}
	return nil
}

func (m *clusterToObjectMapper) Map(obj client.Object) []reconcile.Request {
	cluster, ok := obj.(*extensionsv1alpha1.Cluster)
	if !ok {
		return nil
	}

	objList := m.newObjListFunc()
	if err := m.client.List(m.ctx, objList, client.InNamespace(cluster.Name)); err != nil {
		return nil
	}

	var requests []reconcile.Request

	utilruntime.HandleError(meta.EachListItem(objList, func(obj runtime.Object) error {
		o := obj.(client.Object)
		if !predicateutils.EvalGeneric(o, m.predicates...) {
			return nil
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: o.GetNamespace(),
				Name:      o.GetName(),
			},
		})
		return nil
	}))
	return requests
}

// ClusterToObjectMapper returns a mapper that returns requests for objects whose
// referenced clusters have been modified.
func ClusterToObjectMapper(newObjListFunc func() client.ObjectList, predicates []predicate.Predicate) Mapper {
	return &clusterToObjectMapper{newObjListFunc: newObjListFunc, predicates: predicates}
}
