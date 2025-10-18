// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	druidclient "github.com/gardener/etcd-druid/client/clientset/versioned"

	"golang.org/x/exp/slog"
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

	// Use the typed client to create an EtcdOpsTask Resource.
	// for etcdopsTask for a different task, change the spec.config to be of the type of the etcdopstask needed.
	onDemandSnapshotEtcdOpsTask, err := cl.DruidV1alpha1().EtcdOpsTasks("default").Create(ctx, &druidv1alpha1.EtcdOpsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-ondemand-full-snapshot",
		},
		Spec: druidv1alpha1.EtcdOpsTaskSpec{
			Config: druidv1alpha1.EtcdOpsTaskConfig{
				OnDemandSnapshot: &druidv1alpha1.OnDemandSnapshotConfig{
					Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
					IsFinal:            ptr.To(true),
					TimeoutSecondsFull: ptr.To(int32(480)),
				},
			},
			EtcdName:                ptr.To("example-etcd"),
			TTLSecondsAfterFinished: ptr.To(int32(3600)),
		},
	}, metav1.CreateOptions{})

	if err != nil {
		slog.Error("Error creating EtcdOpsTask resource", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("Successfully created EtcdOpsTask", "EtcdOpsTask", client.ObjectKeyFromObject(onDemandSnapshotEtcdOpsTask))
}
