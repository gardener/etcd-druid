// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lease

import (
	"context"
	"fmt"

	"github.com/gardener/etcd-druid/pkg/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/flow"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *component) deleteAllMemberLeases(ctx context.Context) error {
	labels := utils.GetMemberLeaseLabels(c.values.EtcdName)

	return c.client.DeleteAllOf(ctx, &coordinationv1.Lease{}, client.InNamespace(c.namespace), client.MatchingLabels(labels))
}

func (c *component) syncMemberLeases(ctx context.Context) error {
	var (
		fns []flow.TaskFn

		labels     = utils.GetMemberLeaseLabels(c.values.EtcdName)
		prefix     = c.values.EtcdName
		leaseNames = sets.NewString()
	)

	// Patch or create necessary member leases.
	for i := 0; i < int(c.values.Replicas); i++ {
		leaseName := memberLeaseName(prefix, i)

		lease := c.emptyLease(leaseName)
		fns = append(fns, func(ctx context.Context) error {
			_, err := controllerutils.GetAndCreateOrMergePatch(ctx, c.client, lease, func() error {
				if lease.Labels == nil {
					lease.Labels = make(map[string]string)
				}
				for k, v := range labels {
					lease.Labels[k] = v
				}
				lease.OwnerReferences = []metav1.OwnerReference{c.values.OwnerReference}
				return nil
			})
			return err
		})

		leaseNames = leaseNames.Insert(leaseName)
	}

	leaseList := &coordinationv1.LeaseList{}
	if err := c.client.List(ctx, leaseList, client.InNamespace(c.namespace), client.MatchingLabels(labels)); err != nil {
		return err
	}

	// Clean up superfluous member leases.
	for _, lease := range leaseList.Items {
		ls := lease
		if leaseNames.Has(ls.Name) {
			continue
		}
		fns = append(fns, func(ctx context.Context) error {
			if err := c.client.Delete(ctx, &ls); client.IgnoreNotFound(err) != nil {
				return err
			}
			return nil
		})
	}

	return flow.Parallel(fns...)(ctx)
}

func memberLeaseName(etcdName string, replica int) string {
	return fmt.Sprintf("%s-%d", etcdName, replica)
}
