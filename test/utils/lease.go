// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateLease creates a lease with its owner reference set to etcd.
func CreateLease(name, namespace, etcdName string, etcdUID types.UID, componentName string) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				druidv1alpha1.LabelPartOfKey:    etcdName,
				druidv1alpha1.LabelComponentKey: componentName,
				druidv1alpha1.LabelAppNameKey:   name,
				druidv1alpha1.LabelManagedByKey: "etcd-druid",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         druidv1alpha1.GroupVersion.String(),
				Kind:               "Etcd",
				Name:               etcdName,
				UID:                etcdUID,
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			}},
		},
	}
}

// IsLeaseRemoved checks if the given etcd object is removed
func IsLeaseRemoved(c client.Client, name, namespace string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	lease := &coordinationv1.Lease{}
	req := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	if err := c.Get(ctx, req, lease); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers
			return nil
		}
		return err
	}
	return fmt.Errorf("lease not deleted")
}
