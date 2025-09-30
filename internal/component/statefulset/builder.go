// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druidstore "github.com/gardener/etcd-druid/internal/store"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/internal/utils/imagevector"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// defaults
// -----------------------------------------------------------------------------------------
const (
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
	rootUser                             = int64(0)
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
	provider               *string
	etcdImage              string
	etcdBackupRestoreImage string
	initContainerImage     string
	sts                    *appsv1.StatefulSet
	logger                 logr.Logger

	clientPort  int32
	serverPort  int32
	backupPort  int32
	wrapperPort int32
	// skipSetOrUpdateForbiddenFields if its true then it will set/update values to fields which are forbidden to be updated for an existing StatefulSet.
	// Updates to statefulset spec for fields other than 'replicas', 'ordinals', 'template', 'updateStrategy', 'persistentVolumeClaimRetentionPolicy' and 'minReadySeconds' are forbidden.
	// Only for a new StatefulSet should this be set to true.
	skipSetOrUpdateForbiddenFields bool
}

func newStsBuilder(client client.Client,
	logger logr.Logger,
	etcd *druidv1alpha1.Etcd,
	replicas int32,
	imageVector imagevector.ImageVector,
	skipSetOrUpdateForbiddenFields bool,
	sts *appsv1.StatefulSet) (*stsBuilder, error) {
	etcdImage, etcdBackupRestoreImage, initContainerImage, err := utils.GetEtcdImages(etcd, imageVector)
	if err != nil {
		return nil, err
	}
	provider, err := kubernetes.GetBackupStoreProvider(etcd)
	if err != nil {
		return nil, err
	}
	return &stsBuilder{
		client:                         client,
		logger:                         logger,
		etcd:                           etcd,
		replicas:                       replicas,
		provider:                       provider,
		etcdImage:                      etcdImage,
		etcdBackupRestoreImage:         etcdBackupRestoreImage,
		initContainerImage:             initContainerImage,
		sts:                            sts,
		clientPort:                     ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient),
		serverPort:                     ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer),
		backupPort:                     ptr.Deref(etcd.Spec.Backup.Port, common.DefaultPortEtcdBackupRestore),
		wrapperPort:                    ptr.Deref(etcd.Spec.Etcd.WrapperPort, common.DefaultPortEtcdWrapper),
		skipSetOrUpdateForbiddenFields: skipSetOrUpdateForbiddenFields,
	}, nil
}

// Build builds the StatefulSet for the given Etcd.
func (b *stsBuilder) Build(ctx component.OperatorContext) error {
	b.createStatefulSetObjectMeta()
	if err := b.createStatefulSetSpec(ctx); err != nil {
		return fmt.Errorf("[stsBuilder]: error in creating StatefulSet spec: %w", err)
	}
	return nil
}

func (b *stsBuilder) createStatefulSetObjectMeta() {
	b.sts.ObjectMeta = metav1.ObjectMeta{
		Name:            b.etcd.Name,
		Namespace:       b.etcd.Namespace,
		Labels:          b.getStatefulSetLabels(),
		OwnerReferences: []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(b.etcd.ObjectMeta)},
	}
}

func (b *stsBuilder) getStatefulSetLabels() map[string]string {
	stsLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameStatefulSet,
		druidv1alpha1.LabelAppNameKey:   b.etcd.Name,
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(b.etcd.ObjectMeta), stsLabels)
}

func (b *stsBuilder) createStatefulSetSpec(ctx component.OperatorContext) error {
	err := b.createPodTemplateSpec(ctx)
	b.sts.Spec.Replicas = ptr.To(utils.IfConditionOr[int32](druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(b.etcd.ObjectMeta), b.replicas, 0))
	b.logger.Info("Creating StatefulSet spec", "replicas", b.sts.Spec.Replicas, "name", b.sts.Name, "namespace", b.sts.Namespace)
	b.sts.Spec.UpdateStrategy = defaultUpdateStrategy
	if err != nil {
		return err
	}
	if !b.skipSetOrUpdateForbiddenFields {
		b.sts.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: druidv1alpha1.GetDefaultLabels(b.etcd.ObjectMeta),
		}
		b.sts.Spec.PodManagementPolicy = defaultPodManagementPolicy
		if druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(b.etcd.ObjectMeta) {
			b.sts.Spec.ServiceName = druidv1alpha1.GetPeerServiceName(b.etcd.ObjectMeta)
		}
		b.sts.Spec.VolumeClaimTemplates = b.getVolumeClaimTemplates()
	}
	return nil
}

func (b *stsBuilder) createPodTemplateSpec(ctx component.OperatorContext) error {
	podVolumes, err := b.getPodVolumes(ctx)
	if err != nil {
		return err
	}
	backupRestoreContainer, err := b.getBackupRestoreContainer()
	if err != nil {
		return err
	}
	podTemplateSpec := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			HostAliases:           b.getHostAliases(),
			ShareProcessNamespace: ptr.To(true),
			InitContainers:        b.getPodInitContainers(),
			Containers: []corev1.Container{
				b.getEtcdContainer(),
				backupRestoreContainer,
			},
			SecurityContext:           b.getPodSecurityContext(),
			Affinity:                  b.etcd.Spec.SchedulingConstraints.Affinity,
			TopologySpreadConstraints: b.etcd.Spec.SchedulingConstraints.TopologySpreadConstraints,
			Volumes:                   podVolumes,
			PriorityClassName:         ptr.Deref(b.etcd.Spec.PriorityClassName, ""),
		},
	}
	if druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(b.etcd.ObjectMeta) {
		podTemplateSpec.Spec.ServiceAccountName = druidv1alpha1.GetServiceAccountName(b.etcd.ObjectMeta)
	}
	podTemplateLabels := b.getStatefulSetPodLabels()
	selectorMatchesLabels, err := kubernetes.DoesLabelSelectorMatchLabels(b.sts.Spec.Selector, podTemplateLabels)
	if err != nil {
		return err
	}
	if !selectorMatchesLabels {
		podTemplateLabels = utils.MergeMaps(podTemplateLabels, b.sts.Spec.Template.Labels)
	}
	podTemplateSpec.ObjectMeta = metav1.ObjectMeta{
		Labels:      podTemplateLabels,
		Annotations: b.getPodTemplateAnnotations(ctx),
	}
	b.sts.Spec.Template = podTemplateSpec
	return nil
}

func (b *stsBuilder) getStatefulSetPodLabels() map[string]string {
	return utils.MergeMaps(
		b.etcd.Spec.Labels,
		b.getStatefulSetLabels())
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
	if druidconfigv1alpha1.DefaultFeatureGates.IsEnabled(druidconfigv1alpha1.AllowEmptyDir) {
		// If EmptyDirVolumeSource is specified, emptyDir volumes are used for the etcd pods instead of dynamic provisioning through storage classes.
		if b.etcd.Spec.EmptyDirVolumeSource != nil {
			return nil
		}
	}

	pvcClaim := []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: ptr.Deref(b.etcd.Spec.VolumeClaimTemplate, b.etcd.Name),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: ptr.Deref(b.etcd.Spec.StorageCapacity, defaultStorageCapacity),
					},
				},
			},
		},
	}

	if storageClassName := ptr.Deref(b.etcd.Spec.StorageClass, ""); storageClassName != "" {
		pvcClaim[0].Spec.StorageClassName = &storageClassName
	}

	return pvcClaim
}

func (b *stsBuilder) getPodInitContainers() []corev1.Container {
	if !b.etcd.IsBackupStoreEnabled() || b.provider == nil || *b.provider != druidstore.Local || ptr.Deref(b.etcd.Spec.RunAsRoot, false) || b.getEtcdBackupVolumeMount() == nil {
		return nil
	}

	etcdBackupVolumeMount := b.getEtcdBackupVolumeMount()
	if etcdBackupVolumeMount == nil {
		return nil
	}

	return []corev1.Container{{
		Name:            common.InitContainerNameChangeBackupBucketPermissions,
		Image:           b.initContainerImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"sh", "-c", "--"},
		Args:            []string{fmt.Sprintf("chown -R %d:%d %s", nonRootUser, nonRootUser, kubernetes.MountPathLocalStore(b.etcd, b.provider))},
		VolumeMounts:    []corev1.VolumeMount{*etcdBackupVolumeMount},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			RunAsGroup:               ptr.To[int64](0),
			RunAsNonRoot:             ptr.To(false),
			RunAsUser:                ptr.To[int64](0),
		},
	}}
}

func (b *stsBuilder) getEtcdContainerVolumeMounts() []corev1.VolumeMount {
	etcdVolumeMounts := make([]corev1.VolumeMount, 0, 7)
	etcdVolumeMounts = append(etcdVolumeMounts, b.getEtcdDataVolumeMount())
	etcdVolumeMounts = append(etcdVolumeMounts, getEtcdContainerSecretVolumeMounts(b.etcd)...)
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
	brVolumeMounts = append(brVolumeMounts, getBackupRestoreContainerSecretVolumeMounts(b.etcd)...)

	if b.etcd.IsBackupStoreEnabled() {
		etcdBackupVolumeMount := b.getEtcdBackupVolumeMount()
		if etcdBackupVolumeMount != nil {
			brVolumeMounts = append(brVolumeMounts, *etcdBackupVolumeMount)
		}
	}
	return brVolumeMounts
}

func getBackupRestoreContainerSecretVolumeMounts(etcd *druidv1alpha1.Etcd) []corev1.VolumeMount {
	secretVolumeMounts := make([]corev1.VolumeMount, 0, 3)
	if etcd.Spec.Backup.TLS != nil {
		secretVolumeMounts = append(secretVolumeMounts,
			corev1.VolumeMount{
				Name:      common.VolumeNameBackupRestoreServerTLS,
				MountPath: common.VolumeMountPathBackupRestoreServerTLS,
			},
		)
	}
	if etcd.Spec.Etcd.ClientUrlTLS != nil {
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
	case druidstore.Local:
		if b.etcd.Spec.Backup.Store.Container != nil {
			return &corev1.VolumeMount{
				Name:      common.VolumeNameLocalBackup,
				MountPath: kubernetes.MountPathLocalStore(b.etcd, b.provider),
			}
		}
	case druidstore.GCS:
		return &corev1.VolumeMount{
			Name:      common.VolumeNameProviderBackupSecret,
			MountPath: common.VolumeMountPathGCSBackupSecret,
		}
	case druidstore.S3, druidstore.ABS, druidstore.OSS, druidstore.Swift, druidstore.OCS:
		return &corev1.VolumeMount{
			Name:      common.VolumeNameProviderBackupSecret,
			MountPath: common.VolumeMountPathNonGCSProviderBackupSecret,
		}
	}
	return nil
}

func (b *stsBuilder) getEtcdDataVolumeMount() corev1.VolumeMount {
	volumeMountName := ptr.Deref(b.etcd.Spec.VolumeClaimTemplate, b.etcd.Name)
	if druidconfigv1alpha1.DefaultFeatureGates.IsEnabled(druidconfigv1alpha1.AllowEmptyDir) {
		if b.etcd.Spec.EmptyDirVolumeSource != nil {
			volumeMountName = b.generateEmptyDirVolumeName()
		}
	}
	return corev1.VolumeMount{
		Name:      volumeMountName,
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
		Resources: ptr.Deref(b.etcd.Spec.Etcd.Resources, defaultResourceRequirements),
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
		},
		Env:          b.getEtcdContainerEnvVars(),
		VolumeMounts: b.getEtcdContainerVolumeMounts(),
	}
}

func (b *stsBuilder) getBackupRestoreContainer() (corev1.Container, error) {
	env, err := utils.GetBackupRestoreContainerEnvVars(b.etcd.Spec.Backup.Store)
	if err != nil {
		return corev1.Container{}, err
	}
	providerEnv, err := druidstore.GetProviderEnvVars(b.etcd.Spec.Backup.Store)
	if err != nil {
		return corev1.Container{}, err
	}
	env = append(env, providerEnv...)

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
		Env:       env,
		Resources: ptr.Deref(b.etcd.Spec.Backup.Resources, defaultResourceRequirements),
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
		},
		VolumeMounts: b.getBackupRestoreContainerVolumeMounts(),
	}, nil
}

func (b *stsBuilder) getBackupRestoreContainerCommandArgs() []string {
	commandArgs := []string{"server"}
	commandArgs = append(commandArgs, fmt.Sprintf("--server-port=%d", b.backupPort))

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
		dataKey := ptr.Deref(b.etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey, "ca.crt")
		commandArgs = append(commandArgs, fmt.Sprintf("--cacert=%s/%s", common.VolumeMountPathEtcdCA, dataKey))
		commandArgs = append(commandArgs, fmt.Sprintf("--cert=%s/tls.crt", common.VolumeMountPathEtcdClientTLS))
		commandArgs = append(commandArgs, fmt.Sprintf("--key=%s/tls.key", common.VolumeMountPathEtcdClientTLS))
		commandArgs = append(commandArgs, "--insecure-transport=false")
		commandArgs = append(commandArgs, "--insecure-skip-tls-verify=false")
		commandArgs = append(commandArgs, fmt.Sprintf("--endpoints=https://%s-local:%d", b.etcd.Name, b.clientPort))
		if druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(b.etcd.ObjectMeta) {
			commandArgs = append(commandArgs, fmt.Sprintf("--service-endpoints=https://%s:%d", druidv1alpha1.GetClientServiceName(b.etcd.ObjectMeta), b.clientPort))
		}
	} else {
		commandArgs = append(commandArgs, "--insecure-transport=true")
		commandArgs = append(commandArgs, "--insecure-skip-tls-verify=true")
		commandArgs = append(commandArgs, fmt.Sprintf("--endpoints=http://%s-local:%d", b.etcd.Name, b.clientPort))
		if druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(b.etcd.ObjectMeta) {
			commandArgs = append(commandArgs, fmt.Sprintf("--service-endpoints=http://%s:%d", druidv1alpha1.GetClientServiceName(b.etcd.ObjectMeta), b.clientPort))
		}
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
	commandArgs = append(commandArgs, "--use-etcd-wrapper=true")
	if druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(b.etcd.ObjectMeta) {
		commandArgs = append(commandArgs, "--enable-member-lease-renewal=true")
		heartbeatDuration := defaultHeartbeatDuration
		if b.etcd.Spec.Etcd.HeartbeatDuration != nil {
			heartbeatDuration = b.etcd.Spec.Etcd.HeartbeatDuration.Duration.String()
		}
		commandArgs = append(commandArgs, fmt.Sprintf("--k8s-heartbeat-duration=%s", heartbeatDuration))
	}

	var quota = defaultQuota
	if b.etcd.Spec.Etcd.Quota != nil {
		quota = b.etcd.Spec.Etcd.Quota.Value()
	}
	commandArgs = append(commandArgs, fmt.Sprintf("--embedded-etcd-quota-bytes=%d", quota))
	if ptr.Deref(b.etcd.Spec.Backup.EnableProfiling, false) {
		commandArgs = append(commandArgs, "--enable-profiling=true")
	}

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

	if druidv1alpha1.IsEtcdRuntimeComponentCreationEnabled(b.etcd.ObjectMeta) {
		commandArgs = append(commandArgs, "--enable-snapshot-lease-renewal=true")
		commandArgs = append(commandArgs, fmt.Sprintf("--full-snapshot-lease-name=%s", druidv1alpha1.GetFullSnapshotLeaseName(b.etcd.ObjectMeta)))
		commandArgs = append(commandArgs, fmt.Sprintf("--delta-snapshot-lease-name=%s", druidv1alpha1.GetDeltaSnapshotLeaseName(b.etcd.ObjectMeta)))
	}
	commandArgs = append(commandArgs, fmt.Sprintf("--storage-provider=%s", *b.provider))
	commandArgs = append(commandArgs, fmt.Sprintf("--store-prefix=%s", b.etcd.Spec.Backup.Store.Prefix))

	// Full snapshot command line args
	// -----------------------------------------------------------------------------------------------------------------
	if b.etcd.Spec.Backup.FullSnapshotSchedule != nil {
		commandArgs = append(commandArgs, fmt.Sprintf("--schedule=%s", *b.etcd.Spec.Backup.FullSnapshotSchedule))
	}

	// Delta snapshot command line args
	// -----------------------------------------------------------------------------------------------------------------
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
		commandArgs = append(commandArgs, fmt.Sprintf("--max-backups=%d", ptr.Deref(b.etcd.Spec.Backup.MaxBackupsLimitBasedGC, defaultMaxBackupsLimitBasedGC)))
	}
	if b.etcd.Spec.Backup.GarbageCollectionPeriod != nil {
		commandArgs = append(commandArgs, fmt.Sprintf("--garbage-collection-period=%s", b.etcd.Spec.Backup.GarbageCollectionPeriod.Duration.String()))
	}

	// Snapshot compression and timeout command line args
	// -----------------------------------------------------------------------------------------------------------------
	if b.etcd.Spec.Backup.SnapshotCompression != nil {
		if ptr.Deref(b.etcd.Spec.Backup.SnapshotCompression.Enabled, false) {
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

	scheme := utils.IfConditionOr(b.etcd.Spec.Backup.TLS == nil, corev1.URISchemeHTTP, corev1.URISchemeHTTPS)
	path := utils.IfConditionOr(multiNodeCluster, "/readyz", "/healthz")
	port := utils.IfConditionOr(multiNodeCluster, b.wrapperPort, b.backupPort)

	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path:   path,
			Port:   intstr.FromInt32(port),
			Scheme: scheme,
		},
	}
}

func (b *stsBuilder) getEtcdContainerCommandArgs() []string {
	commandArgs := []string{"start-etcd"}
	commandArgs = append(commandArgs, fmt.Sprintf("--backup-restore-host-port=%s-local:%d", b.etcd.Name, b.backupPort))
	commandArgs = append(commandArgs, fmt.Sprintf("--etcd-server-name=%s-local", b.etcd.Name))

	if b.etcd.Spec.Backup.TLS == nil {
		commandArgs = append(commandArgs, "--backup-restore-tls-enabled=false")
	} else {
		commandArgs = append(commandArgs, "--backup-restore-tls-enabled=true")
		dataKey := ptr.Deref(b.etcd.Spec.Backup.TLS.TLSCASecretRef.DataKey, "ca.crt")
		commandArgs = append(commandArgs, fmt.Sprintf("--backup-restore-ca-cert-bundle-path=%s/%s", common.VolumeMountPathBackupRestoreCA, dataKey))
	}
	if b.etcd.Spec.Etcd.ClientUrlTLS != nil {
		commandArgs = append(commandArgs, fmt.Sprintf("--etcd-client-cert-path=%s/tls.crt", common.VolumeMountPathEtcdClientTLS))
		commandArgs = append(commandArgs, fmt.Sprintf("--etcd-client-key-path=%s/tls.key", common.VolumeMountPathEtcdClientTLS))
	}
	if port := b.clientPort; port != 0 {
		commandArgs = append(commandArgs, fmt.Sprintf("--etcd-client-port=%d", port))
	}
	if port := b.wrapperPort; port != 0 {
		commandArgs = append(commandArgs, fmt.Sprintf("--etcd-wrapper-port=%d", port))
	}
	return commandArgs
}

func (b *stsBuilder) getEtcdContainerEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{}
}

func (b *stsBuilder) getPodSecurityContext() *corev1.PodSecurityContext {
	if ptr.Deref(b.etcd.Spec.RunAsRoot, false) {
		return &corev1.PodSecurityContext{
			RunAsGroup:   ptr.To[int64](rootUser),
			RunAsNonRoot: ptr.To(false),
			RunAsUser:    ptr.To[int64](rootUser),
			FSGroup:      ptr.To[int64](rootUser),
		}
	}

	return &corev1.PodSecurityContext{
		RunAsGroup:   ptr.To[int64](nonRootUser),
		RunAsNonRoot: ptr.To(true),
		RunAsUser:    ptr.To[int64](nonRootUser),
		FSGroup:      ptr.To[int64](nonRootUser),
	}
}

func getEtcdContainerSecretVolumeMounts(etcd *druidv1alpha1.Etcd) []corev1.VolumeMount {
	secretVolumeMounts := make([]corev1.VolumeMount, 0, 6)
	if etcd.Spec.Etcd.ClientUrlTLS != nil {
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
	secretVolumeMounts = append(secretVolumeMounts, getEtcdContainerPeerVolumeMounts(etcd)...)
	if etcd.Spec.Backup.TLS != nil {
		secretVolumeMounts = append(secretVolumeMounts,
			corev1.VolumeMount{
				Name:      common.VolumeNameBackupRestoreCA,
				MountPath: common.VolumeMountPathBackupRestoreCA,
			},
		)
	}
	return secretVolumeMounts
}

func getEtcdContainerPeerVolumeMounts(etcd *druidv1alpha1.Etcd) []corev1.VolumeMount {
	peerTLSVolMounts := make([]corev1.VolumeMount, 0, 2)
	if etcd.Spec.Etcd.PeerUrlTLS != nil {
		peerTLSVolMounts = append(peerTLSVolMounts,
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
	return peerTLSVolMounts
}

// getPodVolumes gets volumes that needs to be mounted onto the etcd StatefulSet pods
func (b *stsBuilder) getPodVolumes(ctx component.OperatorContext) ([]corev1.Volume, error) {
	volumes := []corev1.Volume{
		{
			Name: common.VolumeNameEtcdConfig,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: druidv1alpha1.GetConfigMapName(b.etcd.ObjectMeta),
					},
					Items: []corev1.KeyToPath{
						{
							Key:  common.EtcdConfigFileName, // etcd.conf.yaml key in the configmap, which contains the config for the etcd as yaml
							Path: common.EtcdConfigFileName, // sub-path under the volume mount path, to store the contents of configmap key etcd.conf.yaml
						},
					},
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
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
	if druidconfigv1alpha1.DefaultFeatureGates.IsEnabled(druidconfigv1alpha1.AllowEmptyDir) {
		if b.etcd.Spec.EmptyDirVolumeSource != nil {
			emptyDirVolume := corev1.Volume{
				Name: b.generateEmptyDirVolumeName(),
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium:    b.etcd.Spec.EmptyDirVolumeSource.Medium,
						SizeLimit: ptr.Deref(&b.etcd.Spec.EmptyDirVolumeSource.SizeLimit, &defaultStorageCapacity),
					},
				},
			}
			volumes = append(volumes, emptyDirVolume)
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
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameEtcdServerTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  clientTLSConfig.ServerTLSSecretRef.Name,
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameEtcdClientTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  clientTLSConfig.ClientTLSSecretRef.Name,
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
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
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameEtcdPeerServerTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  peerTLSConfig.ServerTLSSecretRef.Name,
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
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
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameBackupRestoreServerTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tlsConfig.ServerTLSSecretRef.Name,
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
				},
			},
		},
		{
			Name: common.VolumeNameBackupRestoreClientTLS,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tlsConfig.ClientTLSSecretRef.Name,
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
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
	case druidstore.Local:
		hostPath, err := druidstore.GetHostMountPathFromSecretRef(ctx, b.client, b.logger, store, b.etcd.GetNamespace())
		if err != nil {
			return nil, fmt.Errorf("error getting host mount path for etcd: %v Err: %w", druidv1alpha1.GetNamespaceName(b.etcd.ObjectMeta), err)
		}

		hpt := corev1.HostPathDirectoryOrCreate
		return &corev1.Volume{
			Name: common.VolumeNameLocalBackup,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath + "/" + ptr.Deref(store.Container, ""),
					Type: &hpt,
				},
			},
		}, nil
	case druidstore.GCS, druidstore.S3, druidstore.OSS, druidstore.ABS, druidstore.Swift, druidstore.OCS:
		if store.SecretRef == nil {
			return nil, fmt.Errorf("etcd: %v, no secretRef configured for backup store", druidv1alpha1.GetNamespaceName(b.etcd.ObjectMeta))
		}

		return &corev1.Volume{
			Name: common.VolumeNameProviderBackupSecret,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  store.SecretRef.Name,
					DefaultMode: ptr.To(common.ModeOwnerReadWriteGroupRead),
				},
			},
		}, nil
	}
	return nil, nil
}

func (b *stsBuilder) generateEmptyDirVolumeName() string {
	// emptydir has to be all lower case to conform with RFC 1123 label format
	return fmt.Sprintf("%s-emptydir", b.etcd.Name)
}
