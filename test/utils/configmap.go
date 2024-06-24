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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ConfigMapIsCorrectlyReconciled(c client.Client, timeout time.Duration, instance *druidv1alpha1.Etcd, cm *corev1.ConfigMap) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6])),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, cm); err != nil {
		return err
	}

	if !CheckEtcdOwnerReference(cm.GetOwnerReferences(), instance.UID) {
		return fmt.Errorf("ownerReference does not exists")
	}
	return nil
}
