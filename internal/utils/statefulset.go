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
	"github.com/gardener/etcd-druid/internal/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
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

// GetStatefulSet fetches StatefulSet created for the etcd. Nil will be returned if one of these conditions are met:
// - StatefulSet is not found
// - StatefulSet is not controlled by the etcd
func GetStatefulSet(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, error) {
	sts := &appsv1.StatefulSet{}
	if err := cl.Get(ctx, client.ObjectKey{Name: druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), Namespace: etcd.Namespace}, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if metav1.IsControlledBy(sts, etcd) {
		return sts, nil
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

var (
	etcdTLSVolumeMountNames = sets.New[string](common.VolumeNameEtcdCA,
		common.VolumeNameEtcdServerTLS,
		common.VolumeNameEtcdClientTLS,
		common.VolumeNameEtcdPeerCA,
		common.VolumeNameEtcdPeerServerTLS,
		common.VolumeNameBackupRestoreCA)
	possiblePeerTLSVolumeMountNames = sets.New[string](common.VolumeNameEtcdPeerCA,
		common.OldVolumeNameEtcdPeerCA,
		common.VolumeNameEtcdPeerServerTLS,
		common.OldVolumeNameEtcdPeerServerTLS)
	etcdbrTLSVolumeMountNames = sets.New[string](common.VolumeNameBackupRestoreServerTLS,
		common.VolumeNameEtcdCA,
		common.VolumeNameEtcdClientTLS)
)

// GetEtcdContainerPeerTLSVolumeMounts returns the volume mounts for the etcd container that are related to peer TLS.
// It will look at both older names (present in version <= v0.22) and new names (present in version >= v0.23) to create the slice.
func GetEtcdContainerPeerTLSVolumeMounts(sts *appsv1.StatefulSet) []corev1.VolumeMount {
	volumeMounts := make([]corev1.VolumeMount, 0, 2)
	if sts == nil {
		return volumeMounts
	}
	for _, container := range sts.Spec.Template.Spec.Containers {
		if container.Name == common.ContainerNameEtcd {
			for _, volMount := range container.VolumeMounts {
				if possiblePeerTLSVolumeMountNames.Has(volMount.Name) {
					volumeMounts = append(volumeMounts, volMount)
				}
			}
			break
		}
	}
	return volumeMounts
}

// GetStatefulSetContainerTLSVolumeMounts returns a map of container name to TLS volume mounts for the given StatefulSet.
func GetStatefulSetContainerTLSVolumeMounts(sts *appsv1.StatefulSet) map[string][]corev1.VolumeMount {
	containerVolMounts := make(map[string][]corev1.VolumeMount, 2) // each pod is a 2 container pod. Init containers are not counted as containers.
	if sts == nil {
		return containerVolMounts
	}
	for _, container := range sts.Spec.Template.Spec.Containers {
		if _, ok := containerVolMounts[container.Name]; !ok {
			// Assuming 6 volume mounts per container. If there are more in future then this map's capacity will be increased by the golang runtime.
			// A size is assumed to minimize the possibility of a resize, which is usually not cheap.
			containerVolMounts[container.Name] = make([]corev1.VolumeMount, 0, 6)
		}
		containerVolMounts[container.Name] = append(containerVolMounts[container.Name], filterTLSVolumeMounts(container.Name, container.VolumeMounts)...)
	}
	return containerVolMounts
}

func filterTLSVolumeMounts(containerName string, allVolumeMounts []corev1.VolumeMount) []corev1.VolumeMount {
	filteredVolMounts := make([]corev1.VolumeMount, 0, len(allVolumeMounts))
	knownTLSVolMountNames := IfConditionOr(containerName == common.ContainerNameEtcd, etcdTLSVolumeMountNames, etcdbrTLSVolumeMountNames)
	for _, volMount := range allVolumeMounts {
		if knownTLSVolMountNames.Has(volMount.Name) {
			filteredVolMounts = append(filteredVolMounts, volMount)
		}
	}
	return filteredVolMounts
}
