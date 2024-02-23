// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lease

import (
	"context"

	"github.com/gardener/gardener/pkg/controllerutils"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *component) deleteSnapshotLease(ctx context.Context, lease *coordinationv1.Lease) error {
	return client.IgnoreNotFound(c.client.Delete(ctx, lease))
}

func (c *component) syncSnapshotLease(ctx context.Context, lease *coordinationv1.Lease) error {
	if !c.values.BackupEnabled {
		return c.deleteSnapshotLease(ctx, lease)
	}
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, c.client, lease, func() error {
		lease.OwnerReferences = []metav1.OwnerReference{c.values.OwnerReference}
		return nil
	})
	return err
}
