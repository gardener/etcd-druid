// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *component) syncClientService(ctx context.Context, svc *corev1.Service) error {
	_, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, c.client, svc, func() error {
		svc.Labels = utils.MergeStringMaps(c.values.Labels, c.values.ClientServiceLabels)
		svc.Annotations = c.values.ClientServiceAnnotations
		svc.OwnerReferences = []metav1.OwnerReference{c.values.OwnerReference}
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.SessionAffinity = corev1.ServiceAffinityNone
		svc.Spec.Selector = c.values.SelectorLabels
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
				Port:       c.values.PeerPort,
				TargetPort: intstr.FromInt(int(c.values.PeerPort)),
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
