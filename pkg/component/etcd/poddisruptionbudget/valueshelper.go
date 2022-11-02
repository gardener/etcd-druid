// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package poddisruptionbudget

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

// GenerateValues generates `poddisruptionbudget.Values` for the lease component with the given parameters.
func GenerateValues(etcd *druidv1alpha1.Etcd) Values {
	labels := etcd.Spec.Labels
	labels[instanceKey] = etcd.Name
	labels[nameKey] = "etcd"
	labels[appKey] = "etcd-statefulset"

	selectorLabels := map[string]string{
		instanceKey: etcd.Name,
		nameKey:     "etcd",
	}

	annotations := map[string]string{
		ownedByAnnotationKey:   fmt.Sprintf("%s/%s", etcd.Namespace, etcd.Name),
		ownerTypeAnnotationKey: "etcd",
	}

	return Values{
		EtcdName:       etcd.Name,
		EtcdNameSpace:  etcd.Namespace,
		EtcdUID:        etcd.UID,
		Labels:         labels,
		SelectorLabels: selectorLabels,
		Annotations:    annotations,
		MinAvailable:   int32(CalculatePDBMinAvailable(etcd)),
	}
}

// CalculatePDBMinAvailable calculates the minimum available value for the PDB
func CalculatePDBMinAvailable(etcd *druidv1alpha1.Etcd) int {
	// do not enable for single node cluster
	if etcd.Spec.Replicas < 2 {
		return 0
	}

	clusterSize := int(etcd.Spec.Replicas)
	return clusterSize/2 + 1

}
