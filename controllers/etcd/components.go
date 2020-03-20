// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package etcd

import (
	"bytes"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/chartrenderer"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
)

////////////////////
//Chart rendering //
////////////////////
func (r *Reconciler) getInternalServiceFromChart(renderedChart *chartrenderer.RenderedChart) (*corev1.Service, error) {
	serviceManifest, ok := renderedChart.Files()[getChartPathForService()]
	if !ok {
		return nil, fmt.Errorf("missing service template file in the charts: %v", getChartPathForService())
	}

	svc := &corev1.Service{}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(serviceManifest)), 1024)
	if err := decoder.Decode(svc); err != nil {
		return nil, err
	}

	return svc, nil
}

func (r *Reconciler) getConfigmapFromChart(renderedChart *chartrenderer.RenderedChart) (*corev1.ConfigMap, error) {
	cmManifest, ok := renderedChart.Files()[getChartPathForConfigMap()]
	if !ok {
		return nil, fmt.Errorf("missing configmap template file in the charts: %v", getChartPathForConfigMap())
	}

	cm := &corev1.ConfigMap{}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(cmManifest)), 1024)
	if err := decoder.Decode(cm); err != nil {
		return nil, err
	}

	return cm, nil
}

func (r *Reconciler) getStatefulSetFromChart(renderedChart *chartrenderer.RenderedChart) (*appsv1.StatefulSet, error) {
	var err error
	sts := &appsv1.StatefulSet{}
	statefulSetPath := getChartPathForStatefulSet()

	if _, ok := renderedChart.Files()[statefulSetPath]; !ok {
		return nil, fmt.Errorf("missing configmap template file in the charts: %v", statefulSetPath)
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(renderedChart.Files()[statefulSetPath])), 1024)
	if err = decoder.Decode(sts); err != nil {
		return nil, err
	}
	return sts, nil
}

func (r *Reconciler) getExternalServiceFromEtcd(etcd *druidv1alpha1.Etcd, serviceName string) (*corev1.Service, error) {
	selector, err := metav1.LabelSelectorAsMap(etcd.Spec.Selector)
	if err != nil {
		return nil, err
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: etcd.Namespace,
			Labels: utils.MergeStringMaps(selector, map[string]string{
				common.ServiceScopeLabel: common.ServiceScopeExternal,
			}),
		},
		Spec: corev1.ServiceSpec{
			//ClusterIP:                assign existing clusterIP,
			Type:                     corev1.ServiceTypeClusterIP,
			Selector:                 utils.MergeStringMaps(selector, map[string]string{"healthy": "false"}),
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				{
					Name:       "etcd-client",
					Protocol:   corev1.ProtocolTCP,
					Port:       2379,
					TargetPort: intstr.FromInt(2379),
				},
				{
					Name:       "backup",
					Protocol:   corev1.ProtocolTCP,
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}

	return svc, nil
}
