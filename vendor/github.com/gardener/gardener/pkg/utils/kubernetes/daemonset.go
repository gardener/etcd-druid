// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package kubernetes

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

// PodManagedByDaemonSet returns 'true' if the given pod is managed by a DaemonSet, determined by the existing owner references.
func PodManagedByDaemonSet(pod *corev1.Pod) bool {
	for _, ownerRef := range pod.ObjectMeta.OwnerReferences {
		if ownerRef.APIVersion == appsv1.SchemeGroupVersion.String() &&
			ownerRef.Kind == "DaemonSet" &&
			pointer.BoolDeref(ownerRef.Controller, false) {
			return true
		}
	}

	return false
}
