// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsStatefulSetReady reports whether the StatefulSet is ready and up-to-date.
//
// For RollingUpdate the fast path uses status field comparisons. For OnDelete
// it lists pods and compares controller-revision-hash against updateRevision,
// because the K8s StatefulSet controller does not promote status.currentRevision
// under OnDelete on K8s < v1.37 (kubernetes/kubernetes#73492, #106055, fix in
// #136833). The pod-level path stays valid post-v1.37; do not gate it on the
// Kubernetes version.
func IsStatefulSetReady(ctx context.Context, cl client.Client, etcdReplicas int32, statefulSet *appsv1.StatefulSet) (bool, string) {
	if statefulSet.Status.ObservedGeneration < statefulSet.Generation {
		return false, fmt.Sprintf("observed generation %d is outdated in comparison to generation %d", statefulSet.Status.ObservedGeneration, statefulSet.Generation)
	}
	if statefulSet.Status.ReadyReplicas < etcdReplicas {
		return false, fmt.Sprintf("not enough ready replicas (%d/%d)", statefulSet.Status.ReadyReplicas, etcdReplicas)
	}
	if statefulSet.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		allUpdated, unupdatedName, err := AreAllStsPodsAtUpdateRevision(ctx, cl, statefulSet)
		if err != nil {
			return false, fmt.Sprintf("unable to determine pod-level revision state: %s", err.Error())
		}
		if !allUpdated {
			return false, fmt.Sprintf("pod %s is not at the target StatefulSet revision %s", unupdatedName, statefulSet.Status.UpdateRevision)
		}
		return true, ""
	}
	if statefulSet.Status.CurrentRevision != statefulSet.Status.UpdateRevision {
		return false, fmt.Sprintf("Current StatefulSet revision %s is older than the updated StatefulSet revision %s)", statefulSet.Status.CurrentRevision, statefulSet.Status.UpdateRevision)
	}
	if statefulSet.Status.CurrentReplicas != statefulSet.Status.UpdatedReplicas {
		return false, fmt.Sprintf("StatefulSet status.CurrentReplicas (%d) != status.UpdatedReplicas (%d)", statefulSet.Status.CurrentReplicas, statefulSet.Status.UpdatedReplicas)
	}
	return true, ""
}

// AreAllStsPodsAtUpdateRevision reports whether every non-terminating pod of sts
// carries sts.Status.UpdateRevision on its controller-revision-hash label. On
// mismatch it returns the name of the first offending pod, useful for reason
// strings. Terminating pods are excluded so a pod recreation does not flap the
// answer.
func AreAllStsPodsAtUpdateRevision(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet) (bool, string, error) {
	updateRevision := sts.Status.UpdateRevision
	if updateRevision == "" {
		return false, "", nil
	}
	if sts.Spec.Selector == nil {
		return false, "", fmt.Errorf("statefulSet %s/%s has nil spec.selector", sts.Namespace, sts.Name)
	}
	sel, err := metav1.LabelSelectorAsSelector(sts.Spec.Selector)
	if err != nil {
		return false, "", fmt.Errorf("failed to parse statefulSet %s/%s spec.selector: %w", sts.Namespace, sts.Name, err)
	}
	list := &corev1.PodList{}
	if err := cl.List(ctx, list, &client.ListOptions{Namespace: sts.Namespace, LabelSelector: sel}); err != nil {
		return false, "", fmt.Errorf("failed to list pods for statefulSet %s/%s: %w", sts.Namespace, sts.Name, err)
	}
	for i := range list.Items {
		pod := &list.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Labels[appsv1.StatefulSetRevisionLabel] != updateRevision {
			return false, pod.Name, nil
		}
	}
	return true, "", nil
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
	etcdTLSVolumeMountNames = sets.New(common.VolumeNameEtcdCA,
		common.VolumeNameEtcdServerTLS,
		common.VolumeNameEtcdClientTLS,
		common.VolumeNameEtcdPeerCA,
		common.VolumeNameEtcdPeerServerTLS,
		common.VolumeNameBackupRestoreCA)
	possiblePeerTLSVolumeMountNames = sets.New(common.VolumeNameEtcdPeerCA,
		common.OldVolumeNameEtcdPeerCA,
		common.VolumeNameEtcdPeerServerTLS,
		common.OldVolumeNameEtcdPeerServerTLS)
	etcdbrTLSVolumeMountNames = sets.New(common.VolumeNameBackupRestoreServerTLS,
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
	knownTLSVolMountNames := utils.IfConditionOr(containerName == common.ContainerNameEtcd, etcdTLSVolumeMountNames, etcdbrTLSVolumeMountNames)
	for _, volMount := range allVolumeMounts {
		if knownTLSVolMountNames.Has(volMount.Name) {
			filteredVolMounts = append(filteredVolMounts, volMount)
		}
	}
	return filteredVolMounts
}

// GetSecretNameFromVolume looks up a Secret-typed volume by name in the StatefulSet's pod template.
// It returns the volume's SecretName and true if a volume with the given name exists and has a
// Secret volume source; otherwise it returns "" and false (when sts is nil, no volume matches the
// name, or the matching volume's source is not a Secret).
func GetSecretNameFromVolume(sts *appsv1.StatefulSet, volumeName string) (string, bool) {
	if sts == nil {
		return "", false
	}
	for _, vol := range sts.Spec.Template.Spec.Volumes {
		if vol.Name == volumeName && vol.Secret != nil {
			return vol.Secret.SecretName, true
		}
	}
	return "", false
}
