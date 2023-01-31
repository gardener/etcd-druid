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
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ServiceAccountIsCorrectlyReconciled(c client.Client, timeout time.Duration, instance *druidv1alpha1.Etcd, sa *corev1.ServiceAccount) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, sa); err != nil {
		return err
	}
	return nil
}

func RoleIsCorrectlyReconciled(c client.Client, timeout time.Duration, instance *druidv1alpha1.Etcd, role *rbacv1.Role) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("druid.gardener.cloud:etcd:%s", instance.Name),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, role); err != nil {
		return err
	}
	return nil
}

func RoleBindingIsCorrectlyReconciled(c client.Client, timeout time.Duration, instance *druidv1alpha1.Etcd, rb *rbacv1.RoleBinding) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("druid.gardener.cloud:etcd:%s", instance.Name),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, rb); err != nil {
		return err
	}
	return nil
}
