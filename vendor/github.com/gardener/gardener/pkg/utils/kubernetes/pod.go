// Copyright 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
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

package kubernetes

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	appsv1beta2 "k8s.io/api/apps/v1beta2"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/gardener/pkg/utils"
)

// VisitPodSpec calls the given visitor for the PodSpec contained in the given object. The visitor may mutate the
// PodSpec.
func VisitPodSpec(obj runtime.Object, visit func(*corev1.PodSpec)) error {
	switch o := obj.(type) {
	case *corev1.Pod:
		visit(&o.Spec)

	case *appsv1.Deployment:
		visit(&o.Spec.Template.Spec)

	case *appsv1beta2.Deployment:
		visit(&o.Spec.Template.Spec)

	case *appsv1beta1.Deployment:
		visit(&o.Spec.Template.Spec)

	case *appsv1.StatefulSet:
		visit(&o.Spec.Template.Spec)

	case *appsv1beta2.StatefulSet:
		visit(&o.Spec.Template.Spec)

	case *appsv1beta1.StatefulSet:
		visit(&o.Spec.Template.Spec)

	case *appsv1.DaemonSet:
		visit(&o.Spec.Template.Spec)

	case *appsv1beta2.DaemonSet:
		visit(&o.Spec.Template.Spec)

	case *batchv1.Job:
		visit(&o.Spec.Template.Spec)

	case *batchv1.CronJob:
		visit(&o.Spec.JobTemplate.Spec.Template.Spec)

	case *batchv1beta1.CronJob:
		visit(&o.Spec.JobTemplate.Spec.Template.Spec)

	default:
		return fmt.Errorf("unhandled object type %T", obj)
	}

	return nil
}

// VisitContainers calls the given visitor for all (init) containers in the given PodSpec. If containerNames are given
// it only visits (init) containers with matching names. The visitor may mutate the Container.
func VisitContainers(podSpec *corev1.PodSpec, visit func(*corev1.Container), containerNames ...string) {
	for i, c := range podSpec.InitContainers {
		container := c
		if len(containerNames) == 0 || utils.ValueExists(container.Name, containerNames) {
			visit(&container)
			podSpec.InitContainers[i] = container
		}
	}

	for i, c := range podSpec.Containers {
		container := c
		if len(containerNames) == 0 || utils.ValueExists(container.Name, containerNames) {
			visit(&container)
			podSpec.Containers[i] = container
		}
	}
}

// AddVolume adds the given Volume to the given PodSpec if not present. If a Volume with the given name is already
// present it optionally overwrites the Volume according to the overwrite parameter.
func AddVolume(podSpec *corev1.PodSpec, volume corev1.Volume, overwrite bool) {
	for i, v := range podSpec.Volumes {
		if v.Name == volume.Name {
			// volume with given name is already present
			if overwrite {
				podSpec.Volumes[i] = volume
			}
			return
		}
	}

	// volume with given name is not present, add it
	podSpec.Volumes = append(podSpec.Volumes, volume)
}

// AddVolumeMount adds the given VolumeMount to the given Container if not present. If a VolumeMount with the given name
// is already present it optionally overwrites the VolumeMount according to the overwrite parameter.
func AddVolumeMount(container *corev1.Container, volumeMount corev1.VolumeMount, overwrite bool) {
	for i, v := range container.VolumeMounts {
		if v.Name == volumeMount.Name {
			// volumeMount with given name is already present
			if overwrite {
				container.VolumeMounts[i] = volumeMount
			}
			return
		}
	}

	// volumeMount with given name is not present, add it
	container.VolumeMounts = append(container.VolumeMounts, volumeMount)
}

// AddEnvVar adds the given EnvVar to the given Container if not present. If a EnvVar with the given name
// is already present it optionally overwrites the EnvVar according to the overwrite parameter.
func AddEnvVar(container *corev1.Container, envVar corev1.EnvVar, overwrite bool) {
	for i, e := range container.Env {
		if e.Name == envVar.Name {
			// envVar with given name is already present
			if overwrite {
				container.Env[i] = envVar
			}
			return
		}
	}

	// envVar with given name is not present, add it
	container.Env = append(container.Env, envVar)
}

// HasEnvVar checks if the given container has an EnvVar with the given name.
func HasEnvVar(container corev1.Container, name string) bool {
	envVars := sets.New[string]()

	for _, e := range container.Env {
		envVars.Insert(e.Name)
	}

	return envVars.Has(name)
}
