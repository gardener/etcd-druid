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

	"github.com/gardener/gardener/pkg/controllerutils"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type component struct {
	client    client.Client
	namespace string

	values Values
}

func (c *component) Deploy(ctx context.Context) error {
	fetchedSts := &appsv1.StatefulSet{}
	err := c.client.Get(ctx, types.NamespacedName{Name: c.values.EtcdName, Namespace: c.values.EtcdNameSpace}, fetchedSts)
	if apierrors.IsNotFound(err) {
		sts := c.emptyStatefulset(c.values.StsName)

		return c.syncStatefulset(ctx, sts)

	}

	if err != nil {
		return fmt.Errorf("cound not fetch statefulset before deploying a statefulset: %v", err)
	}

	if fetchedSts.Spec.ServiceName != c.values.ServiceName {
		if clusterScaledUpToMultiNode(c.values) {
			return c.createStatefulset(ctx, fetchedSts)
		}
	}

	return c.syncStatefulset(ctx, fetchedSts)
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

func (c *component) createStatefulset(ctx context.Context, ss *appsv1.StatefulSet) error {
	skipDelete := false
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if !skipDelete {
			if err := c.client.Delete(ctx, ss); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		skipDelete = true
		sts := c.emptyStatefulset(c.values.StsName)

		return c.syncStatefulset(ctx, sts)
	})
	return err
}

func (c *component) syncStatefulset(ctx context.Context, sts *appsv1.StatefulSet) error {
	_, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, c.client, sts, func() error {
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
		return nil
	})
	return err
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
