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
	"strconv"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const peerURLTLSEnabledKey = "member.etcd.gardener.cloud/tls-enabled"

// IsPeerURLTLSEnabledForAllMembers checks if TLS has been enabled for all existing members of an etcd cluster identified by etcdName and in the provided namespace.
func IsPeerURLTLSEnabledForAllMembers(ctx context.Context, cl client.Client, logger logr.Logger, namespace, etcdName string) (bool, error) {
	leaseList := &coordinationv1.LeaseList{}
	if err := cl.List(ctx, leaseList, client.InNamespace(namespace), client.MatchingLabels(map[string]string{
		druidv1alpha1.LabelComponentKey: common.MemberLeaseComponentName,
		druidv1alpha1.LabelPartOfKey:    etcdName,
		druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
	})); err != nil {
		return false, err
	}
	tlsEnabledForAllMembers := true
	for _, lease := range leaseList.Items {
		tlsEnabled, err := parseAndGetTLSEnabledValue(lease, logger)
		if err != nil {
			return false, err
		}
		tlsEnabledForAllMembers = tlsEnabledForAllMembers && tlsEnabled
	}
	return tlsEnabledForAllMembers, nil
}

func parseAndGetTLSEnabledValue(lease coordinationv1.Lease, logger logr.Logger) (bool, error) {
	if lease.Annotations != nil {
		if tlsEnabledStr, ok := lease.Annotations[peerURLTLSEnabledKey]; ok {
			tlsEnabled, err := strconv.ParseBool(tlsEnabledStr)
			if err != nil {
				logger.Error(err, "tls-enabled value is not a valid boolean", "namespace", lease.Namespace, "leaseName", lease.Name)
				return false, err
			}
			return tlsEnabled, nil
		}
		logger.V(4).Info("tls-enabled annotation not present for lease.", "namespace", lease.Namespace, "leaseName", lease.Name)
	}
	return false, nil
}
