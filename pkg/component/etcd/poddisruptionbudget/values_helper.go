// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package poddisruptionbudget

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
)

// GenerateValues generates `poddisruptionbudget.Values` for the lease component with the given parameters.
func GenerateValues(etcd *druidv1alpha1.Etcd) Values {
	annotations := map[string]string{
		common.GardenerOwnedBy:   fmt.Sprintf("%s/%s", etcd.Namespace, etcd.Name),
		common.GardenerOwnerType: "etcd",
	}

	return Values{
		Name:           etcd.Name,
		Labels:         etcd.GetDefaultLabels(),
		SelectorLabels: etcd.GetDefaultLabels(),
		Annotations:    annotations,
		MinAvailable:   int32(CalculatePDBMinAvailable(etcd)),
		OwnerReference: etcd.GetAsOwnerReference(),
	}
}

// CalculatePDBMinAvailable calculates the minimum available value for the PDB
func CalculatePDBMinAvailable(etcd *druidv1alpha1.Etcd) int {
	// do not enable for single node cluster
	if etcd.Spec.Replicas <= 1 {
		return 0
	}

	clusterSize := int(etcd.Spec.Replicas)
	return clusterSize/2 + 1

}
