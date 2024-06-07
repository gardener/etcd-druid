// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateEvent creates an event with the given parameters.
func CreateEvent(name, namespace, reason, message, eventType string, involvedObject client.Object, involvedObjectGVK schema.GroupVersionKind) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion:      involvedObjectGVK.GroupVersion().String(),
			Kind:            involvedObjectGVK.Kind,
			Name:            involvedObject.GetName(),
			Namespace:       involvedObject.GetNamespace(),
			UID:             involvedObject.GetUID(),
			ResourceVersion: involvedObject.GetResourceVersion(),
		},
		Reason:  reason,
		Message: message,
		Type:    eventType,
	}
}
