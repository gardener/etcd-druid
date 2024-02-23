// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *component) syncPeerService(ctx context.Context, svc *corev1.Service) error {
	_, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, c.client, svc, func() error {
		svc.Labels = c.values.Labels
		svc.OwnerReferences = []metav1.OwnerReference{c.values.OwnerReference}
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.ClusterIP = corev1.ClusterIPNone
		svc.Spec.SessionAffinity = corev1.ServiceAffinityNone
		svc.Spec.Selector = c.values.SelectorLabels
		svc.Spec.PublishNotReadyAddresses = true
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "peer",
				Protocol:   corev1.ProtocolTCP,
				Port:       c.values.PeerPort,
				TargetPort: intstr.FromInt(int(c.values.PeerPort)),
			},
		}

		return nil
	})
	return err
}
