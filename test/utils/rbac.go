// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
