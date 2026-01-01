// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// podHasLabels checks if a pod has the given labels.
func podHasLabels(pod *corev1.Pod, labels map[string]string) bool {
	podLabels := pod.GetLabels()
	for key, value := range labels {
		if podLabels[key] != value {
			return false
		}
	}
	return true
}

// CheckEtcdPodLabels checks if all pods for a given etcd have the given labels.
func CheckEtcdPodLabels(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd, labels map[string]string) error {
	var errs error
	podList := druidv1alpha1.GetAllPodNames(etcd.ObjectMeta, etcd.Spec.Replicas)

	for _, podName := range podList {
		pod := &corev1.Pod{}
		if err := cl.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: podName}, pod); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to get pod %s/%s from etcd: %w", pod.Namespace, podName, err))
		}
		if !podHasLabels(pod, labels) {
			errs = errors.Join(errs, fmt.Errorf("pod %s/%s does not have the expected labels", pod.Namespace, podName))
		}
	}

	return errs
}
