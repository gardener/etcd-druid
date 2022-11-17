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
	"github.com/gardener/etcd-druid/pkg/utils"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/utils/flow"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/retry"
	gardenerretry "github.com/gardener/gardener/pkg/utils/retry"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	client client.Client
	logger logr.Logger
	values Values
}

// New creates a new statefulset deployer instance.
func New(c client.Client, logger logr.Logger, values Values) Interface {
	objectLogger := logger.WithValues("sts", client.ObjectKey{Name: values.Name, Namespace: values.Namespace})

	return &component{
		client: c,
		logger: objectLogger,
		values: values,
	}
}

// Destroy deletes the StatefulSet
func (c *component) Destroy(ctx context.Context) error {
	sts := c.emptyStatefulset()
	if err := client.IgnoreNotFound(c.client.Delete(ctx, sts)); err != nil {
		return err
	}
	return nil
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
)

// Wait waits for the deployment of the StatefulSet to finish
func (c *component) Wait(ctx context.Context) error {
	sts := c.emptyStatefulset()
	err := c.waitDeploy(ctx, sts, c.values.Replicas)
	if err != nil {
		messages, err2 := c.fetchPVCEventsFor(ctx, sts)
		if err2 != nil {
			c.logger.Error(err2, "Error while fetching events for depending PVC")
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

func (c *component) waitDeploy(ctx context.Context, sts *appsv1.StatefulSet, replicas int32) error {
	return gardenerretry.UntilTimeout(ctx, defaultInterval, defaultTimeout, func(ctx context.Context) (bool, error) {
		if err := c.client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
			if apierrors.IsNotFound(err) {
				return gardenerretry.MinorError(err)
			}
			return gardenerretry.SevereError(err)
		}
		if err := utils.CheckStatefulSet(replicas, sts); err != nil {
			return gardenerretry.MinorError(err)
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
			return retry.Ok()
		case err == nil:
			// StatefulSet is still available, so we should retry.
			return false, nil
		default:
			return retry.SevereError(err)
		}
	})
}

func (c *component) createDeployFlow(ctx context.Context) (*flow.Flow, error) {
	sts, err := c.getExistingSts(ctx)
	if err != nil {
		return nil, err
	}

	flowName := fmt.Sprintf("(etcd: %s) Deploy Flow for StatefulSet %s for Namespace: %s", c.values.EtcdUID, c.values.Name, c.values.Namespace)
	g := flow.NewGraph(flowName)

	var taskID *flow.TaskID
	if sts != nil {
		taskID = c.addTasksForPeerUrlTLSChangedToEnabled(g, sts)
		if taskID == nil {
			// if sts recreation tasks for peer url tls have already been added then there is no need to additionally add tasks to explicitly handle immutable field updates.
			taskID = c.addImmutableFieldUpdateTask(g, sts)
		}
	}
	if taskID != nil || sts == nil {
		sts = c.emptyStatefulset()
	}
	c.addCreateOrPatchTask(g, sts, taskID)

	return g.Compile(), nil
}

// addTasksForPeerUrlTLSChangedToEnabled adds tasks to the deployment flow in case the peer url tls has been changed to `enabled`.
// To ensure that the tls enablement of peer url is properly reflected in etcd, the etcd StatefulSet should be recreated twice. Assume
// that the current state of etcd is that peer url is not TLS enabled. First restart pushes a new configuration which contains
// PeerUrlTLS configuration. etcd-backup-restore will update the member peer url. This will result in the change of the peer url in the etcd db file,
// but it will not reflect in the already running etcd container. Ideally a restart of an etcd container would have been sufficient but currently k8s
// does not expose an API to force restart a single container within a pod. Therefore, we delete the sts once again. When it gets created the second time
// it will now start etcd with the correct peer url which will be TLS enabled.
func (c *component) addTasksForPeerUrlTLSChangedToEnabled(g *flow.Graph, sts *appsv1.StatefulSet) *flow.TaskID {
	var existingStsReplicas int32
	if sts.Spec.Replicas != nil {
		existingStsReplicas = *sts.Spec.Replicas
	}

	if c.values.PeerTLSChangedToEnabled {
		firstDelOpName := "(recreate-sts): delete sts due to peer url tls"
		delTaskID := g.Add(flow.Task{
			Name:         firstDelOpName,
			Fn:           func(ctx context.Context) error { return c.destroyAndWait(ctx, firstDelOpName) },
			Dependencies: nil,
		})

		// ensure that the recreation of StatefulSet is done using existing replicas and not the desired replicas as contained in c.values.Replicas
		createOrPatchOpName := fmt.Sprintf("(recreate-sts): create sts due to peer url tls with replicas: %d", existingStsReplicas)
		createTaskID := g.Add(flow.Task{
			Name:         createOrPatchOpName,
			Fn:           func(ctx context.Context) error { return c.createAndWait(ctx, createOrPatchOpName, existingStsReplicas) },
			Dependencies: flow.NewTaskIDs(delTaskID),
		})

		secondDelOpName := "(recreate-sts): second-delete of sts due to peer url tls"
		secondDeleteTaskID := g.Add(flow.Task{
			Name:         secondDelOpName,
			Fn:           func(ctx context.Context) error { return c.destroyAndWait(ctx, secondDelOpName) },
			Dependencies: flow.NewTaskIDs(createTaskID),
		})

		c.logger.Info("added tasks to deploy flow due to peer url tls changed to enabled", "namespace", c.values.Namespace, "name", c.values.Name, "etcdUID", c.values.EtcdUID, "replicas", c.values.Replicas)
		return &secondDeleteTaskID
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
		c.logger.Info("added delete StatefulSet task to deploy flow due to immutable field update task", "namespace", c.values.Namespace, "name", c.values.Name, "etcdUID", c.values.EtcdUID)
		return &taskID
	}
	return nil
}

func (c *component) addCreateOrPatchTask(g *flow.Graph, sts *appsv1.StatefulSet, taskIDDependency *flow.TaskID) {
	var dependencies flow.TaskIDs
	if taskIDDependency != nil {
		dependencies = flow.NewTaskIDs(taskIDDependency)
	}

	taskID := g.Add(flow.Task{
		Name: "sync StatefulSet task",
		Fn: func(ctx context.Context) error {
			c.logger.Info("createOrPatch sts")
			return c.createOrPatch(ctx, sts, c.values.Replicas)
		},
		Dependencies: dependencies,
	})
	c.logger.Info("added createOrPatch StatefulSet task to the deploy flow", "taskID", taskID, "namespace", c.values.Namespace, "etcdUID", c.values.EtcdUID, "StatefulSetName", c.values.Name, "replicas", c.values.Replicas)
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

func (c *component) createAndWait(ctx context.Context, opName string, replicas int32) error {
	sts := c.emptyStatefulset()
	c.logger.Info("createOrPatch sts", "namespace", c.values.Namespace, "name", c.values.Name, "operation", opName, "etcdUID", c.values.EtcdUID, "replicas", replicas)
	err := c.createOrPatch(ctx, sts, replicas)
	if err != nil {
		return err
	}
	c.logger.Info("waiting for createOrPatch sts to finish", "namespace", c.values.Namespace, "name", c.values.Name, "operation", opName, "etcdUID", c.values.EtcdUID, "replicas", replicas)
	return c.waitDeploy(ctx, sts, replicas)
}

func (c *component) destroyAndWait(ctx context.Context, opName string) error {
	deleteAndWait := gardenercomponent.OpDestroyAndWait(c)
	c.logger.Info("deleting sts", "namespace", c.values.Namespace, "name", c.values.Name, "operation", opName, "etcdUID", c.values.EtcdUID)
	if err := deleteAndWait.Destroy(ctx); err != nil {
		return err
	}
	return nil
}

func immutableFieldUpdate(sts *appsv1.StatefulSet, val Values) bool {
	return sts.Spec.ServiceName != val.PeerServiceName || sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement
}

func clusterScaledUpToMultiNode(val *Values, sts *appsv1.StatefulSet) bool {
	if sts != nil && sts.Spec.Replicas != nil {
		return val.Replicas > 1 && *sts.Spec.Replicas == 1
	}
	return val.Replicas > 1 && val.StatusReplicas == 1
}

func (c *component) createOrPatch(ctx context.Context, sts *appsv1.StatefulSet, replicas int32) error {
	var (
		stsOriginal = sts.DeepCopy()
		patch       = client.StrategicMergeFrom(stsOriginal)
	)

	podVolumes, err := getVolumes(ctx, c.client, c.logger, c.values)
	if err != nil {
		return err
	}

	sts.ObjectMeta = getObjectMeta(&c.values, sts)
	sts.Spec = appsv1.StatefulSetSpec{
		PodManagementPolicy: appsv1.ParallelPodManagement,
		UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		},
		Replicas:    &replicas,
		ServiceName: c.values.PeerServiceName,
		Selector: &metav1.LabelSelector{
			MatchLabels: getCommonLabels(&c.values),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: c.values.Annotations,
				Labels:      sts.GetLabels(),
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
						Command:         c.values.EtcdCommand,
						ReadinessProbe: &corev1.Probe{
							Handler:             getReadinessHandler(c.values),
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
						Command:         c.values.EtcdBackupCommand,
						Ports:           getBackupPorts(c.values),
						Resources:       getBackupResources(c.values),
						Env:             getBackupRestoreEnvVars(c.values),
						VolumeMounts:    getBackupRestoreVolumeMounts(c.values),
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
	if c.values.StorageClass != nil && *c.values.StorageClass != "" {
		sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName = c.values.StorageClass
	}
	if c.values.PriorityClassName != nil {
		sts.Spec.Template.Spec.PriorityClassName = *c.values.PriorityClassName
	}

	if stsOriginal.Generation > 0 {
		// Keep immutable fields
		sts.Spec.PodManagementPolicy = stsOriginal.Spec.PodManagementPolicy
		sts.Spec.ServiceName = stsOriginal.Spec.ServiceName
		return c.client.Patch(ctx, sts, patch)
	}

	return c.client.Create(ctx, sts)
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

func (c *component) emptyStatefulset() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.values.Name,
			Namespace: c.values.Namespace,
		},
	}
}

func getCommonLabels(val *Values) map[string]string {
	return map[string]string{
		"name":     "etcd",
		"instance": val.Name,
	}
}

func getObjectMeta(val *Values, sts *appsv1.StatefulSet) metav1.ObjectMeta {
	labels := utils.MergeStringMaps(getCommonLabels(val), val.Labels)
	annotations := getStsAnnotations(val, sts)
	ownerRefs := []metav1.OwnerReference{
		{
			APIVersion:         druidv1alpha1.GroupVersion.String(),
			Kind:               "Etcd",
			Name:               val.Name,
			UID:                val.EtcdUID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	}

	return metav1.ObjectMeta{
		Name:            val.Name,
		Namespace:       val.Namespace,
		Labels:          labels,
		Annotations:     annotations,
		OwnerReferences: ownerRefs,
	}
}

const (
	scaleToMultiNodeAnnotationKey = "gardener.cloud/scaled-to-multi-node"
	ownedByAnnotationKey          = "gardener.cloud/owned-by"
	ownerTypeAnnotationKey        = "gardener.cloud/owner-type"
)

func getStsAnnotations(val *Values, sts *appsv1.StatefulSet) map[string]string {
	annotations := utils.MergeStringMaps(
		map[string]string{
			ownedByAnnotationKey:   fmt.Sprintf("%s/%s", val.Namespace, val.Name),
			ownerTypeAnnotationKey: "etcd",
		},
		val.Annotations,
	)
	if clusterScaledUpToMultiNode(val, sts) {
		annotations[scaleToMultiNodeAnnotationKey] = ""
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
		getEnvVarFromValue("ENABLE_TLS", strconv.FormatBool(val.BackupTLS != nil)),
		getEnvVarFromValue("BACKUP_ENDPOINT", endpoint),
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

func getBackupRestoreVolumeMounts(val Values) []corev1.VolumeMount {
	vms := []corev1.VolumeMount{
		{
			Name:      val.VolumeClaimTemplateName,
			MountPath: "/var/etcd/data",
		},
		{
			Name:      "etcd-config-file",
			MountPath: "/var/etcd/config/",
		},
	}

	vms = append(vms, getSecretVolumeMounts(val.ClientUrlTLS, val.PeerUrlTLS)...)

	if val.BackupStore == nil {
		return vms
	}

	provider, err := utils.StorageProviderFromInfraProvider(val.BackupStore.Provider)
	if err != nil {
		return vms
	}

	switch provider {
	case utils.Local:
		if val.BackupStore.Container != nil {
			vms = append(vms, corev1.VolumeMount{
				Name:      "host-storage",
				MountPath: pointer.StringPtrDerefOr(val.BackupStore.Container, ""),
			})
		}
	case utils.GCS:
		vms = append(vms, corev1.VolumeMount{
			Name:      "etcd-backup",
			MountPath: "/root/.gcp/",
		})
	case utils.S3, utils.ABS, utils.OSS, utils.Swift, utils.OCS:
		vms = append(vms, corev1.VolumeMount{
			Name:      "etcd-backup",
			MountPath: "/root/etcd-backup/",
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
					DefaultMode: pointer.Int32(0644),
				},
			},
		},
	}

	if val.ClientUrlTLS != nil {
		vs = append(vs, corev1.Volume{
			Name: "client-url-ca-etcd",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: val.ClientUrlTLS.TLSCASecretRef.Name,
				},
			},
		},
			corev1.Volume{
				Name: "client-url-etcd-server-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: val.ClientUrlTLS.ServerTLSSecretRef.Name,
					},
				},
			},
			corev1.Volume{
				Name: "client-url-etcd-client-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: val.ClientUrlTLS.ClientTLSSecretRef.Name,
					},
				},
			})
	}

	if val.PeerUrlTLS != nil {
		vs = append(vs, corev1.Volume{
			Name: "peer-url-ca-etcd",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: val.PeerUrlTLS.TLSCASecretRef.Name,
				},
			},
		},
			corev1.Volume{
				Name: "peer-url-etcd-server-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: val.PeerUrlTLS.ServerTLSSecretRef.Name,
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
					Path: hostPath + "/" + pointer.StringPtrDerefOr(storeValues.Container, ""),
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
					SecretName: storeValues.SecretRef.Name,
				},
			},
		})
	}

	return vs, nil
}

func getBackupRestoreEnvVars(val Values) []corev1.EnvVar {
	var (
		env              []corev1.EnvVar
		storageContainer string
		storeValues      = val.BackupStore
	)

	if val.BackupStore != nil {
		storageContainer = pointer.StringDeref(val.BackupStore.Container, "")
	}

	// TODO(timuthy, shreyas-s-rao): Move STORAGE_CONTAINER a few lines below so that we can append and exit in one step. This should only be done in a release where a restart of etcd is acceptable.
	env = append(env, getEnvVarFromValue("STORAGE_CONTAINER", storageContainer))
	env = append(env, getEnvVarFromField("POD_NAME", "metadata.name"))
	env = append(env, getEnvVarFromField("POD_NAMESPACE", "metadata.namespace"))

	if storeValues == nil {
		return env
	}

	provider, err := utils.StorageProviderFromInfraProvider(val.BackupStore.Provider)
	if err != nil {
		return env
	}

	// TODO(timuthy): move this to a non root path when we switch to a rootless distribution
	const credentialsMountPath = "/root/etcd-backup"
	switch provider {
	case utils.S3:
		env = append(env, getEnvVarFromValue("AWS_APPLICATION_CREDENTIALS", credentialsMountPath))

	case utils.ABS:
		env = append(env, getEnvVarFromValue("AZURE_APPLICATION_CREDENTIALS", credentialsMountPath))

	case utils.GCS:
		env = append(env, getEnvVarFromValue("GOOGLE_APPLICATION_CREDENTIALS", "/root/.gcp/serviceaccount.json"))

	case utils.Swift:
		env = append(env, getEnvVarFromValue("OPENSTACK_APPLICATION_CREDENTIALS", credentialsMountPath))

	case utils.OSS:
		env = append(env, getEnvVarFromValue("ALICLOUD_APPLICATION_CREDENTIALS", credentialsMountPath))

	case utils.ECS:
		env = append(env, getEnvVarFromSecrets("ECS_ENDPOINT", storeValues.SecretRef.Name, "endpoint"))
		env = append(env, getEnvVarFromSecrets("ECS_ACCESS_KEY_ID", storeValues.SecretRef.Name, "accessKeyID"))
		env = append(env, getEnvVarFromSecrets("ECS_SECRET_ACCESS_KEY", storeValues.SecretRef.Name, "secretAccessKey"))

	case utils.OCS:
		env = append(env, getEnvVarFromValue("OPENSHIFT_APPLICATION_CREDENTIALS", credentialsMountPath))
	}

	return env
}

func getEnvVarFromValue(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}

func getEnvVarFromField(name, fieldPath string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: fieldPath,
			},
		},
	}
}

func getEnvVarFromSecrets(name, secretName, secretKey string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: secretKey,
			},
		},
	}
}

func getReadinessHandler(val Values) corev1.Handler {
	if val.Replicas > 1 {
		// TODO(timuthy): Special handling for multi-node etcd can be removed as soon as
		// etcd-backup-restore supports `/healthz` for etcd followers, see https://github.com/gardener/etcd-backup-restore/pull/491.
		return getReadinessHandlerForMultiNode(val)
	}
	return getReadinessHandlerForSingleNode(val)
}

func getReadinessHandlerForSingleNode(val Values) corev1.Handler {
	scheme := corev1.URISchemeHTTPS
	if val.BackupTLS == nil {
		scheme = corev1.URISchemeHTTP
	}

	return corev1.Handler{
		HTTPGet: &corev1.HTTPGetAction{
			Path:   "/healthz",
			Port:   intstr.FromInt(int(pointer.Int32Deref(val.BackupPort, defaultBackupPort))),
			Scheme: scheme,
		},
	}
}

func getReadinessHandlerForMultiNode(val Values) corev1.Handler {
	return corev1.Handler{
		Exec: &corev1.ExecAction{
			Command: val.ReadinessProbeCommand,
		},
	}
}
