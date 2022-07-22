// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package statefulset

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type component struct {
	client    client.Client
	namespace string

	values Values
}

func (c *component) Deploy(ctx context.Context) error {
	var (
		sts    = c.emptyStatefulset(c.values.StsName)
		create = false
	)

	if err := c.client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("cound not fetch statefulset before deploying a statefulset: %v", err)
		}
		create = true
	}

	if sts.Generation > 1 && sts.Spec.ServiceName != c.values.ServiceName {
		// Earlier clusters referred to the client service in `sts.Spec.ServiceName` which must be changed
		// when a multi-node cluster is used, see https://github.com/gardener/etcd-druid/pull/293.
		if clusterScaledUpToMultiNode(c.values) {
			if err := c.client.Delete(ctx, sts); client.IgnoreNotFound(err) != nil {
				return err
			}
			create = true
		}
	}

	return c.syncStatefulset(ctx, sts, create)
}

func (c *component) Destroy(ctx context.Context) error {
	sts := c.emptyStatefulset(c.values.StsName)

	if err := c.deleteStatefulset(ctx, sts); err != nil {
		return err
	}
	return nil
}

func clusterScaledUpToMultiNode(val Values) bool {
	return val.Replicas != 1 &&
		// Also consider `0` here because this field was not maintained in earlier releases.
		(val.StatusReplicas == 0 ||
			val.StatusReplicas == 1)
}

func (c *component) syncStatefulset(ctx context.Context, sts *appsv1.StatefulSet, create bool) error {
	patch := client.StrategicMergeFrom(sts.DeepCopy())

	sts.ObjectMeta = getObjectMeta(&c.values)
	sts.Spec = appsv1.StatefulSetSpec{
		VolumeClaimTemplates: []v1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: c.values.VolumeClaimTemplateName,
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					StorageClassName: c.values.StorageClass,
					Resources:        getStorageReq(c.values),
				},
			},
		},
		PodManagementPolicy: appsv1.ParallelPodManagement,
		UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		},
		Replicas:    pointer.Int32(c.values.Replicas),
		ServiceName: c.values.ServiceName,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"name":     "etcd",
				"instance": c.values.EtcdName,
			},
		},
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: c.values.Annotations,
				Labels:      c.values.Labels,
			},
			Spec: v1.PodSpec{
				HostAliases: []v1.HostAlias{
					{
						IP:        "127.0.0.1",
						Hostnames: []string{c.values.EtcdName + "-local"},
					},
				},
				ServiceAccountName:        c.values.ServiceAccountName,
				Affinity:                  c.values.Affinity,
				TopologySpreadConstraints: c.values.TopologySpreadConstraints,
				Containers: []v1.Container{
					{
						Name:            "etcd",
						Image:           c.values.EtcdImage,
						ImagePullPolicy: v1.PullIfNotPresent,
						Command:         c.values.EtcdCommand,
						ReadinessProbe: &v1.Probe{
							Handler: v1.Handler{
								Exec: &v1.ExecAction{
									Command: c.values.ReadinessProbeCommand,
								},
							},
							InitialDelaySeconds: 15,
							PeriodSeconds:       5,
							FailureThreshold:    5,
						},
						LivenessProbe: &v1.Probe{
							Handler: v1.Handler{
								Exec: &v1.ExecAction{
									Command: c.values.LivenessProbCommand,
								},
							},
							InitialDelaySeconds: 15,
							PeriodSeconds:       5,
							FailureThreshold:    5,
						},
						Ports:        getEtcdPorts(c.values),
						Resources:    getEtcdResources(c.values),
						Env:          getEtcdEnvVar(c.values),
						VolumeMounts: getEtcdVolumeMounts(c.values),
					},
					{
						Name:            "backup-restore",
						Image:           c.values.BackupImage,
						ImagePullPolicy: v1.PullIfNotPresent,
						Command:         c.values.EtcdBackupCommand,
						Ports:           getBackupPorts(c.values),
						Resources:       getBackupResources(c.values),
						Env:             getStsEnvVar(c.values),
						VolumeMounts:    getBackupRestoreVolumeMounts(c.values),
						SecurityContext: &v1.SecurityContext{
							Capabilities: &v1.Capabilities{
								Add: []v1.Capability{
									v1.Capability("SYS_PTRACE"),
								},
							},
						},
					},
				},
				ShareProcessNamespace: pointer.Bool(true),
				Volumes:               getBackupRestoreVolumes(c.values),
			},
		},
	}
	if c.values.PriorityClassName != nil {
		sts.Spec.Template.Spec.PriorityClassName = *c.values.PriorityClassName
	}

	if create {
		return c.client.Create(ctx, sts)
	}

	return c.client.Patch(ctx, sts, patch)
}

func (c *component) deleteStatefulset(ctx context.Context, sts *appsv1.StatefulSet) error {
	return client.IgnoreNotFound(c.client.Delete(ctx, sts))
}

// New creates a new statefulset deployer instance.
func New(c client.Client, namespace string, values Values) gardenercomponent.Deployer {
	return &component{
		client:    c,
		namespace: namespace,
		values:    values,
	}
}

func (c *component) emptyStatefulset(name string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.namespace,
		},
	}
}

func getObjectMeta(val *Values) metav1.ObjectMeta {
	labels := map[string]string{"name": "etcd", "instance": val.EtcdName}
	for key, value := range val.Labels {
		labels[key] = value
	}

	annotations := map[string]string{
		"gardener.cloud/owned-by":   fmt.Sprintf("%s/%s", val.EtcdNameSpace, val.EtcdName),
		"gardener.cloud/owner-type": "etcd",
	}

	if val.Annotations != nil {
		for key, value := range val.Annotations {
			annotations[key] = value
		}
	}

	ownerRefs := []metav1.OwnerReference{
		{
			APIVersion:         druidv1alpha1.GroupVersion.String(),
			Kind:               "Etcd",
			Name:               val.EtcdName,
			UID:                val.EtcdUID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	}

	return metav1.ObjectMeta{
		Name:            val.StsName,
		Namespace:       val.EtcdNameSpace,
		Labels:          labels,
		Annotations:     annotations,
		OwnerReferences: ownerRefs,
	}
}
