// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"
	"strconv"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/operator/component"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// defaults
// -----------------------------------------------------------------------------------------
const (
	defaultWrapperPort             int   = 9095
	defaultMaxBackupsLimitBasedGC  int32 = 7
	defaultQuota                   int64 = 8 * 1024 * 1024 * 1024 // 8Gi
	defaultSnapshotMemoryLimit     int64 = 100 * 1024 * 1024      // 100Mi
	defaultHeartbeatDuration             = "10s"
	defaultGbcPolicy                     = "LimitBased"
	defaultAutoCompactionRetention       = "30m"
	defaultEtcdSnapshotTimeout           = "15m"
	defaultEtcdDefragTimeout             = "15m"
	defaultAutoCompactionMode            = "periodic"
	defaultEtcdConnectionTimeout         = "5m"
	defaultPodManagementPolicy           = appsv1.ParallelPodManagement
	nonRootUser                          = int64(65532)
)

var (
	defaultStorageCapacity      = apiresource.MustParse("16Gi")
	defaultResourceRequirements = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    apiresource.MustParse("50m"),
			corev1.ResourceMemory: apiresource.MustParse("128Mi"),
		},
	}
	defaultUpdateStrategy = appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType}
)

type stsBuilder struct {
	client                 client.Client
	etcd                   *druidv1alpha1.Etcd
	replicas               int32
	useEtcdWrapper         bool
	provider               *string
	etcdImage              string
	etcdBackupRestoreImage string
	initContainerImage     string
	sts                    *appsv1.StatefulSet
	logger                 logr.Logger

	clientPort int32
	serverPort int32
	backupPort int32
}

func newStsBuilder(client client.Client,
	logger logr.Logger,
	etcd *druidv1alpha1.Etcd,
	replicas int32,
	useEtcdWrapper bool,
	imageVector imagevector.ImageVector,
	sts *appsv1.StatefulSet) (*stsBuilder, error) {
	etcdImage, etcdBackupRestoreImage, initContainerImage, err := utils.GetEtcdImages(etcd, imageVector, useEtcdWrapper)
	if err != nil {
		return nil, err
	}
	provider, err := getBackupStoreProvider(etcd)
	if err != nil {
		return nil, err
	}
	return &stsBuilder{
		client:                 client,
		logger:                 logger,
		etcd:                   etcd,
		replicas:               replicas,
		useEtcdWrapper:         useEtcdWrapper,
		provider:               provider,
		etcdImage:              etcdImage,
		etcdBackupRestoreImage: etcdBackupRestoreImage,
		initContainerImage:     initContainerImage,
		sts:                    sts,
		clientPort:             pointer.Int32Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient),
		serverPort:             pointer.Int32Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer),
		backupPort:             pointer.Int32Deref(etcd.Spec.Backup.Port, common.DefaultPortEtcdBackupRestore),
	}, nil
}

// Build builds the StatefulSet for the given Etcd.
func (b *stsBuilder) Build(ctx component.OperatorContext) error {
	b.createStatefulSetObjectMeta()
	return b.createStatefulSetSpec(ctx)
}

func (b *stsBuilder) createStatefulSetObjectMeta() {
	b.sts.ObjectMeta = metav1.ObjectMeta{
		Name:            b.etcd.Name,
		Namespace:       b.etcd.Namespace,
		Labels:          b.getStatefulSetLabels(),
		OwnerReferences: []metav1.OwnerReference{b.etcd.GetAsOwnerReference()},
	}
}

func (b *stsBuilder) getStatefulSetLabels() map[string]string {
	stsLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameStatefulSet,
		druidv1alpha1.LabelAppNameKey:   b.etcd.Name,
	}
	return utils.MergeMaps(b.etcd.GetDefaultLabels(), stsLabels)
}

func (b *stsBuilder) createStatefulSetSpec(ctx component.OperatorContext) error {
	podVolumes, err := b.getPodVolumes(ctx)
	if err != nil {
		return err
	}

	backupRestoreContainer, err := b.getBackupRestoreContainer()
	if err != nil {
		return err
	}

	b.sts.Spec = appsv1.StatefulSetSpec{
		Replicas: pointer.Int32(b.replicas),
		Selector: &metav1.LabelSelector{
			MatchLabels: b.etcd.GetDefaultLabels(),
		},
		PodManagementPolicy:  defaultPodManagementPolicy,
		UpdateStrategy:       defaultUpdateStrategy,
		VolumeClaimTemplates: b.getVolumeClaimTemplates(),
		ServiceName:          b.etcd.GetPeerServiceName(),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      utils.MergeMaps(b.etcd.Spec.Labels, b.getStatefulSetLabels()),
				Annotations: b.getPodTemplateAnnotations(ctx),
			},
			Spec: corev1.PodSpec{
				HostAliases:           b.getHostAliases(),
				ServiceAccountName:    b.etcd.GetServiceAccountName(),
				ShareProcessNamespace: pointer.Bool(true),
				InitContainers:        b.getPodInitContainers(),
				Containers: []corev1.Container{
					b.getEtcdContainer(),
					backupRestoreContainer,
				},
				SecurityContext:           b.getPodSecurityContext(),
				Affinity:                  b.etcd.Spec.SchedulingConstraints.Affinity,
				TopologySpreadConstraints: b.etcd.Spec.SchedulingConstraints.TopologySpreadConstraints,
				Volumes:                   podVolumes,
				PriorityClassName:         utils.TypeDeref[string](b.etcd.Spec.PriorityClassName, ""),
			},
		},
	}
	return nil
}

func (b *stsBuilder) getHostAliases() []corev1.HostAlias {
	return []corev1.HostAlias{
		{
			IP:        "127.0.0.1",
			Hostnames: []string{b.etcd.Name + "-local"},
		},
	}
}

func (b *stsBuilder) getPodTemplateAnnotations(ctx component.OperatorContext) map[string]string {
	if configMapCheckSum, ok := ctx.Data[common.CheckSumKeyConfigMap]; ok {
		return utils.MergeMaps(b.etcd.Spec.Annotations, map[string]string{
			common.CheckSumKeyConfigMap: configMapCheckSum,
		})
	}
	return b.etcd.Spec.Annotations
}

func (b *stsBuilder) getVolumeClaimTemplates() []corev1.PersistentVolumeClaim {
	return []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.TypeDeref[string](b.etcd.Spec.VolumeClaimTemplate, b.etcd.Name),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: utils.TypeDeref[apiresource.Quantity](b.etcd.Spec.StorageCapacity, defaultStorageCapacity),
					},
				},
				StorageClassName: b.etcd.Spec.StorageClass,
			},
		},
	}
}

func (b *stsBuilder) getPodInitContainers() []corev1.Container {
	initContainers := make([]corev1.Container, 0, 2)
	if !b.useEtcdWrapper {
		return initContainers
	}
	initContainers = append(initContainers, corev1.Container{
		Name:            common.InitContainerNameChangePermissions,
		Image:           b.initContainerImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"sh", "-c", "--"},
		Args:            []string{fmt.Sprintf("chown -R %d:%d %s", nonRootUser, nonRootUser, common.VolumeMountPathEtcdData)},
		VolumeMounts:    []corev1.VolumeMount{b.getEtcdDataVolumeMount()},
		SecurityContext: &corev1.SecurityContext{
			RunAsGroup:   pointer.Int64(0),
			RunAsNonRoot: pointer.Bool(false),
			RunAsUser:    pointer.Int64(0),
		},
	})
	if b.etcd.IsBackupStoreEnabled() {
		if b.provider != nil && *b.provider == utils.Local {
			etcdBackupVolumeMount := b.getEtcdBackupVolumeMount()
			if etcdBackupVolumeMount != nil {
				initContainers = append(initContainers, corev1.Container{
					Name:            common.InitContainerNameChangeBackupBucketPermissions,
					Image:           b.initContainerImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"sh", "-c", "--"},
					Args:            []string{fmt.Sprintf("chown -R %d:%d /home/nonroot/%s", nonRootUser, nonRootUser, *b.etcd.Spec.Backup.Store.Container)},
					VolumeMounts:    []corev1.VolumeMount{*etcdBackupVolumeMount},
					SecurityContext: &corev1.SecurityContext{
						RunAsGroup:   pointer.Int64(0),
						RunAsNonRoot: pointer.Bool(false),
						RunAsUser:    pointer.Int64(0),
					},
				})
			}
		}
	}
	return initContainers
}

func (b *stsBuilder) getEtcdContainerVolumeMounts() []corev1.VolumeMount {
	etcdVolumeMounts := make([]corev1.VolumeMount, 0, 7)
	etcdVolumeMounts = append(etcdVolumeMounts, b.getEtcdDataVolumeMount())
	etcdVolumeMounts = append(etcdVolumeMounts, b.getEtcdContainerSecretVolumeMounts()...)
	return etcdVolumeMounts
}

func (b *stsBuilder) getBackupRestoreContainerVolumeMounts() []corev1.VolumeMount {
	brVolumeMounts := make([]corev1.VolumeMount, 0, 6)
	brVolumeMounts = append(brVolumeMounts,
		b.getEtcdDataVolumeMount(),
		corev1.VolumeMount{
			Name:      common.VolumeNameEtcdConfig,
			MountPath: etcdConfigFileMountPath,
		},
	)
	brVolumeMounts = append(brVolumeMounts, b.getBackupRestoreContainerSecretVolumeMounts()...)

	if b.etcd.IsBackupStoreEnabled() {
		etcdBackupVolumeMount := b.getEtcdBackupVolumeMount()
		if etcdBackupVolumeMount != nil {
			brVolumeMounts = append(brVolumeMounts, *etcdBackupVolumeMount)
		}
	}
	return brVolumeMounts
}

func (b *stsBuilder) getBackupRestoreContainerSecretVolumeMounts() []corev1.VolumeMount {
	secretVolumeMounts := make([]corev1.VolumeMount, 0, 3)
	if b.etcd.Spec.Backup.TLS != nil {
		secretVolumeMounts = append(secretVolumeMounts,
			corev1.VolumeMount{
				Name:      common.VolumeNameBackupRestoreServerTLS,
				MountPath: common.VolumeMountPathBackupRestoreServerTLS,
			},
		)
	}
	if b.etcd.Spec.Etcd.ClientUrlTLS != nil {
		secretVolumeMounts = append(secretVolumeMounts,
			corev1.VolumeMount{
				Name:      common.VolumeNameEtcdCA,
				MountPath: common.VolumeMountPathEtcdCA,
			},
			corev1.VolumeMount{
				Name:      common.VolumeNameEtcdClientTLS,
				MountPath: common.VolumeMountPathEtcdClientTLS,
			},
		)
	}

	return secretVolumeMounts
}

func (b *stsBuilder) getEtcdBackupVolumeMount() *corev1.VolumeMount {
	switch *b.provider {
	case utils.Local:
		if b.etcd.Spec.Backup.Store.Container != nil {
			if b.useEtcdWrapper {
				return &corev1.VolumeMount{
					Name:      common.VolumeNameLocalBackup,
					MountPath: fmt.Sprintf("/home/nonroot/%s", pointer.StringDeref(b.etcd.Spec.Backup.Store.Container, "")),
				}
			} else {
				return &corev1.VolumeMount{
					Name:      common.VolumeNameLocalBackup,
					MountPath: pointer.StringDeref(b.etcd.Spec.Backup.Store.Container, ""),
				}
			}
		}
	case utils.GCS:
		return &corev1.VolumeMount{
			Name:      common.VolumeNameProviderBackupSecret,
			MountPath: common.VolumeMountPathGCSBackupSecret,
		}
	case utils.S3, utils.ABS, utils.OSS, utils.Swift, utils.OCS:
		return &corev1.VolumeMount{
			Name:      common.VolumeNameProviderBackupSecret,
			MountPath: common.VolumeMountPathNonGCSProviderBackupSecret,
		}
	}
	return nil
}

func (b *stsBuilder) getEtcdDataVolumeMount() corev1.VolumeMount {
	volumeClaimTemplateName := utils.TypeDeref[string](b.etcd.Spec.VolumeClaimTemplate, b.etcd.Name)
	return corev1.VolumeMount{
		Name:      volumeClaimTemplateName,
		MountPath: common.VolumeMountPathEtcdData,
	}
}

func (b *stsBuilder) getEtcdContainer() corev1.Container {
	return corev1.Container{
		Name:            common.ContainerNameEtcd,
		Image:           b.etcdImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            b.getEtcdContainerCommandArgs(),
		ReadinessProbe:  b.getEtcdContainerReadinessProbe(),
		Ports: []corev1.ContainerPort{
			{
				Name:          serverPortName,
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: b.serverPort,
			},
			{
				Name:          clientPortName,
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: b.clientPort,
			},
		},
		Resources:    utils.TypeDeref[corev1.ResourceRequirements](b.etcd.Spec.Etcd.Resources, defaultResourceRequirements),
		Env:          b.getEtcdContainerEnvVars(),
		VolumeMounts: b.getEtcdContainerVolumeMounts(),
	}
}

func (b *stsBuilder) getBackupRestoreContainer() (corev1.Container, error) {
	env, err := utils.GetBackupRestoreContainerEnvVars(b.etcd.Spec.Backup.Store)
	if err != nil {
		return corev1.Container{}, err
	}
	return corev1.Container{
		Name:            common.ContainerNameEtcdBackupRestore,
		Image:           b.etcdBackupRestoreImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            b.getBackupRestoreContainerCommandArgs(),
		Ports: []corev1.ContainerPort{
			{
				Name:          serverPortName,
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: b.backupPort,
			},
		},
		Env:          env,
		Resources:    utils.TypeDeref[corev1.ResourceRequirements](b.etcd.Spec.Backup.Resources, defaultResourceRequirements),
		VolumeMounts: b.getBackupRestoreContainerVolumeMounts(),
	}, nil
}

func (b *stsBuilder) getBackupRestoreContainerCommandArgs() []string {
	commandArgs := []string{"server"}

	// Backup store related command line args
	// -----------------------------------------------------------------------------------------------------------------
	if b.etcd.IsBackupStoreEnabled() {
		commandArgs = append(commandArgs, b.getBackupStoreCommandArgs()...)
	}

	// Defragmentation command line args
	// -----------------------------------------------------------------------------------------------------------------
	if b.etcd.Spec.Etcd.DefragmentationSchedule != nil {
		commandArgs = append(commandArgs, fmt.Sprintf("--defragmentation-schedule=%s", *b.etcd.Spec.Etcd.DefragmentationSchedule))
	}
	etcdDefragTimeout := defaultEtcdDefragTimeout
	if b.etcd.Spec.Etcd.EtcdDefragTimeout != nil {
		etcdDefragTimeout = b.etcd.Spec.Etcd.EtcdDefragTimeout.Duration.String()
	}
	commandArgs = append(commandArgs, "--etcd-defrag-timeout="+etcdDefragTimeout)

	// Compaction command line args
	// -----------------------------------------------------------------------------------------------------------------
	compactionMode := defaultAutoCompactionMode
	if b.etcd.Spec.Common.AutoCompactionMode != nil {
		compactionMode = string(*b.etcd.Spec.Common.AutoCompactionMode)
	}
	commandArgs = append(commandArgs, "--auto-compaction-mode="+compactionMode)

	compactionRetention := defaultAutoCompactionRetention
	if b.etcd.Spec.Common.AutoCompactionRetention != nil {
		compactionRetention = *b.etcd.Spec.Common.AutoCompactionRetention
	}
	commandArgs = append(commandArgs, fmt.Sprintf("--auto-compaction-retention=%s", compactionRetention))

	// Client and Backup TLS command line args
	// -----------------------------------------------------------------------------------------------------------------
	if b.etcd.Spec.Etcd.ClientUrlTLS != nil {
		dataKey := utils.TypeDeref(b.etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey, "ca.crt")
		commandArgs = append(commandArgs, fmt.Sprintf("--cacert=%s/%s", common.VolumeMountPathEtcdCA, dataKey))
		commandArgs = append(commandArgs, fmt.Sprintf("--cert=%s/tls.crt", common.VolumeMountPathEtcdClientTLS))
		commandArgs = append(commandArgs, fmt.Sprintf("--key=%s/tls.key", common.VolumeMountPathEtcdClientTLS))
		commandArgs = append(commandArgs, "--insecure-transport=false")
		commandArgs = append(commandArgs, "--insecure-skip-tls-verify=false")
		commandArgs = append(commandArgs, fmt.Sprintf("--endpoints=https://%s-local:%d", b.etcd.Name, b.clientPort))
		commandArgs = append(commandArgs, fmt.Sprintf("--service-endpoints=https://%s:%d", b.etcd.GetClientServiceName(), b.clientPort))
	} else {
		commandArgs = append(commandArgs, "--insecure-transport=true")
		commandArgs = append(commandArgs, "--insecure-skip-tls-verify=true")
		commandArgs = append(commandArgs, fmt.Sprintf("--endpoints=http://%s-local:%d", b.etcd.Name, b.clientPort))
		commandArgs = append(commandArgs, fmt.Sprintf("--service-endpoints=http://%s:%d", b.etcd.GetClientServiceName(), b.clientPort))
	}
	if b.etcd.Spec.Backup.TLS != nil {
		commandArgs = append(commandArgs, fmt.Sprintf("--server-cert=%s/tls.crt", common.VolumeMountPathBackupRestoreServerTLS))
		commandArgs = append(commandArgs, fmt.Sprintf("--server-key=%s/tls.key", common.VolumeMountPathBackupRestoreServerTLS))
	}

	// Other misc command line args
	// -----------------------------------------------------------------------------------------------------------------
	commandArgs = append(commandArgs, fmt.Sprintf("--data-dir=%s/new.etcd", common.VolumeMountPathEtcdData))
	commandArgs = append(commandArgs, fmt.Sprintf("--restoration-temp-snapshots-dir=%s/restoration.temp", common.VolumeMountPathEtcdData))
	commandArgs = append(commandArgs, fmt.Sprintf("--snapstore-temp-directory=%s/temp", common.VolumeMountPathEtcdData))
	commandArgs = append(commandArgs, fmt.Sprintf("--etcd-connection-timeout=%s", defaultEtcdConnectionTimeout))
	commandArgs = append(commandArgs, "--enable-member-lease-renewal=true")

	var quota = defaultQuota
	if b.etcd.Spec.Etcd.Quota != nil {
		quota = b.etcd.Spec.Etcd.Quota.Value()
	}
	commandArgs = append(commandArgs, fmt.Sprintf("--embedded-etcd-quota-bytes=%d", quota))
	if utils.TypeDeref[bool](b.etcd.Spec.Backup.EnableProfiling, false) {
		commandArgs = append(commandArgs, "--enable-profiling=true")
	}

	heartbeatDuration := defaultHeartbeatDuration
	if b.etcd.Spec.Etcd.HeartbeatDuration != nil {
		heartbeatDuration = b.etcd.Spec.Etcd.HeartbeatDuration.Duration.String()
	}
	commandArgs = append(commandArgs, fmt.Sprintf("--k8s-heartbeat-duration=%s", heartbeatDuration))

	if b.etcd.Spec.Backup.LeaderElection != nil {
		if b.etcd.Spec.Backup.LeaderElection.EtcdConnectionTimeout != nil {
			commandArgs = append(commandArgs, fmt.Sprintf("--etcd-connection-timeout-leader-election=%s", b.etcd.Spec.Backup.LeaderElection.EtcdConnectionTimeout.Duration.String()))
		}
		if b.etcd.Spec.Backup.LeaderElection.ReelectionPeriod != nil {
			commandArgs = append(commandArgs, fmt.Sprintf("--reelection-period=%s", b.etcd.Spec.Backup.LeaderElection.ReelectionPeriod.Duration.String()))
		}
	}

	return commandArgs
}

func (b *stsBuilder) getBackupStoreCommandArgs() []string {
	var commandArgs []string

	commandArgs = append(commandArgs, "--enable-snapshot-lease-renewal=true")
	commandArgs = append(commandArgs, fmt.Sprintf("--storage-provider=%s", *b.provider))
	commandArgs = append(commandArgs, fmt.Sprintf("--store-prefix=%s", b.etcd.Spec.Backup.Store.Prefix))

	// Full snapshot command line args
	// -----------------------------------------------------------------------------------------------------------------
	commandArgs = append(commandArgs, fmt.Sprintf("--full-snapshot-lease-name=%s", b.etcd.GetFullSnapshotLeaseName()))
	if b.etcd.Spec.Backup.FullSnapshotSchedule != nil {
		commandArgs = append(commandArgs, fmt.Sprintf("--schedule=%s", *b.etcd.Spec.Backup.FullSnapshotSchedule))
	}

	// Delta snapshot command line args
	// -----------------------------------------------------------------------------------------------------------------
	commandArgs = append(commandArgs, fmt.Sprintf("--delta-snapshot-lease-name=%s", b.etcd.GetDeltaSnapshotLeaseName()))
	if b.etcd.Spec.Backup.DeltaSnapshotPeriod != nil {
		commandArgs = append(commandArgs, fmt.Sprintf("--delta-snapshot-period=%s", b.etcd.Spec.Backup.DeltaSnapshotPeriod.Duration.String()))
	}
	if b.etcd.Spec.Backup.DeltaSnapshotRetentionPeriod != nil {
		commandArgs = append(commandArgs, fmt.Sprintf("--delta-snapshot-retention-period=%s", b.etcd.Spec.Backup.DeltaSnapshotRetentionPeriod.Duration.String()))
	}
	var deltaSnapshotMemoryLimit = defaultSnapshotMemoryLimit
	if b.etcd.Spec.Backup.DeltaSnapshotMemoryLimit != nil {
		deltaSnapshotMemoryLimit = b.etcd.Spec.Backup.DeltaSnapshotMemoryLimit.Value()
	}
	commandArgs = append(commandArgs, fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapshotMemoryLimit))

	// garbage collection command line args
	// -----------------------------------------------------------------------------------------------------------------
	garbageCollectionPolicy := defaultGbcPolicy
	if b.etcd.Spec.Backup.GarbageCollectionPolicy != nil {
		garbageCollectionPolicy = string(*b.etcd.Spec.Backup.GarbageCollectionPolicy)
	}
	commandArgs = append(commandArgs, fmt.Sprintf("--garbage-collection-policy=%s", garbageCollectionPolicy))
	if garbageCollectionPolicy == "LimitBased" {
		commandArgs = append(commandArgs, fmt.Sprintf("--max-backups=%d", utils.TypeDeref[int32](b.etcd.Spec.Backup.MaxBackupsLimitBasedGC, defaultMaxBackupsLimitBasedGC)))
	}
	if b.etcd.Spec.Backup.GarbageCollectionPeriod != nil {
		commandArgs = append(commandArgs, fmt.Sprintf("--garbage-collection-period=%s", b.etcd.Spec.Backup.GarbageCollectionPeriod.Duration.String()))
	}

	// Snapshot compression and timeout command line args
	// -----------------------------------------------------------------------------------------------------------------
	if b.etcd.Spec.Backup.SnapshotCompression != nil {
		if utils.TypeDeref[bool](b.etcd.Spec.Backup.SnapshotCompression.Enabled, false) {
			commandArgs = append(commandArgs, fmt.Sprintf("--compress-snapshots=%t", *b.etcd.Spec.Backup.SnapshotCompression.Enabled))
		}
		if b.etcd.Spec.Backup.SnapshotCompression.Policy != nil {
			commandArgs = append(commandArgs, fmt.Sprintf("--compression-policy=%s", string(*b.etcd.Spec.Backup.SnapshotCompression.Policy)))
		}
	}

	etcdSnapshotTimeout := defaultEtcdSnapshotTimeout
	if b.etcd.Spec.Backup.EtcdSnapshotTimeout != nil {
		etcdSnapshotTimeout = b.etcd.Spec.Backup.EtcdSnapshotTimeout.Duration.String()
	}
	commandArgs = append(commandArgs, "--etcd-snapshot-timeout="+etcdSnapshotTimeout)

	return commandArgs
}

func (b *stsBuilder) getEtcdContainerReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:        b.getEtcdContainerReadinessHandler(),
		InitialDelaySeconds: 15,
		PeriodSeconds:       5,
		FailureThreshold:    5,
	}
}

func (b *stsBuilder) getEtcdContainerReadinessHandler() corev1.ProbeHandler {
	multiNodeCluster := b.etcd.Spec.Replicas > 1
	if multiNodeCluster && !b.useEtcdWrapper {
		return corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: b.getEtcdContainerReadinessProbeCommand(),
			},
		}
	}
	scheme := utils.IfConditionOr[corev1.URIScheme](b.etcd.Spec.Backup.TLS == nil, corev1.URISchemeHTTP, corev1.URISchemeHTTPS)
	path := utils.IfConditionOr[string](multiNodeCluster, "/readyz", "/healthz")
	port := utils.IfConditionOr[int32](multiNodeCluster, common.DefaultPortEtcdWrapper, common.DefaultPortEtcdBackupRestore)

	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path:   path,
			Port:   intstr.FromInt32(port),
			Scheme: scheme,
		},
	}
}

func (b *stsBuilder) getEtcdContainerReadinessProbeCommand() []string {
	cmdBuilder := strings.Builder{}
	cmdBuilder.WriteString("ETCDCTL_API=3 etcdctl")
	if b.etcd.Spec.Etcd.ClientUrlTLS != nil {
		dataKey := utils.TypeDeref(b.etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey, "ca.crt")
		cmdBuilder.WriteString(fmt.Sprintf(" --cacert=%s/%s", common.VolumeMountPathEtcdCA, dataKey))
		cmdBuilder.WriteString(fmt.Sprintf(" --cert=%s/tls.crt", common.VolumeMountPathEtcdClientTLS))
		cmdBuilder.WriteString(fmt.Sprintf(" --key=%s/tls.key", common.VolumeMountPathEtcdClientTLS))
		cmdBuilder.WriteString(fmt.Sprintf(" --endpoints=https://%s-local:%d", b.etcd.Name, b.clientPort))
	} else {
		cmdBuilder.WriteString(fmt.Sprintf(" --endpoints=http://%s-local:%d", b.etcd.Name, b.clientPort))
	}
	cmdBuilder.WriteString(" get foo")
	cmdBuilder.WriteString(" --consistency=l")

	return []string{
		"/bin/sh",
		"-ec",
		cmdBuilder.String(),
	}
}

func (b *stsBuilder) getEtcdContainerCommandArgs() []string {
	if !b.useEtcdWrapper {
		// safe to return an empty string array here since etcd-custom-image:v3.4.13-bootstrap-12 (as well as v3.4.26) now uses an entry point that calls bootstrap.sh
		return []string{}
	}
	commandArgs := []string{"start-etcd"}
	// TODO
	commandArgs = append(commandArgs, fmt.Sprintf("--backup-restore-host-port=%s-local:%d", b.etcd.Name, common.DefaultPortEtcdBackupRestore))
	commandArgs = append(commandArgs, fmt.Sprintf("--etcd-server-name=%s-local", b.etcd.Name))

	if b.etcd.Spec.Etcd.ClientUrlTLS == nil {
		commandArgs = append(commandArgs, "--backup-restore-tls-enabled=false")
	} else {
		commandArgs = append(commandArgs, "--backup-restore-tls-enabled=true")
		dataKey := utils.TypeDeref(b.etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey, "ca.crt")
		commandArgs = append(commandArgs, fmt.Sprintf("--backup-restore-ca-cert-bundle-path=%s/%s", common.VolumeMountPathBackupRestoreCA, dataKey))
		commandArgs = append(commandArgs, fmt.Sprintf("--etcd-client-cert-path=%s/tls.crt", common.VolumeMountPathEtcdClientTLS))
		commandArgs = append(commandArgs, fmt.Sprintf("--etcd-client-key-path=%s/tls.key", common.VolumeMountPathEtcdClientTLS))
	}
	return commandArgs
}

func (b *stsBuilder) getEtcdContainerEnvVars() []corev1.EnvVar {
	if b.useEtcdWrapper {
		return []corev1.EnvVar{}
	}
	backTLSEnabled := b.etcd.Spec.Backup.TLS != nil
	scheme := utils.IfConditionOr[string](backTLSEnabled, "https", "http")
	endpoint := fmt.Sprintf("%s://%s-local:%d", scheme, b.etcd.Name, b.backupPort)

	return []corev1.EnvVar{
		{Name: "ENABLE_TLS", Value: strconv.FormatBool(backTLSEnabled)},
		{Name: "BACKUP_ENDPOINT", Value: endpoint},
	}
}

func (b *stsBuilder) getPodSecurityContext() *corev1.PodSecurityContext {
	if !b.useEtcdWrapper {
		return nil
	}
	return &corev1.PodSecurityContext{
		RunAsGroup:   pointer.Int64(nonRootUser),
		RunAsNonRoot: pointer.Bool(true),
		RunAsUser:    pointer.Int64(nonRootUser),
		FSGroup:      pointer.Int64(nonRootUser),
	}
}

func (b *stsBuilder) getEtcdContainerSecretVolumeMounts() []corev1.VolumeMount {
	secretVolumeMounts := make([]corev1.VolumeMount, 0, 6)
	if b.etcd.Spec.Etcd.ClientUrlTLS != nil {
		secretVolumeMounts = append(secretVolumeMounts,
			corev1.VolumeMount{
				Name:      common.VolumeNameEtcdCA,
				MountPath: common.VolumeMountPathEtcdCA,
			},
			corev1.VolumeMount{
				Name:      common.VolumeNameEtcdServerTLS,
				MountPath: common.VolumeMountPathEtcdServerTLS,
			},
			corev1.VolumeMount{
				Name:      common.VolumeNameEtcdClientTLS,
				MountPath: common.VolumeMountPathEtcdClientTLS,
			},
		)
	}
	if b.etcd.Spec.Etcd.PeerUrlTLS != nil {
		secretVolumeMounts = append(secretVolumeMounts,
			corev1.VolumeMount{
				Name:      common.VolumeNameEtcdPeerCA,
				MountPath: common.VolumeMountPathEtcdPeerCA,
			},
			corev1.VolumeMount{
				Name:      common.VolumeNameEtcdPeerServerTLS,
				MountPath: common.VolumeMountPathEtcdPeerServerTLS,
			},
		)
	}
	if b.etcd.Spec.Backup.TLS != nil {
		secretVolumeMounts = append(secretVolumeMounts,
			corev1.VolumeMount{
				Name:      common.VolumeNameBackupRestoreCA,
				MountPath: common.VolumeMountPathBackupRestoreCA,
			},
		)
	}
	return secretVolumeMounts
}

// getPodVolumes gets volumes that needs to be mounted onto the etcd StatefulSet pods
func (b *stsBuilder) getPodVolumes(ctx component.OperatorContext) ([]corev1.Volume, error) {
	volumes := []corev1.Volume{
		{
			Name: common.VolumeNameEtcdConfig,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: b.etcd.GetConfigMapName(),
					},
					Items: []corev1.KeyToPath{
						{
							Key:  etcdConfigFileName,
							Path: etcdConfigFileName,
						},
					},
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
	}

	if b.etcd.Spec.Etcd.ClientUrlTLS != nil {
		volumes = append(volumes, b.getClientTLSVolumes()...)
	}
	if b.etcd.Spec.Etcd.PeerUrlTLS != nil {
		volumes = append(volumes, b.getPeerTLSVolumes()...)
	}
	if b.etcd.Spec.Backup.TLS != nil {
		volumes = append(volumes, b.getBackupRestoreTLSVolumes()...)
	}
	if b.etcd.IsBackupStoreEnabled() {
		backupVolume, err := b.getBackupVolume(ctx)
		if err != nil {
			return nil, err
		}
		if backupVolume != nil {
			volumes = append(volumes, *backupVolume)
		}
	}
	return volumes, nil
}

func (b *stsBuilder) getClientTLSVolumes() []corev1.Volume {
	clientTLSConfig := b.etcd.Spec.Etcd.ClientUrlTLS
	return []corev1.Volume{
		{
			Name: common.VolumeNameEtcdCA,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  clientTLSConfig.TLSCASecretRef.Name,
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameEtcdServerTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  clientTLSConfig.ServerTLSSecretRef.Name,
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameEtcdClientTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  clientTLSConfig.ClientTLSSecretRef.Name,
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
	}
}

func (b *stsBuilder) getPeerTLSVolumes() []corev1.Volume {
	peerTLSConfig := b.etcd.Spec.Etcd.PeerUrlTLS
	return []corev1.Volume{
		{
			Name: common.VolumeNameEtcdPeerCA,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  peerTLSConfig.TLSCASecretRef.Name,
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameEtcdPeerServerTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  peerTLSConfig.ServerTLSSecretRef.Name,
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
	}
}

func (b *stsBuilder) getBackupRestoreTLSVolumes() []corev1.Volume {
	tlsConfig := b.etcd.Spec.Backup.TLS
	return []corev1.Volume{
		{
			Name: common.VolumeNameBackupRestoreCA,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tlsConfig.TLSCASecretRef.Name,
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameBackupRestoreServerTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tlsConfig.ServerTLSSecretRef.Name,
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameBackupRestoreClientTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tlsConfig.ClientTLSSecretRef.Name,
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
	}
}

func (b *stsBuilder) getBackupVolume(ctx component.OperatorContext) (*corev1.Volume, error) {
	if b.provider == nil {
		return nil, nil
	}
	store := b.etcd.Spec.Backup.Store
	switch *b.provider {
	case utils.Local:
		hostPath, err := utils.GetHostMountPathFromSecretRef(ctx, b.client, b.logger, store, b.etcd.GetNamespace())
		if err != nil {
			return nil, fmt.Errorf("error getting host mount path for etcd: %v Err: %w", b.etcd.GetNamespaceName(), err)
		}

		hpt := corev1.HostPathDirectory
		return &corev1.Volume{
			Name: common.VolumeNameLocalBackup,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath + "/" + pointer.StringDeref(store.Container, ""),
					Type: &hpt,
				},
			},
		}, nil
	case utils.GCS, utils.S3, utils.OSS, utils.ABS, utils.Swift, utils.OCS:
		if store.SecretRef == nil {
			return nil, fmt.Errorf("etcd: %v, no secretRef configured for backup store", b.etcd.GetNamespaceName())
		}

		return &corev1.Volume{
			Name: common.VolumeNameProviderBackupSecret,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  store.SecretRef.Name,
					DefaultMode: pointer.Int32(common.ModeOwnerReadWriteGroupRead),
				},
			},
		}, nil
	}
	return nil, nil
}

func getBackupStoreProvider(etcd *druidv1alpha1.Etcd) (*string, error) {
	if !etcd.IsBackupStoreEnabled() {
		return nil, nil
	}
	provider, err := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
	if err != nil {
		return nil, err
	}
	return &provider, nil
}
