// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func StatefulsetIsCorrectlyReconciled(ctx context.Context, c client.Client, instance *druidv1alpha1.Etcd, ss *appsv1.StatefulSet) error {
	req := types.NamespacedName{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, ss); err != nil {
		return err
	}
	if !CheckEtcdOwnerReference(ss.GetOwnerReferences(), instance) {
		return fmt.Errorf("ownerReference does not exist")
	}
	return nil
}

func StatefulSetRemoved(ctx context.Context, c client.Client, ss *appsv1.StatefulSet) error {
	sts := &appsv1.StatefulSet{}
	req := types.NamespacedName{
		Name:      ss.Name,
		Namespace: ss.Namespace,
	}
	if err := c.Get(ctx, req, sts); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers
			return nil
		}
		return err
	}
	return fmt.Errorf("statefulset not removed")
}

func SetStatefulSetReady(s *appsv1.StatefulSet) {
	s.Status.ObservedGeneration = s.Generation

	replicas := int32(1)
	if s.Spec.Replicas != nil {
		replicas = *s.Spec.Replicas
	}
	s.Status.Replicas = replicas
	s.Status.ReadyReplicas = replicas
}
