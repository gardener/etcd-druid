// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package lease

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gardener/etcd-druid/pkg/common"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/flow"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *component) deleteAllMemberLeases(ctx context.Context) error {
	labels := getMemberLeaseLabels(c.values.EtcdName)

	return c.client.DeleteAllOf(ctx, &coordinationv1.Lease{}, client.InNamespace(c.namespace), client.MatchingLabels(labels))
}

func (c *component) syncMemberLeases(ctx context.Context) error {
	var (
		fns []flow.TaskFn

		labels     = getMemberLeaseLabels(c.values.EtcdName)
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
				lease.OwnerReferences = getOwnerReferences(c.values)
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

func (c *component) getTLSEnabledAnnotationValues(ctx context.Context, etcdName string) ([]bool, error) {
	var tlsEnabledValues []bool
	labels := getMemberLeaseLabels(etcdName)
	leaseList := &coordinationv1.LeaseList{}
	if err := c.client.List(ctx, leaseList, client.InNamespace(c.namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}
	for _, lease := range leaseList.Items {
		tlsEnabled := c.parseAndGetTLSEnabledValue(lease)
		if tlsEnabled != nil {
			tlsEnabledValues = append(tlsEnabledValues, *tlsEnabled)
		}
	}
	return tlsEnabledValues, nil
}

func (c *component) parseAndGetTLSEnabledValue(lease coordinationv1.Lease) *bool {
	const peerURLTLSEnabledKey = "member.etcd.gardener.cloud/tls-enabled"
	if lease.Annotations != nil {
		if tlsEnabledStr, ok := lease.Annotations[peerURLTLSEnabledKey]; ok {
			tlsEnabled, err := strconv.ParseBool(tlsEnabledStr)
			if err != nil {
				c.logger.Error(err, "tls-enabled value is not a valid boolean", "namespace", lease.Namespace, "leaseName", lease.Name)
				return nil
			}
			return &tlsEnabled
		}
		c.logger.V(4).Info("tls-enabled annotation not present for lease.", "namespace", lease.Namespace, "leaseName", lease.Name)
	}
	return nil
}

// PurposeMemberLease is a constant used as a purpose for etcd member lease objects.
const PurposeMemberLease = "etcd-member-lease"

func getMemberLeaseLabels(etcdName string) map[string]string {
	return map[string]string{
		common.GardenerOwnedBy:           etcdName,
		v1beta1constants.GardenerPurpose: PurposeMemberLease,
	}
}

func memberLeaseName(etcdName string, replica int) string {
	return fmt.Sprintf("%s-%d", etcdName, replica)
}
