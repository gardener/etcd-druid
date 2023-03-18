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

package configmap

import (
	"fmt"
	"strconv"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

// GenerateValues generates `configmap.Values` for the configmap component with the given parameters.
func GenerateValues(etcd *druidv1alpha1.Etcd) *Values {
	initialCluster := prepareInitialCluster(etcd)
	values := &Values{
		Name:                    etcd.GetConfigmapName(),
		EtcdUID:                 etcd.UID,
		Metrics:                 etcd.Spec.Etcd.Metrics,
		Quota:                   etcd.Spec.Etcd.Quota,
		InitialCluster:          initialCluster,
		ClientUrlTLS:            etcd.Spec.Etcd.ClientUrlTLS,
		PeerUrlTLS:              etcd.Spec.Etcd.PeerUrlTLS,
		ClientServiceName:       etcd.GetClientServiceName(),
		ClientPort:              etcd.Spec.Etcd.ClientPort,
		PeerServiceName:         etcd.GetPeerServiceName(),
		ServerPort:              etcd.Spec.Etcd.ServerPort,
		AutoCompactionMode:      etcd.Spec.Common.AutoCompactionMode,
		AutoCompactionRetention: etcd.Spec.Common.AutoCompactionRetention,
		OwnerReferences:         []metav1.OwnerReference{etcd.GetAsOwnerReference()},
		Labels:                  etcd.GetDefaultLabels(),
	}
	return values
}

func prepareInitialCluster(etcd *druidv1alpha1.Etcd) string {
	protocol := "http"
	if etcd.Spec.Etcd.PeerUrlTLS != nil {
		protocol = "https"
	}

	statefulsetReplicas := int(etcd.Spec.Replicas)

	// Form the service name and pod name for mutinode cluster with the help of ETCD name
	podName := etcd.GetOrdinalPodName(0)
	domaiName := fmt.Sprintf("%s.%s.%s", etcd.GetPeerServiceName(), etcd.Namespace, "svc")
	serverPort := strconv.Itoa(int(pointer.Int32Deref(etcd.Spec.Etcd.ServerPort, defaultServerPort)))

	initialCluster := fmt.Sprintf("%s=%s://%s.%s:%s", podName, protocol, podName, domaiName, serverPort)
	if statefulsetReplicas > 1 {
		// form initial cluster
		initialCluster = ""
		for i := 0; i < statefulsetReplicas; i++ {
			podName = etcd.GetOrdinalPodName(i)
			initialCluster = initialCluster + fmt.Sprintf("%s=%s://%s.%s:%s,", podName, protocol, podName, domaiName, serverPort)
		}
	}

	initialCluster = strings.Trim(initialCluster, ",")
	return initialCluster
}
