// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	statefulSets := &appsv1.StatefulSetList{}
	if err := cl.List(ctx, statefulSets, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: labels.Set(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)).AsSelector()}); err != nil {
		return nil, err
	}

	for _, sts := range statefulSets.Items {
		if metav1.IsControlledBy(&sts, etcd) {
			return &sts, nil
		}
	}

	return nil, nil
}

// FetchPVCWarningMessagesForStatefulSet fetches warning messages for PVCs for a statefulset, if found concatenates the first 2 warning messages and returns
// them as string warning message. In case it fails to fetch events, it collects the errors and returns the combined error.
func FetchPVCWarningMessagesForStatefulSet(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet) (string, error) {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := cl.List(ctx, pvcs, client.InNamespace(sts.GetNamespace())); err != nil {
		return "", fmt.Errorf("unable to list PVCs for sts %s: %w", sts.Name, err)
	}

	var (
		events []string
		pvcErr error
	)

	for _, volumeClaim := range sts.Spec.VolumeClaimTemplates {
		pvcPrefix := fmt.Sprintf("%s-%s", volumeClaim.Name, sts.Name)
		for _, pvc := range pvcs.Items {
			if !strings.HasPrefix(pvc.GetName(), pvcPrefix) || pvc.Status.Phase == corev1.ClaimBound {
				continue
			}
			// TODO (shreyas-s-rao): switch to kutil.FetchEventMessages() once g/g is upgraded to v1.90.0+
			messages, err := fetchEventMessages(ctx, cl.Scheme(), cl, &pvc, corev1.EventTypeWarning, 2)
			if err != nil {
				pvcErr = errors.Join(pvcErr, fmt.Errorf("unable to fetch warning events for PVC %s/%s: %w", pvc.Namespace, pvc.Name, err))
			}
			if messages != "" {
				events = append(events, fmt.Sprintf("Warning for PVC %s/%s: %s", pvc.Namespace, pvc.Name, messages))
			}
		}
	}
	return strings.TrimSpace(strings.Join(events, "; ")), pvcErr
}
