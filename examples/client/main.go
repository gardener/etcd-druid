// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	druidclient "github.com/gardener/etcd-druid/client/clientset/versioned"

	"golang.org/x/exp/slog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	// Get the rest.Config
	// Either define "--kubeconfig" command line flag or set "KUBECONFIG" environment property
	config := controllerruntime.GetConfigOrDie()

	// Create a typed druid client
	cl := druidclient.NewForConfigOrDie(config)

	ctx := context.Background()

	// Use the typed client to create an Etcd resource.
	etcdCluster, err := cl.DruidV1alpha1().Etcds("default").Create(ctx, &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name: "etcd-test",
		},
		Spec: druidv1alpha1.EtcdSpec{
			Labels: map[string]string{
				druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
				druidv1alpha1.LabelPartOfKey:    "etcd-test",
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
					druidv1alpha1.LabelPartOfKey:    "etcd-test",
				},
			},
			Replicas:            1,
			StorageCapacity:     ptr.To(resource.MustParse("10Gi")),
			StorageClass:        ptr.To("standard"),
			VolumeClaimTemplate: ptr.To("etcd-test"),
			Common: druidv1alpha1.SharedConfig{
				AutoCompactionMode:      ptr.To(druidv1alpha1.Periodic),
				AutoCompactionRetention: ptr.To("30m"),
			},
			Etcd: druidv1alpha1.EtcdConfig{
				Quota:                   ptr.To(resource.MustParse("8Gi")),
				SnapshotCount:           ptr.To(int64(10000)),
				DefragmentationSchedule: ptr.To("0 */24 * * *"),
				ServerPort:              ptr.To[int32](2380),
				ClientPort:              ptr.To[int32](2379),
				WrapperPort:             ptr.To[int32](9095),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"cpu":    resource.MustParse("500m"),
						"memory": resource.MustParse("1000Mi"),
					},
				},
				EtcdDefragTimeout: &metav1.Duration{
					Duration: 10 * time.Minute,
				},
				HeartbeatDuration: &metav1.Duration{
					Duration: 10 * time.Second,
				},
			},
		},
	}, metav1.CreateOptions{})

	if err != nil {
		slog.Error("Error creating Etcd resource", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("Successfully created Etcd cluster", "Etcd", client.ObjectKeyFromObject(etcdCluster))
}
