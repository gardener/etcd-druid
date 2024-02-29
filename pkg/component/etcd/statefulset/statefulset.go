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
	"strconv"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/pkg/utils"

	gardenercomponent "github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/flow"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	gardenerretry "github.com/gardener/gardener/pkg/utils/retry"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interface contains functions for a StatefulSet deployer.
type Interface interface {
	gardenercomponent.DeployWaiter
	// Get gets the etcd StatefulSet.
	Get(context.Context) (*appsv1.StatefulSet, error)
}

type component struct {
	client       client.Client
	logger       logr.Logger
	values       Values
	featureGates map[featuregate.Feature]bool
}

// New creates a new StatefulSet deployer instance.
func New(c client.Client, logger logr.Logger, values Values, featureGates map[featuregate.Feature]bool) Interface {
	objectLogger := logger.WithValues("sts", client.ObjectKey{Name: values.Name, Namespace: values.Namespace})

	return &component{
		client:       c,
		logger:       objectLogger,
		values:       values,
		featureGates: featureGates,
	}
}

// Destroy deletes the StatefulSet
func (c *component) Destroy(ctx context.Context) error {
	sts := c.emptyStatefulset()
	return client.IgnoreNotFound(c.client.Delete(ctx, sts))
}

// Deploy executes a deploy-flow to ensure that the StatefulSet is synchronized correctly
func (c *component) Deploy(ctx context.Context) error {
	deployFlow, err := c.createDeployFlow(ctx)
	if err != nil {
		return err
	}
	return deployFlow.Run(ctx, flow.Opts{})
}

// Get retrieves the existing StatefulSet
func (c *component) Get(ctx context.Context) (*appsv1.StatefulSet, error) {
	sts := c.emptyStatefulset()
	if err := c.client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
		return nil, err
	}
	return sts, nil
}

const (
	// defaultInterval is the default interval for retry operations.
	defaultInterval = 5 * time.Second
	// defaultTimeout is the default timeout for retry operations.
	defaultTimeout = 90 * time.Second

	// ScaleToMultiNodeAnnotationKey is used to represent scale-up annotation.
	ScaleToMultiNodeAnnotationKey = "gardener.cloud/scaled-to-multi-node"
)

// Wait waits for the deployment of the StatefulSet to finish
func (c *component) Wait(ctx context.Context) error {
	sts := c.emptyStatefulset()
	err := c.waitDeploy(ctx, sts, c.values.Replicas, defaultTimeout)
	if err != nil {
		if getErr := c.client.Get(ctx, client.ObjectKeyFromObject(sts), sts); getErr != nil {
			return err
		}
		messages, errorPVC, err2 := c.fetchPVCEventsForStatefulset(ctx, sts)
		if err2 != nil {
			c.logger.Error(err2, "Error while fetching events for depending PVCs for statefulset",
				"namespace", sts.Namespace, "statefulsetName", sts.Name, "pvcName", *errorPVC)
			// don't expose this error since fetching events is the best effort
			// and shouldn't be confused with the actual error
			return err
		}
		if messages != "" {
			return fmt.Errorf("%w\n\n%s", err, messages)
		}
	}

	return err
}

func (c *component) getLatestPodCreationTime(ctx context.Context, sts *appsv1.StatefulSet) (time.Time, error) {
	pods := corev1.PodList{}
	if err := c.client.List(ctx, &pods, client.InNamespace(sts.Namespace), client.MatchingLabels(sts.Spec.Template.Labels)); err != nil {
		return time.Time{}, err
	}
	var recentCreationTime time.Time
	for _, pod := range pods.Items {
		if recentCreationTime.Before(pod.CreationTimestamp.Time) {
			recentCreationTime = pod.CreationTimestamp.Time
		}
	}
	return recentCreationTime, nil
}

func (c *component) waitUntilPodsReady(ctx context.Context, originalSts *appsv1.StatefulSet, podDeletionTime time.Time, interval time.Duration, timeout time.Duration) error {
	sts := appsv1.StatefulSet{}
	return gardenerretry.UntilTimeout(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		if err := c.client.Get(ctx, client.ObjectKeyFromObject(originalSts), &sts); err != nil {
			if apierrors.IsNotFound(err) {
				return gardenerretry.MinorError(err)
			}
			return gardenerretry.SevereError(err)
		}
		if sts.Status.ReadyReplicas < *sts.Spec.Replicas {
			return gardenerretry.MinorError(fmt.Errorf("only %d out of %d replicas are ready", sts.Status.ReadyReplicas, sts.Spec.Replicas))
		}
		recentPodCreationTime, err := c.getLatestPodCreationTime(ctx, &sts)
		if err != nil {
			return gardenerretry.MinorError(fmt.Errorf("failed to get most recent pod creation timestamp: %w", err))
		}
		if recentPodCreationTime.Before(podDeletionTime) {
			return gardenerretry.MinorError(fmt.Errorf("most recent pod creation time %v is still before the %v time when the pods were deleted", recentPodCreationTime, podDeletionTime))
		}
		return gardenerretry.Ok()
	})
}

func (c *component) waitDeploy(ctx context.Context, originalSts *appsv1.StatefulSet, replicas int32, timeout time.Duration) error {
	updatedSts := appsv1.StatefulSet{}
	return gardenerretry.UntilTimeout(ctx, defaultInterval, timeout, func(ctx context.Context) (bool, error) {
		if err := c.client.Get(ctx, client.ObjectKeyFromObject(originalSts), &updatedSts); err != nil {
			if apierrors.IsNotFound(err) {
				return gardenerretry.MinorError(err)
			}
			return gardenerretry.SevereError(err)
		}
		if updatedSts.Generation < originalSts.Generation {
			return gardenerretry.MinorError(fmt.Errorf("statefulset generation has not yet been updated in the cache"))
		}
		if ready, reason := utils.IsStatefulSetReady(replicas, &updatedSts); !ready {
			return gardenerretry.MinorError(fmt.Errorf(reason))
		}
		return gardenerretry.Ok()
	})
}

// WaitCleanup waits for the deletion of the StatefulSet to complete
func (c *component) WaitCleanup(ctx context.Context) error {
	return gardenerretry.UntilTimeout(ctx, defaultInterval, defaultTimeout, func(ctx context.Context) (done bool, err error) {
		sts := c.emptyStatefulset()
		err = c.client.Get(ctx, client.ObjectKeyFromObject(sts), sts)
		switch {
		case apierrors.IsNotFound(err):
			return gardenerretry.Ok()
		case err == nil:
			// StatefulSet is still available, so we should retry.
			return false, nil
		default:
			return gardenerretry.SevereError(err)
		}
	})
}

func (c *component) createDeployFlow(ctx context.Context) (*flow.Flow, error) {
	var (
		sts *appsv1.StatefulSet
		err error
	)
	sts, err = c.getExistingSts(ctx)
	if err != nil {
		return nil, err
	}

	flowName := fmt.Sprintf("(etcd: %s) Deploy Flow for StatefulSet %s for Namespace: %s", getOwnerReferenceNameWithUID(c.values.OwnerReference), c.values.Name, c.values.Namespace)
	g := flow.NewGraph(flowName)

	var taskID *flow.TaskID
	if sts != nil {
		taskID = c.addTasksForPeerUrlTLSChangedToEnabled(g, sts)
		if taskID == nil {
			// if sts recreation tasks for peer url tls have already been added then there is no need to additionally add tasks to explicitly handle immutable field updates.
			taskID = c.addImmutableFieldUpdateTask(g, sts)
		}
	}
	c.addCreateOrPatchTask(g, sts, taskID)

	return g.Compile(), nil
}

// addTasksForPeerUrlTLSChangedToEnabled adds tasks to the deployment flow in case the peer url tls has been changed to `enabled`.
// To ensure that the tls enablement of peer url is properly reflected in etcd, the existing etcd StatefulSet pods should be restarted twice. Assume
// that the current state of etcd is that peer url is not TLS enabled. First restart pushes a new configuration which contains
// PeerUrlTLS configuration. etcd-backup-restore will update the member peer url. This will result in the change of the peer url in the etcd db file,
// but it will not reflect in the already running etcd container. Ideally a restart of an etcd container would have been sufficient but currently k8s
// does not expose an API to force restart a single container within a pod. Therefore, we need to restart the StatefulSet pod(s) once again. When the pod(s) is
// restarted the second time it will now start etcd with the correct peer url which will be TLS enabled.
// To achieve 2 restarts following is done:
// 1. An update is made to the spec mounting the peer URL TLS secrets. This will cause a rolling update of the existing pod.
// 2. Once the update is successfully completed, then we delete StatefulSet pods, causing a restart by the StatefulSet controller.
// NOTE: The need to restart etcd pods twice will change in the future.
func (c *component) addTasksForPeerUrlTLSChangedToEnabled(g *flow.Graph, sts *appsv1.StatefulSet) *flow.TaskID {
	var existingStsReplicas int32
	if sts.Spec.Replicas != nil {
		existingStsReplicas = *sts.Spec.Replicas
	}

	if c.values.PeerTLSChangedToEnabled {
		updateStsOpName := "(update-sts-spec): update Peer TLS secret mount"
		updateTaskID := g.Add(flow.Task{
			Name: updateStsOpName,
			Fn: func(ctx context.Context) error {
				return c.updateAndWait(ctx, updateStsOpName, sts, existingStsReplicas)
			},
			Dependencies: nil,
		})
		c.logger.Info("adding task to deploy flow", "name", updateStsOpName, "ID", updateTaskID)

		waitForLeaseUpdateOpName := "(wait-lease-update): Wait for lease to be updated with peer TLS"
		waitLeaseUpdateID := g.Add(flow.Task{
			Name: waitForLeaseUpdateOpName,
			Fn: func(ctx context.Context) error {
				return c.waitUntilTLSEnabled(ctx, 3*time.Minute)
			},
			Dependencies: flow.NewTaskIDs(updateTaskID),
		})
		c.logger.Info("adding task to deploy flow", "name", updateStsOpName, "ID", waitLeaseUpdateID)

		deleteAllStsPodsOpName := "(delete-sts-pods): deleting all sts pods"
		deleteAllStsPodsTaskID := g.Add(flow.Task{
			Name: deleteAllStsPodsOpName,
			Fn: func(ctx context.Context) error {
				return c.deleteAllStsPods(ctx, deleteAllStsPodsOpName, sts)
			},
			Dependencies: flow.NewTaskIDs(waitLeaseUpdateID),
		})

		c.logger.Info("adding task to deploy flow", "name", deleteAllStsPodsOpName, "ID", deleteAllStsPodsTaskID)

		return &deleteAllStsPodsTaskID
	}
	return nil
}

func (c *component) addImmutableFieldUpdateTask(g *flow.Graph, sts *appsv1.StatefulSet) *flow.TaskID {
	if sts.Generation > 1 && immutableFieldUpdate(sts, c.values) {
		opName := "delete sts due to immutable field update"
		taskID := g.Add(flow.Task{
			Name:         opName,
			Fn:           func(ctx context.Context) error { return c.destroyAndWait(ctx, opName) },
			Dependencies: nil,
		})
		c.logger.Info("added delete StatefulSet task to deploy flow due to immutable field update task", "namespace", c.values.Namespace, "name", c.values.Name, "etcdUID", getOwnerReferenceNameWithUID(c.values.OwnerReference))
		return &taskID
	}
	return nil
}

func (c *component) addCreateOrPatchTask(g *flow.Graph, originalSts *appsv1.StatefulSet, taskIDDependency *flow.TaskID) {
	var (
		dependencies flow.TaskIDs
	)
	if taskIDDependency != nil {
		dependencies = flow.NewTaskIDs(taskIDDependency)
	}

	taskID := g.Add(flow.Task{
		Name: "sync StatefulSet task",
		Fn: func(ctx context.Context) error {
			c.logger.Info("createOrPatch sts", "namespace", c.values.Namespace, "name", c.values.Name, "replicas", c.values.Replicas)
			var (
				sts = originalSts
				err error
			)
			if taskIDDependency != nil {
				sts, err = c.getExistingSts(ctx)
				if err != nil {
					return err
				}
			}
			if sts == nil {
				sts = c.emptyStatefulset()
			}
			return c.createOrPatch(ctx, sts, c.values.Replicas, false)
		},
		Dependencies: dependencies,
	})
	c.logger.Info("added createOrPatch StatefulSet task to the deploy flow", "taskID", taskID, "namespace", c.values.Namespace, "etcdUID", getOwnerReferenceNameWithUID(c.values.OwnerReference), "StatefulSetName", c.values.Name, "replicas", c.values.Replicas)
}

func (c *component) getExistingSts(ctx context.Context) (*appsv1.StatefulSet, error) {
	sts, err := c.Get(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return sts, nil
}

func (c *component) updateAndWait(ctx context.Context, opName string, sts *appsv1.StatefulSet, replicas int32) error {
	c.logger.Info("Updating StatefulSet spec with Peer URL TLS mount", "namespace", c.values.Namespace, "name", c.values.Name, "operation", opName, "etcdUID", getOwnerReferenceNameWithUID(c.values.OwnerReference), "replicas", replicas)
	return c.doCreateOrUpdate(ctx, opName, sts, replicas, true)
}

func (c *component) waitUntilTLSEnabled(ctx context.Context, timeout time.Duration) error {
	return gardenerretry.UntilTimeout(ctx, defaultInterval, timeout, func(ctx context.Context) (bool, error) {
		tlsEnabled, err := utils.IsPeerURLTLSEnabled(ctx, c.client, c.values.Namespace, c.values.Name, c.logger)
		if err != nil {
			return gardenerretry.MinorError(err)
		}
		if !tlsEnabled {
			return gardenerretry.MinorError(fmt.Errorf("TLS not yet enabled for etcd [name: %s, namespace: %s]", c.values.Name, c.values.Namespace))
		}
		return gardenerretry.Ok()
	})
}

func (c *component) deleteAllStsPods(ctx context.Context, opName string, sts *appsv1.StatefulSet) error {
	replicas := sts.Spec.Replicas
	c.logger.Info("Deleting all StatefulSet pods", "namespace", c.values.Namespace, "name", c.values.Name, "operation", opName, "etcdUID", getOwnerReferenceNameWithUID(c.values.OwnerReference), "replicas", replicas, "matching labels", sts.Spec.Template.Labels)
	timeBeforeDeletion := time.Now()

	if err := c.client.DeleteAllOf(ctx, &corev1.Pod{}, client.InNamespace(sts.Namespace), client.MatchingLabels(sts.Spec.Template.Labels)); err != nil {
		return err
	}

	const timeout = 3 * time.Minute
	const interval = 2 * time.Second

	c.logger.Info("waiting for StatefulSet pods to start again after delete", "namespace", c.values.Namespace, "name", c.values.Name, "operation", opName, "etcdUID", getOwnerReferenceNameWithUID(c.values.OwnerReference), "replicas", replicas)
	return c.waitUntilPodsReady(ctx, sts, timeBeforeDeletion, interval, timeout)
}

func (c *component) doCreateOrUpdate(ctx context.Context, opName string, sts *appsv1.StatefulSet, replicas int32, preserveObjectMetadata bool) error {
	if err := c.createOrPatch(ctx, sts, replicas, preserveObjectMetadata); err != nil {
		return err
	}
	c.logger.Info("waiting for StatefulSet replicas to be ready", "namespace", c.values.Namespace, "name", c.values.Name, "operation", opName, "target-replicas", replicas)
	return c.waitDeploy(ctx, sts, replicas, defaultTimeout)
}

func (c *component) destroyAndWait(ctx context.Context, opName string) error {
	deleteAndWait := gardenercomponent.OpDestroyAndWait(c)
	c.logger.Info("deleting sts", "namespace", c.values.Namespace, "name", c.values.Name, "operation", opName, "etcdUID", getOwnerReferenceNameWithUID(c.values.OwnerReference))
	return deleteAndWait.Destroy(ctx)
}

func immutableFieldUpdate(sts *appsv1.StatefulSet, val Values) bool {
	return sts.Spec.ServiceName != val.PeerServiceName || sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement
}

func clusterScaledUpToMultiNode(val *Values, sts *appsv1.StatefulSet) bool {
	if sts != nil && sts.Spec.Replicas != nil {
		return (val.Replicas > 1 && *sts.Spec.Replicas == 1 && sts.Status.AvailableReplicas == 1) ||
			(metav1.HasAnnotation(sts.ObjectMeta, ScaleToMultiNodeAnnotationKey) &&
				(sts.Status.UpdatedReplicas < *sts.Spec.Replicas || sts.Status.AvailableReplicas < sts.Status.UpdatedReplicas))
	}
	return val.Replicas > 1 && val.StatusReplicas == 1
}

func (c *component) createOrPatch(ctx context.Context, sts *appsv1.StatefulSet, replicas int32, preserveAnnotations bool) error {
	mutatingFn := func() error {
		var stsOriginal = sts.DeepCopy()

		podVolumes, err := getVolumes(ctx, c.client, c.logger, c.values)
		if err != nil {
			return err
		}
		sts.ObjectMeta = getObjectMeta(&c.values, sts, preserveAnnotations)
		sts.Spec = appsv1.StatefulSetSpec{
			PodManagementPolicy: appsv1.ParallelPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Replicas:    &replicas,
			ServiceName: c.values.PeerServiceName,
			Selector: &metav1.LabelSelector{
				MatchLabels: c.values.Labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: c.values.Annotations,
					Labels:      utils.MergeStringMaps(make(map[string]string), c.values.AdditionalPodLabels, c.values.Labels),
				},
				Spec: corev1.PodSpec{
					HostAliases: []corev1.HostAlias{
						{
							IP:        "127.0.0.1",
							Hostnames: []string{c.values.Name + "-local"},
						},
					},
					ServiceAccountName:        c.values.ServiceAccountName,
					Affinity:                  c.values.Affinity,
					TopologySpreadConstraints: c.values.TopologySpreadConstraints,
					Containers: []corev1.Container{
						{
							Name:            "etcd",
							Image:           c.values.EtcdImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            c.values.EtcdCommandArgs,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler:        getReadinessHandler(c.values),
								InitialDelaySeconds: 15,
								PeriodSeconds:       5,
								FailureThreshold:    5,
							},
							Ports:        getEtcdPorts(c.values),
							Resources:    getEtcdResources(c.values),
							Env:          getEtcdEnvVars(c.values),
							VolumeMounts: getEtcdVolumeMounts(c.values),
						},
						{
							Name:            "backup-restore",
							Image:           c.values.BackupImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            c.values.EtcdBackupRestoreCommandArgs,
							Ports:           getBackupPorts(c.values),
							Resources:       getBackupResources(c.values),
							VolumeMounts:    getBackupRestoreVolumeMounts(c),
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										"SYS_PTRACE",
									},
								},
							},
						},
					},
					ShareProcessNamespace: pointer.Bool(true),
					Volumes:               podVolumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: c.values.VolumeClaimTemplateName,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: getStorageReq(c.values),
					},
				},
			},
		}
		if backupRestoreEnvVars, err := utils.GetBackupRestoreContainerEnvVars(c.values.BackupStore); err != nil {
			return fmt.Errorf("unable to fetch env vars for backup-restore container: %v", err)
		} else {
			sts.Spec.Template.Spec.Containers[1].Env = backupRestoreEnvVars
		}
		if c.values.StorageClass != nil && *c.values.StorageClass != "" {
			sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName = c.values.StorageClass
		}
		if c.values.PriorityClassName != nil {
			sts.Spec.Template.Spec.PriorityClassName = *c.values.PriorityClassName
		}
		if c.values.UseEtcdWrapper {
			// sections to add only when using etcd wrapper
			// TODO: @aaronfern add this back to sts.Spec when UseEtcdWrapper becomes GA
			sts.Spec.Template.Spec.InitContainers = []corev1.Container{
				{
					Name:            "change-permissions",
					Image:           c.values.InitContainerImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"sh", "-c", "--"},
					Args:            []string{"chown -R 65532:65532 /var/etcd/data"},
					VolumeMounts:    getEtcdVolumeMounts(c.values),
					SecurityContext: &corev1.SecurityContext{
						RunAsGroup:   pointer.Int64(0),
						RunAsNonRoot: pointer.Bool(false),
						RunAsUser:    pointer.Int64(0),
					},
				},
			}
			if c.values.BackupStore != nil {
				// Special container to change permissions of backup bucket folder to 65532 (nonroot)
				// Only used with local provider
				prov, _ := utils.StorageProviderFromInfraProvider(c.values.BackupStore.Provider)
				if prov == utils.Local {
					sts.Spec.Template.Spec.InitContainers = append(sts.Spec.Template.Spec.InitContainers, corev1.Container{
						Name:            "change-backup-bucket-permissions",
						Image:           c.values.InitContainerImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"sh", "-c", "--"},
						Args:            []string{fmt.Sprintf("chown -R 65532:65532 /home/nonroot/%s", *c.values.BackupStore.Container)},
						VolumeMounts:    getBackupRestoreVolumeMounts(c),
						SecurityContext: &corev1.SecurityContext{
							RunAsGroup:   pointer.Int64(0),
							RunAsNonRoot: pointer.Bool(false),
							RunAsUser:    pointer.Int64(0),
						},
					})
				}
			}
			sts.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
				RunAsGroup:   pointer.Int64(65532),
				RunAsNonRoot: pointer.Bool(true),
				RunAsUser:    pointer.Int64(65532),
				FSGroup:      pointer.Int64(65532),
			}
		}

		if stsOriginal.Generation > 0 {
			// Keep immutable fields
			sts.Spec.PodManagementPolicy = stsOriginal.Spec.PodManagementPolicy
			sts.Spec.ServiceName = stsOriginal.Spec.ServiceName
		}
		return nil
	}

	operationResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, c.client, sts, mutatingFn)
	if err != nil {
		return err
	}
	c.logger.Info("createOrPatch is completed", "namespace", sts.Namespace, "name", sts.Name, "operation-result", operationResult)
	return nil
}

// fetchPVCEventsForStatefulset fetches events for PVCs for a statefulset and return the events,
// as well as possible error and name of the PVC that caused the error
func (c *component) fetchPVCEventsForStatefulset(ctx context.Context, ss *appsv1.StatefulSet) (string, *string, error) {
	pvcs := &corev1.PersistentVolumeClaimList{}
	if err := c.client.List(ctx, pvcs, client.InNamespace(ss.GetNamespace())); err != nil {
		return "", pointer.String(""), err
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
				return "", &pvc.Name, err
			}
			if messages != "" {
				pvcMessages += fmt.Sprintf("Warning for PVC %s:\n%s\n", pvc.Name, messages)
			}
		}
	}
	return pvcMessages, nil, nil
}

func (c *component) emptyStatefulset() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.values.Name,
			Namespace: c.values.Namespace,
		},
	}
}

func getObjectMeta(val *Values, sts *appsv1.StatefulSet, preserveAnnotations bool) metav1.ObjectMeta {
	annotations := sts.Annotations
	if !preserveAnnotations {
		annotations = getStsAnnotations(val, sts)
	}

	return metav1.ObjectMeta{
		Name:            val.Name,
		Namespace:       val.Namespace,
		Labels:          val.Labels,
		Annotations:     annotations,
		OwnerReferences: []metav1.OwnerReference{val.OwnerReference},
	}
}

func getStsAnnotations(val *Values, sts *appsv1.StatefulSet) map[string]string {
	annotations := utils.MergeStringMaps(
		map[string]string{
			common.GardenerOwnedBy:   fmt.Sprintf("%s/%s", val.Namespace, val.Name),
			common.GardenerOwnerType: "etcd",
		},
		val.Annotations,
	)

	if clusterScaledUpToMultiNode(val, sts) {
		annotations[ScaleToMultiNodeAnnotationKey] = ""
	}
	return annotations
}

func getEtcdPorts(val Values) []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "server",
			Protocol:      "TCP",
			ContainerPort: pointer.Int32Deref(val.ServerPort, defaultServerPort),
		},
		{
			Name:          "client",
			Protocol:      "TCP",
			ContainerPort: pointer.Int32Deref(val.ClientPort, defaultClientPort),
		},
	}
}

var defaultResourceRequirements = corev1.ResourceRequirements{
	Requests: corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("50m"),
		corev1.ResourceMemory: resource.MustParse("128Mi"),
	},
}

func getEtcdResources(val Values) corev1.ResourceRequirements {
	if val.EtcdResourceRequirements != nil {
		return *val.EtcdResourceRequirements
	}

	return defaultResourceRequirements
}

func getEtcdEnvVars(val Values) []corev1.EnvVar {
	protocol := "http"
	if val.BackupTLS != nil {
		protocol = "https"
	}

	endpoint := fmt.Sprintf("%s://%s-local:%d", protocol, val.Name, pointer.Int32Deref(val.BackupPort, defaultBackupPort))

	return []corev1.EnvVar{
		utils.GetEnvVarFromValue("ENABLE_TLS", strconv.FormatBool(val.BackupTLS != nil)),
		utils.GetEnvVarFromValue("BACKUP_ENDPOINT", endpoint),
	}
}

func getEtcdVolumeMounts(val Values) []corev1.VolumeMount {
	vms := []corev1.VolumeMount{
		{
			Name:      val.VolumeClaimTemplateName,
			MountPath: "/var/etcd/data/",
		},
	}

	vms = append(vms, getSecretVolumeMounts(val.ClientUrlTLS, val.PeerUrlTLS)...)

	return vms
}

func getSecretVolumeMounts(clientUrlTLS, peerUrlTLS *druidv1alpha1.TLSConfig) []corev1.VolumeMount {
	var vms []corev1.VolumeMount

	if clientUrlTLS != nil {
		vms = append(vms, corev1.VolumeMount{
			Name:      "client-url-ca-etcd",
			MountPath: "/var/etcd/ssl/client/ca",
		}, corev1.VolumeMount{
			Name:      "client-url-etcd-server-tls",
			MountPath: "/var/etcd/ssl/client/server",
		}, corev1.VolumeMount{
			Name:      "client-url-etcd-client-tls",
			MountPath: "/var/etcd/ssl/client/client",
		})
	}

	if peerUrlTLS != nil {
		vms = append(vms, corev1.VolumeMount{
			Name:      "peer-url-ca-etcd",
			MountPath: "/var/etcd/ssl/peer/ca",
		}, corev1.VolumeMount{
			Name:      "peer-url-etcd-server-tls",
			MountPath: "/var/etcd/ssl/peer/server",
		})
	}

	return vms
}

func getBackupRestoreVolumeMounts(c *component) []corev1.VolumeMount {
	vms := []corev1.VolumeMount{
		{
			Name:      c.values.VolumeClaimTemplateName,
			MountPath: "/var/etcd/data",
		},
		{
			Name:      "etcd-config-file",
			MountPath: "/var/etcd/config/",
		},
	}

	vms = append(vms, getSecretVolumeMounts(c.values.ClientUrlTLS, c.values.PeerUrlTLS)...)

	if c.values.BackupStore == nil {
		return vms
	}

	provider, err := utils.StorageProviderFromInfraProvider(c.values.BackupStore.Provider)
	if err != nil {
		return vms
	}

	switch provider {
	case utils.Local:
		if c.values.BackupStore.Container != nil {
			if c.featureGates["UseEtcdWrapper"] {
				vms = append(vms, corev1.VolumeMount{
					Name:      "host-storage",
					MountPath: "/home/nonroot/" + pointer.StringDeref(c.values.BackupStore.Container, ""),
				})
			} else {
				vms = append(vms, corev1.VolumeMount{
					Name:      "host-storage",
					MountPath: pointer.StringDeref(c.values.BackupStore.Container, ""),
				})
			}
		}
	case utils.GCS:
		vms = append(vms, corev1.VolumeMount{
			Name:      "etcd-backup",
			MountPath: "/var/.gcp/",
		})
	case utils.S3, utils.ABS, utils.OSS, utils.Swift, utils.OCS:
		vms = append(vms, corev1.VolumeMount{
			Name:      "etcd-backup",
			MountPath: "/var/etcd-backup/",
		})
	}

	return vms
}

func getStorageReq(val Values) corev1.ResourceRequirements {
	storageCapacity := defaultStorageCapacity
	if val.StorageCapacity != nil {
		storageCapacity = *val.StorageCapacity
	}

	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceStorage: storageCapacity,
		},
	}
}

func getBackupPorts(val Values) []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "server",
			Protocol:      "TCP",
			ContainerPort: pointer.Int32Deref(val.BackupPort, defaultBackupPort),
		},
	}
}

func getBackupResources(val Values) corev1.ResourceRequirements {
	if val.BackupResourceRequirements != nil {
		return *val.BackupResourceRequirements
	}
	return defaultResourceRequirements
}

func getVolumes(ctx context.Context, cl client.Client, logger logr.Logger, val Values) ([]corev1.Volume, error) {
	vs := []corev1.Volume{
		{
			Name: "etcd-config-file",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: val.ConfigMapName,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "etcd.conf.yaml",
							Path: "etcd.conf.yaml",
						},
					},
					DefaultMode: pointer.Int32(0640),
				},
			},
		},
	}

	if val.ClientUrlTLS != nil {
		vs = append(vs, corev1.Volume{
			Name: "client-url-ca-etcd",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  val.ClientUrlTLS.TLSCASecretRef.Name,
					DefaultMode: pointer.Int32(0640),
				},
			},
		},
			corev1.Volume{
				Name: "client-url-etcd-server-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  val.ClientUrlTLS.ServerTLSSecretRef.Name,
						DefaultMode: pointer.Int32(0640),
					},
				},
			},
			corev1.Volume{
				Name: "client-url-etcd-client-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  val.ClientUrlTLS.ClientTLSSecretRef.Name,
						DefaultMode: pointer.Int32(0640),
					},
				},
			})
	}

	if val.PeerUrlTLS != nil {
		vs = append(vs, corev1.Volume{
			Name: "peer-url-ca-etcd",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  val.PeerUrlTLS.TLSCASecretRef.Name,
					DefaultMode: pointer.Int32(0640),
				},
			},
		},
			corev1.Volume{
				Name: "peer-url-etcd-server-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  val.PeerUrlTLS.ServerTLSSecretRef.Name,
						DefaultMode: pointer.Int32(0640),
					},
				},
			})
	}

	if val.BackupStore == nil {
		return vs, nil
	}

	storeValues := val.BackupStore
	provider, err := utils.StorageProviderFromInfraProvider(storeValues.Provider)
	if err != nil {
		return vs, nil
	}

	switch provider {
	case "Local":
		hostPath, err := utils.GetHostMountPathFromSecretRef(ctx, cl, logger, storeValues, val.Namespace)
		if err != nil {
			return nil, err
		}

		hpt := corev1.HostPathDirectory
		vs = append(vs, corev1.Volume{
			Name: "host-storage",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath + "/" + pointer.StringDeref(storeValues.Container, ""),
					Type: &hpt,
				},
			},
		})
	case utils.GCS, utils.S3, utils.OSS, utils.ABS, utils.Swift, utils.OCS:
		if storeValues.SecretRef == nil {
			return nil, fmt.Errorf("no secretRef configured for backup store")
		}

		vs = append(vs, corev1.Volume{
			Name: "etcd-backup",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  storeValues.SecretRef.Name,
					DefaultMode: pointer.Int32(0640),
				},
			},
		})
	}

	return vs, nil
}

func getReadinessHandler(val Values) corev1.ProbeHandler {
	if val.Replicas > 1 {
		// TODO(timuthy): Special handling for multi-node etcd can be removed as soon as
		// etcd-backup-restore supports `/healthz` for etcd followers, see https://github.com/gardener/etcd-backup-restore/pull/491.
		return getReadinessHandlerForMultiNode(val)
	}
	return getReadinessHandlerForSingleNode(val)
}

func getReadinessHandlerForSingleNode(val Values) corev1.ProbeHandler {
	scheme := corev1.URISchemeHTTPS
	if val.BackupTLS == nil {
		scheme = corev1.URISchemeHTTP
	}

	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path:   "/healthz",
			Port:   intstr.FromInt(int(pointer.Int32Deref(val.BackupPort, defaultBackupPort))),
			Scheme: scheme,
		},
	}
}

func getReadinessHandlerForMultiNode(val Values) corev1.ProbeHandler {
	if val.UseEtcdWrapper {
		//TODO @aaronfern: remove this feature gate when UseEtcdWrapper becomes GA
		scheme := corev1.URISchemeHTTPS
		if val.BackupTLS == nil {
			scheme = corev1.URISchemeHTTP
		}

		return corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/readyz",
				Port:   intstr.FromInt(int(pointer.Int32Deref(val.WrapperPort, defaultWrapperPort))),
				Scheme: scheme,
			},
		}
	}

	return corev1.ProbeHandler{
		Exec: &corev1.ExecAction{
			Command: val.ReadinessProbeCommand,
		},
	}
}

func getOwnerReferenceNameWithUID(ref metav1.OwnerReference) string {
	return fmt.Sprintf("%s:%s", ref.Name, ref.UID)
}
