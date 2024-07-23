// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"fmt"
	"strconv"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsStatefulSetCorrectlyReconciled(ctx context.Context, c client.Client, instance *druidv1alpha1.Etcd, ss *appsv1.StatefulSet) (bool, error) {
	if err := c.Get(ctx, client.ObjectKeyFromObject(instance), ss); err != nil {
		return false, err
	}
	if metav1.IsControlledBy(ss, instance) {
		return true, nil
	}
	return false, nil
}

// SetStatefulSetReady updates the status sub-resource of the passed in StatefulSet with ObservedGeneration, Replicas and ReadyReplicas
// ensuring that the StatefulSet ready check will succeed.
func SetStatefulSetReady(s *appsv1.StatefulSet) {
	s.Status.ObservedGeneration = s.Generation

	replicas := int32(1)
	if s.Spec.Replicas != nil {
		replicas = *s.Spec.Replicas
	}
	s.Status.Replicas = replicas
	s.Status.ReadyReplicas = replicas
}

// SetStatefulSetPodsUpdated updates the status sub-resource of the passed in StatefulSet with UpdatedReplicas, UpdateRevision and CurrentRevision
func SetStatefulSetPodsUpdated(s *appsv1.StatefulSet) {
	replicas := int32(1)
	if s.Spec.Replicas != nil {
		replicas = *s.Spec.Replicas
	}
	s.Status.UpdatedReplicas = replicas
	s.Status.CurrentReplicas = replicas

	s.Status.UpdateRevision = "123456"
	s.Status.CurrentRevision = "123456"
}

// CreateStatefulSet creates a statefulset with its owner reference set to etcd.
func CreateStatefulSet(name, namespace string, etcdUID types.UID, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":      "etcd-statefulset",
				"instance": name,
				"name":     "etcd",
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
					"app":      "etcd-statefulset",
					"instance": name,
					"name":     "etcd",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":      "etcd-statefulset",
						"instance": name,
						"name":     "etcd",
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

func CreateStsPods(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet) {
	stsReplicas := *sts.Spec.Replicas
	for i := 0; i < int(stsReplicas); i++ {
		podName := fmt.Sprintf("%s-%d", sts.Name, i)

		podLabels := mergeStringMaps(sts.Spec.Template.Labels, map[string]string{
			"apps.kubernetes.io/pod-index":       strconv.Itoa(i),
			"statefulset.kubernetes.io/pod-name": podName,
			"controller-revision-hash":           sts.Status.UpdateRevision,
		})

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: sts.Namespace,
				Labels:    podLabels,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         appsv1.SchemeGroupVersion.Version,
						Kind:               "StatefulSet",
						BlockOwnerDeletion: pointer.Bool(true),
						Name:               sts.Name,
						UID:                sts.UID,
					},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "etcd",
						Image: "etcd-wrapper:latest",
					},
				},
			},
		}

		ExpectWithOffset(1, cl.Create(ctx, pod)).To(Succeed())
		// Update pod status and set ready condition to true
		pod.Status.Conditions = []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		}
		ExpectWithOffset(1, cl.Status().Update(ctx, pod)).To(Succeed())
	}
}

func DeleteStsPods(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet) {
	stsReplicas := *sts.Spec.Replicas
	for i := 0; i < int(stsReplicas); i++ {
		podName := fmt.Sprintf("%s-%d", sts.Name, i)
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: sts.Namespace,
			},
		}
		ExpectWithOffset(1, cl.Delete(ctx, pod)).To(Succeed())
	}
}
