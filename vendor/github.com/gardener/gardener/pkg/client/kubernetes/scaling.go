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

package kubernetes

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/gardener/pkg/utils/retry"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ScaleStatefulSet scales a StatefulSet.
func ScaleStatefulSet(ctx context.Context, c client.Client, key client.ObjectKey, replicas int32) error {
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
	}

	return scaleResource(ctx, c, statefulset, replicas)
}

// ScaleEtcd scales a Etcd resource.
func ScaleEtcd(ctx context.Context, c client.Client, key client.ObjectKey, replicas int) error {
	etcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
	}

	return scaleResource(ctx, c, etcd, int32(replicas))
}

// ScaleDeployment scales a Deployment.
func ScaleDeployment(ctx context.Context, c client.Client, key client.ObjectKey, replicas int32) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
	}

	return scaleResource(ctx, c, deployment, replicas)
}

// scaleResource scales resource's 'spec.replicas' to replicas count
func scaleResource(ctx context.Context, c client.Client, obj client.Object, replicas int32) error {
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))

	// TODO: replace this with call to scale subresource once controller-runtime supports it
	// see: https://github.com/kubernetes-sigs/controller-runtime/issues/172
	return c.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch))
}

// WaitUntilDeploymentScaledToDesiredReplicas waits for the number of available replicas to be equal to the deployment's desired replicas count.
func WaitUntilDeploymentScaledToDesiredReplicas(ctx context.Context, client client.Client, key types.NamespacedName, desiredReplicas int32) error {
	return retry.UntilTimeout(ctx, 5*time.Second, 300*time.Second, func(ctx context.Context) (done bool, err error) {
		deployment := &appsv1.Deployment{}
		if err := client.Get(ctx, key, deployment); err != nil {
			return retry.SevereError(err)
		}

		if deployment.Generation != deployment.Status.ObservedGeneration {
			return retry.MinorError(fmt.Errorf("%q not observed at latest generation (%d/%d)", key.Name,
				deployment.Status.ObservedGeneration, deployment.Generation))
		}

		if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != desiredReplicas {
			return retry.SevereError(fmt.Errorf("waiting for deployment %q to scale failed. spec.replicas does not match the desired replicas", key.Name))
		}

		if deployment.Status.Replicas == desiredReplicas && deployment.Status.AvailableReplicas == desiredReplicas {
			return retry.Ok()
		}

		return retry.MinorError(fmt.Errorf("deployment %q currently has '%d' replicas. Desired: %d", key.Name, deployment.Status.AvailableReplicas, desiredReplicas))
	})
}
