// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

// CreateStatefulSet creates a statefulset with its owner reference set to etcd.
func CreateStatefulSet(name, namespace string, etcdUID types.UID, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
				druidv1alpha1.LabelPartOfKey:    name,
				druidv1alpha1.LabelComponentKey: common.ComponentNameStatefulSet,
				druidv1alpha1.LabelAppNameKey:   name,
			},
			Annotations: nil,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         druidv1alpha1.GroupVersion.String(),
				Kind:               "Etcd",
				Name:               name,
				UID:                etcdUID,
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			}},
			Finalizers:    nil,
			ManagedFields: nil,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: pointer.Int32(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
					druidv1alpha1.LabelPartOfKey:    name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
						druidv1alpha1.LabelPartOfKey:    name,
						druidv1alpha1.LabelComponentKey: common.ComponentNameStatefulSet,
						druidv1alpha1.LabelAppNameKey:   name,
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Selector:    nil,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("25Gi")},
						},
						StorageClassName: pointer.String("gardener.cloud-fast"),
					},
				},
			},
			ServiceName:         fmt.Sprintf("%s-peer", name),
			PodManagementPolicy: appsv1.ParallelPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
		},
	}
}
