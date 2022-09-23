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

package service

import (
	"context"

	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *component) syncClientService(ctx context.Context, svc *corev1.Service) error {
	_, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, c.client, svc, func() error {
		svc.Labels = getLabels(c.values)
		svc.Annotations = c.values.ClientServiceAnnotations
		svc.OwnerReferences = getOwnerReferences(c.values)
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.SessionAffinity = corev1.ServiceAffinityNone
		svc.Spec.Selector = getLabels(c.values)
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "client",
				Protocol:   corev1.ProtocolTCP,
				Port:       c.values.ClientPort,
				TargetPort: intstr.FromInt(int(c.values.ClientPort)),
			},
			// TODO: Remove the "server" port in a future release
			{
				Name:       "server",
				Protocol:   corev1.ProtocolTCP,
				Port:       c.values.ServerPort,
				TargetPort: intstr.FromInt(int(c.values.ServerPort)),
			},
			{
				Name:       "backuprestore",
				Protocol:   corev1.ProtocolTCP,
				Port:       c.values.BackupPort,
				TargetPort: intstr.FromInt(int(c.values.BackupPort)),
			},
		}

		return nil
	})
	return err
}
