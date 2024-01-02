// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsStatefulSetReady checks whether the given StatefulSet is ready and up-to-date.
// A StatefulSet is considered healthy if its controller observed its current revision,
// it is not in an update (i.e. UpdateRevision is empty) and if its current replicas are equal to
// desired replicas specified in ETCD specs.
// It returns ready status (bool) and in case it is not ready then the second return value holds the reason.
func IsStatefulSetReady(etcdReplicas int32, statefulSet *appsv1.StatefulSet) (bool, string) {
	if statefulSet.Status.ObservedGeneration < statefulSet.Generation {
		return false, fmt.Sprintf("observed generation %d is outdated in comparison to generation %d", statefulSet.Status.ObservedGeneration, statefulSet.Generation)
	}
	if statefulSet.Status.ReadyReplicas < etcdReplicas {
		return false, fmt.Sprintf("not enough ready replicas (%d/%d)", statefulSet.Status.ReadyReplicas, etcdReplicas)
	}
	if statefulSet.Status.CurrentRevision != statefulSet.Status.UpdateRevision {
		return false, fmt.Sprintf("Current StatefulSet revision %s is older than the updated StatefulSet revision %s)", statefulSet.Status.CurrentRevision, statefulSet.Status.UpdateRevision)
	}
	if statefulSet.Status.CurrentReplicas != statefulSet.Status.UpdatedReplicas {
		return false, fmt.Sprintf("StatefulSet status.CurrentReplicas (%d) != status.UpdatedReplicas (%d)", statefulSet.Status.CurrentReplicas, statefulSet.Status.UpdatedReplicas)
	}
	return true, ""
}

// GetStatefulSet fetches StatefulSet created for the etcd.
func GetStatefulSet(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, error) {
	sts := &appsv1.StatefulSet{}

	if err := cl.Get(ctx, client.ObjectKey{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}, sts); err != nil {

		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return sts, nil
}
