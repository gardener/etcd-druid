// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	k8sutils "github.com/gardener/etcd-druid/internal/utils/kubernetes"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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
				APIVersion:         druidv1alpha1.SchemeGroupVersion.String(),
				Kind:               "Etcd",
				Name:               etcdName,
				UID:                etcdUID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
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

// verifyPeerTLSEnabledOnMemberLease checks if peer TLS is enabled on the Etcd member via its corresponding member lease.
func verifyPeerTLSEnabledOnMemberLease(ctx context.Context, c client.Client, name, namespace string) error {
	lease := &coordinationv1.Lease{}
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, lease)
	if err != nil {
		return fmt.Errorf("failed to get lease %s/%s: %w", namespace, name, err)
	}
	if val, ok := lease.GetAnnotations()[k8sutils.LeaseAnnotationKeyPeerURLTLSEnabled]; ok {
		tlsEnabled, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("failed to parse TLS enabled annotation value %s on lease %s/%s: %w", val, namespace, name, err)
		}
		if tlsEnabled {
			return nil
		}
	}
	return fmt.Errorf("peer TLS is not enabled on lease %s/%s", namespace, name)
}

// VerifyPeerTLSEnabledOnAllMemberLeases checks if peer TLS is enabled on all member leases of the given Etcd cluster.
func VerifyPeerTLSEnabledOnAllMemberLeases(ctx context.Context, c client.Client, etcd *druidv1alpha1.Etcd) error {
	var errs error
	leaseNames := druidv1alpha1.GetMemberLeaseNames(etcd.ObjectMeta, etcd.Spec.Replicas)
	for _, leaseName := range leaseNames {
		err := verifyPeerTLSEnabledOnMemberLease(ctx, c, leaseName, etcd.Namespace)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("peer TLS enablement verification failed for member lease %s/%s: %w", etcd.Namespace, leaseName, err))
		}
	}
	return errs
}
