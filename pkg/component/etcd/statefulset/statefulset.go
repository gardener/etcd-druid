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
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/retry"
	gardenerretry "github.com/gardener/gardener/pkg/utils/retry"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/etcd-druid/pkg/utils"
)

// Interface contains functions for a StatefulSet deployer.
type Interface interface {
	gardenercomponent.DeployWaiter
	// Get gets the etcd StatefulSet.
	Get(context.Context) (*appsv1.StatefulSet, error)
}

type component struct {
	client client.Client
	logger logr.Logger

	namespace string

	values Values
}

func (c *component) Get(ctx context.Context) (*appsv1.StatefulSet, error) {
	sts := c.emptyStatefulset(c.values.StsName)

	if err := c.client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
		return nil, err
	}

	return sts, nil
}

func (c *component) Deploy(ctx context.Context) error {
	sts, err := c.Get(ctx)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		sts = c.emptyStatefulset(c.values.StsName)
	}

	if sts.Generation > 1 && sts.Spec.ServiceName != c.values.ServiceName {
		// Earlier clusters referred to the client service in `sts.Spec.ServiceName` which must be changed
		// when a multi-node cluster is used, see https://github.com/gardener/etcd-druid/pull/293.
		if clusterScaledUpToMultiNode(c.values) {
			deleteAndWait := gardenercomponent.OpDestroyAndWait(c)
			if err := deleteAndWait.Destroy(ctx); err != nil {
				return err
			}
			sts = c.emptyStatefulset(c.values.StsName)
		}
	}

	return c.syncStatefulset(ctx, sts, sts.Generation == 0)
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

const (
	// DefaultInterval is the default interval for retry operations.
	DefaultInterval = 5 * time.Second
	// DefaultTimeout is the default timeout for retry operations.
	DefaultTimeout = 1 * time.Minute
)

func (c *component) Wait(ctx context.Context) error {
	sts := c.emptyStatefulset(c.values.StsName)

	err := gardenerretry.UntilTimeout(ctx, DefaultInterval, DefaultTimeout, func(ctx context.Context) (bool, error) {
		if err := c.client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
			if apierrors.IsNotFound(err) {
				return gardenerretry.MinorError(err)
			}
			return gardenerretry.SevereError(err)
		}
		if err := utils.CheckStatefulSet(c.values.Replicas, sts); err != nil {
			return gardenerretry.MinorError(err)
		}
		return gardenerretry.Ok()
	})
	if err != nil {
		messages, err2 := c.fetchPVCEventsFor(ctx, sts)
		if err2 != nil {
			c.logger.Error(err2, "Error while fetching events for depending PVC")
			// don't expose this error since fetching events is a best effort
			// and shouldn't be confused with the actual error
			return err
		}
		if messages != "" {
			return fmt.Errorf("%w\n\n%s", err, messages)
		}
	}

	return err
}

func (c *component) WaitCleanup(ctx context.Context) error {
	return gardenerretry.UntilTimeout(ctx, DefaultInterval, DefaultTimeout, func(ctx context.Context) (done bool, err error) {
		sts := c.emptyStatefulset(c.values.StsName)
		err = c.client.Get(ctx, client.ObjectKeyFromObject(sts), sts)
		switch {
		case apierrors.IsNotFound(err):
			return retry.Ok()
		case err == nil:
			return retry.MinorError(err)
		default:
			return retry.SevereError(err)
		}
	})
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
			MatchLabels: getCommonLabels(&c.values),
		},
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: c.values.Annotations,
				Labels:      sts.GetLabels(),
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

func (c *component) fetchPVCEventsFor(ctx context.Context, ss *appsv1.StatefulSet) (string, error) {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := c.client.List(ctx, pvcs, client.InNamespace(ss.GetNamespace())); err != nil {
		return "", err
	}

	var (
		pvcMessages  string
		volumeClaims = ss.Spec.VolumeClaimTemplates
	)
	for _, volumeClaim := range volumeClaims {
		for _, pvc := range pvcs.Items {
			if !strings.HasPrefix(pvc.GetName(), fmt.Sprintf("%s-%s", volumeClaim.Name, ss.Name)) || pvc.Status.Phase == corev1.ClaimBound {
				continue
			}
			messages, err := kutil.FetchEventMessages(ctx, c.client.Scheme(), c.client, &pvc, corev1.EventTypeWarning, 2)
			if err != nil {
				return "", err
			}
			if messages != "" {
				pvcMessages += fmt.Sprintf("Warning for PVC %s:\n%s\n", pvc.Name, messages)
			}
		}
	}
	return pvcMessages, nil
}

// New creates a new statefulset deployer instance.
func New(c client.Client, logger logr.Logger, namespace string, values Values) Interface {
	return &component{
		client:    c,
		logger:    logger,
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

func getCommonLabels(val *Values) map[string]string {
	return map[string]string{
		"name":     "etcd",
		"instance": val.EtcdName,
	}
}

func getObjectMeta(val *Values) metav1.ObjectMeta {
	labels := utils.MergeStringMaps(getCommonLabels(val), val.Labels)

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
