// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
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

package etcd

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/controllerutils"

	"github.com/gardener/gardener/pkg/utils/flow"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (e *etcd) emptyLease(name string) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: e.namespace,
		},
	}
}

func (e *etcd) reconcileLease(ctx context.Context, lease *coordinationv1.Lease, enabled bool) error {
	if !enabled {
		return client.IgnoreNotFound(e.client.Delete(ctx, lease))
	}
	_, err := controllerutils.MergePatchOrCreate(ctx, e.client, lease, func() error {
		lease.OwnerReferences = getOwnerReferences(e.values)
		return nil
	})
	return err
}

func (e *etcd) reconcileMemberLease(ctx context.Context) error {
	var (
		fns []flow.TaskFn

		prefix     = e.values.EtcdName
		labels     = getsMemberLeaseLabels(e.values)
		leaseNames = sets.NewString()
	)

	// Patch or create necessary member leases.
	for i := 0; i < e.values.Replicas; i++ {
		leaseName := memberLeaseName(prefix, i)

		lease := e.emptyLease(leaseName)
		fns = append(fns, func(ctx context.Context) error {
			_, err := controllerutils.MergePatchOrCreate(ctx, e.client, lease, func() error {
				if lease.Labels == nil {
					lease.Labels = make(map[string]string)
				}
				for k, v := range labels {
					lease.Labels[k] = v
				}
				lease.OwnerReferences = getOwnerReferences(e.values)
				return nil
			})
			return err
		})

		leaseNames = leaseNames.Insert(leaseName)
	}

	leaseList := &coordinationv1.LeaseList{}
	if err := e.client.List(ctx, leaseList, client.MatchingLabels(labels)); err != nil {
		return err
	}

	// Clean up superfluous member leases.
	for _, lease := range leaseList.Items {
		l := lease
		if leaseNames.Has(l.Name) {
			continue
		}
		fns = append(fns, func(ctx context.Context) error {
			if err := e.client.Delete(ctx, &l); client.IgnoreNotFound(err) != nil {
				return err
			}
			return nil
		})
	}

	return flow.Parallel(fns...)(ctx)
}

func getOwnerReferences(val Values) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         druidv1alpha1.GroupVersion.String(),
			Kind:               "etcd",
			Name:               val.EtcdName,
			UID:                val.EtcdUID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	}
}

func getsMemberLeaseLabels(val Values) map[string]string {
	return map[string]string{
		common.GardenerOwnedBy: val.EtcdName,
		common.GardenerPurpose: "etcd-member-lease",
	}
}

func memberLeaseName(etcdName string, replica int) string {
	return fmt.Sprintf("%s-%d", etcdName, replica)
}
