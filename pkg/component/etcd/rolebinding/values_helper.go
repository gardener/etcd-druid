// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package rolebinding

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"
)

// GenerateValues generates `serviceaccount.Values` for the serviceaccount component with the given `etcd` object.
func GenerateValues(etcd *druidv1alpha1.Etcd) *Values {
	return &Values{
		Name:               utils.GetRoleBindingName(etcd),
		Namespace:          etcd.Namespace,
		Labels:             etcd.Spec.Labels,
		OwnerReferences:    utils.GenerateEtcdOwnerReference(etcd),
		RoleName:           utils.GetRoleName(etcd),
		ServiceAccountName: utils.GetServiceAccountName(etcd),
	}
}
