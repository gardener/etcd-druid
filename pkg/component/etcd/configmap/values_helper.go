// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package configmap

import (
	"fmt"
	"strconv"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"k8s.io/utils/pointer"
)

// GenerateValues generates `configmap.Values` for the configmap component with the given parameters.
func GenerateValues(etcd *druidv1alpha1.Etcd) *Values {
	initialCluster := prepareInitialCluster(etcd)
	values := &Values{
		Name:                    etcd.GetConfigMapName(),
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
		OwnerReference:          etcd.GetAsOwnerReference(),
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
