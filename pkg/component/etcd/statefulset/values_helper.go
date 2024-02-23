// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"
)

const (
	defaultBackupPort              int32 = 8080
	defaultServerPort              int32 = 2380
	defaultClientPort              int32 = 2379
	defaultWrapperPort             int32 = 9095
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
)

var defaultStorageCapacity = resource.MustParse("16Gi")

// GenerateValues generates `statefulset.Values` for the statefulset component with the given parameters.
func GenerateValues(
	etcd *druidv1alpha1.Etcd,
	clientPort, serverPort, backupPort *int32,
	etcdImage, backupImage, initContainerImage string,
	checksumAnnotations map[string]string,
	peerTLSChangedToEnabled, useEtcdWrapper bool) (*Values, error) {

	volumeClaimTemplateName := etcd.Name
	if etcd.Spec.VolumeClaimTemplate != nil && len(*etcd.Spec.VolumeClaimTemplate) != 0 {
		volumeClaimTemplateName = *etcd.Spec.VolumeClaimTemplate
	}

	values := Values{
		Name:                      etcd.Name,
		Namespace:                 etcd.Namespace,
		OwnerReference:            etcd.GetAsOwnerReference(),
		Replicas:                  etcd.Spec.Replicas,
		StatusReplicas:            etcd.Status.Replicas,
		Annotations:               utils.MergeStringMaps(checksumAnnotations, etcd.Spec.Annotations),
		Labels:                    etcd.GetDefaultLabels(),
		AdditionalPodLabels:       etcd.Spec.Labels,
		EtcdImage:                 etcdImage,
		BackupImage:               backupImage,
		InitContainerImage:        initContainerImage,
		PriorityClassName:         etcd.Spec.PriorityClassName,
		ServiceAccountName:        etcd.GetServiceAccountName(),
		Affinity:                  etcd.Spec.SchedulingConstraints.Affinity,
		TopologySpreadConstraints: etcd.Spec.SchedulingConstraints.TopologySpreadConstraints,

		EtcdResourceRequirements:   etcd.Spec.Etcd.Resources,
		BackupResourceRequirements: etcd.Spec.Backup.Resources,

		VolumeClaimTemplateName: volumeClaimTemplateName,

		FullSnapLeaseName:  etcd.GetFullSnapshotLeaseName(),
		DeltaSnapLeaseName: etcd.GetDeltaSnapshotLeaseName(),

		StorageCapacity: etcd.Spec.StorageCapacity,
		StorageClass:    etcd.Spec.StorageClass,

		ClientUrlTLS: etcd.Spec.Etcd.ClientUrlTLS,
		PeerUrlTLS:   etcd.Spec.Etcd.PeerUrlTLS,
		BackupTLS:    etcd.Spec.Backup.TLS,

		LeaderElection: etcd.Spec.Backup.LeaderElection,

		BackupStore:     etcd.Spec.Backup.Store,
		EnableProfiling: etcd.Spec.Backup.EnableProfiling,

		DeltaSnapshotPeriod:          etcd.Spec.Backup.DeltaSnapshotPeriod,
		DeltaSnapshotRetentionPeriod: etcd.Spec.Backup.DeltaSnapshotRetentionPeriod,
		DeltaSnapshotMemoryLimit:     etcd.Spec.Backup.DeltaSnapshotMemoryLimit,

		DefragmentationSchedule: etcd.Spec.Etcd.DefragmentationSchedule,
		FullSnapshotSchedule:    etcd.Spec.Backup.FullSnapshotSchedule,

		EtcdSnapshotTimeout: etcd.Spec.Backup.EtcdSnapshotTimeout,
		EtcdDefragTimeout:   etcd.Spec.Etcd.EtcdDefragTimeout,

		GarbageCollectionPolicy: etcd.Spec.Backup.GarbageCollectionPolicy,
		MaxBackupsLimitBasedGC:  etcd.Spec.Backup.MaxBackupsLimitBasedGC,
		GarbageCollectionPeriod: etcd.Spec.Backup.GarbageCollectionPeriod,

		SnapshotCompression: etcd.Spec.Backup.SnapshotCompression,
		HeartbeatDuration:   etcd.Spec.Etcd.HeartbeatDuration,

		MetricsLevel:      etcd.Spec.Etcd.Metrics,
		Quota:             etcd.Spec.Etcd.Quota,
		ClientServiceName: etcd.GetClientServiceName(),
		ClientPort:        clientPort,
		PeerServiceName:   etcd.GetPeerServiceName(),
		ServerPort:        serverPort,
		BackupPort:        backupPort,
		WrapperPort:       pointer.Int32(defaultWrapperPort),

		AutoCompactionMode:      etcd.Spec.Common.AutoCompactionMode,
		AutoCompactionRetention: etcd.Spec.Common.AutoCompactionRetention,
		ConfigMapName:           etcd.GetConfigmapName(),
		PeerTLSChangedToEnabled: peerTLSChangedToEnabled,

		UseEtcdWrapper: useEtcdWrapper,
	}

	values.EtcdCommandArgs = getEtcdCommandArgs(values)

	// Use linearizability for readiness probe so that pod is only considered ready
	// when it has an active connection to the cluster and the cluster maintains a quorum.
	values.ReadinessProbeCommand = getProbeCommand(values, linearizable)

	etcdBackupRestoreCommandArgs, err := getBackupRestoreCommandArgs(values)
	if err != nil {
		return nil, err
	}
	values.EtcdBackupRestoreCommandArgs = etcdBackupRestoreCommandArgs

	return &values, nil
}

func getEtcdCommandArgs(val Values) []string {
	if !val.UseEtcdWrapper {
		// safe to return an empty string array here since etcd-custom-image:v3.4.13-bootstrap-12 (as well as v3.4.26) now uses an entry point that calls bootstrap.sh
		return []string{}
	}
	//TODO @aaronfern: remove this feature gate when UseEtcdWrapper becomes GA
	command := []string{"" + "start-etcd"}
	command = append(command, fmt.Sprintf("--backup-restore-host-port=%s-local:8080", val.Name))
	command = append(command, fmt.Sprintf("--etcd-server-name=%s-local", val.Name))

	if val.ClientUrlTLS == nil {
		command = append(command, "--backup-restore-tls-enabled=false")
	} else {
		dataKey := "ca.crt"
		if val.ClientUrlTLS.TLSCASecretRef.DataKey != nil {
			dataKey = *val.ClientUrlTLS.TLSCASecretRef.DataKey
		}
		command = append(command, "--backup-restore-tls-enabled=true")
		command = append(command, "--etcd-client-cert-path=/var/etcd/ssl/client/client/tls.crt")
		command = append(command, "--etcd-client-key-path=/var/etcd/ssl/client/client/tls.key")
		command = append(command, fmt.Sprintf("--backup-restore-ca-cert-bundle-path=/var/etcd/ssl/client/ca/%s", dataKey))
	}

	return command
}

type consistencyLevel string

const (
	linearizable consistencyLevel = "linearizable"
	serializable consistencyLevel = "serializable"
)

func getProbeCommand(val Values, consistency consistencyLevel) []string {
	var etcdCtlCommand strings.Builder

	etcdCtlCommand.WriteString("ETCDCTL_API=3 etcdctl")

	if val.ClientUrlTLS != nil {
		dataKey := "ca.crt"
		if val.ClientUrlTLS.TLSCASecretRef.DataKey != nil {
			dataKey = *val.ClientUrlTLS.TLSCASecretRef.DataKey
		}

		etcdCtlCommand.WriteString(" --cacert=/var/etcd/ssl/client/ca/" + dataKey)
		etcdCtlCommand.WriteString(" --cert=/var/etcd/ssl/client/client/tls.crt")
		etcdCtlCommand.WriteString(" --key=/var/etcd/ssl/client/client/tls.key")
		etcdCtlCommand.WriteString(fmt.Sprintf(" --endpoints=https://%s-local:%d", val.Name, pointer.Int32Deref(val.ClientPort, defaultClientPort)))

	} else {
		etcdCtlCommand.WriteString(fmt.Sprintf(" --endpoints=http://%s-local:%d", val.Name, pointer.Int32Deref(val.ClientPort, defaultClientPort)))
	}

	etcdCtlCommand.WriteString(" get foo")

	switch consistency {
	case linearizable:
		etcdCtlCommand.WriteString(" --consistency=l")
	case serializable:
		etcdCtlCommand.WriteString(" --consistency=s")
	}

	return []string{
		"/bin/sh",
		"-ec",
		etcdCtlCommand.String(),
	}
}

func getBackupRestoreCommandArgs(val Values) ([]string, error) {
	command := []string{"server"}

	if val.BackupStore != nil {
		command = append(command, "--enable-snapshot-lease-renewal=true")
		command = append(command, "--delta-snapshot-lease-name="+val.DeltaSnapLeaseName)
		command = append(command, "--full-snapshot-lease-name="+val.FullSnapLeaseName)
	}

	if val.DefragmentationSchedule != nil {
		command = append(command, "--defragmentation-schedule="+*val.DefragmentationSchedule)
	}

	if val.FullSnapshotSchedule != nil {
		command = append(command, "--schedule="+*val.FullSnapshotSchedule)
	}

	garbageCollectionPolicy := defaultGbcPolicy
	if val.GarbageCollectionPolicy != nil {
		garbageCollectionPolicy = string(*val.GarbageCollectionPolicy)
	}

	command = append(command, "--garbage-collection-policy="+garbageCollectionPolicy)
	if garbageCollectionPolicy == "LimitBased" {
		command = append(command, "--max-backups="+fmt.Sprint(pointer.Int32Deref(val.MaxBackupsLimitBasedGC, defaultMaxBackupsLimitBasedGC)))
	}

	command = append(command, "--data-dir=/var/etcd/data/new.etcd")
	command = append(command, "--restoration-temp-snapshots-dir=/var/etcd/data/restoration.temp")

	if val.BackupStore != nil {
		store, err := utils.StorageProviderFromInfraProvider(val.BackupStore.Provider)
		if err != nil {
			return nil, err
		}
		command = append(command, "--storage-provider="+store)
		command = append(command, "--store-prefix="+string(val.BackupStore.Prefix))
	}

	var quota = defaultQuota
	if val.Quota != nil {
		quota = val.Quota.Value()
	}

	command = append(command, "--embedded-etcd-quota-bytes="+fmt.Sprint(quota))

	if pointer.BoolDeref(val.EnableProfiling, false) {
		command = append(command, "--enable-profiling=true")
	}

	if val.ClientUrlTLS != nil {
		command = append(command, "--cert=/var/etcd/ssl/client/client/tls.crt")
		command = append(command, "--key=/var/etcd/ssl/client/client/tls.key")
		command = append(command, "--cacert=/var/etcd/ssl/client/ca/"+pointer.StringDeref(val.ClientUrlTLS.TLSCASecretRef.DataKey, "ca.crt"))
		command = append(command, "--insecure-transport=false")
		command = append(command, "--insecure-skip-tls-verify=false")
		command = append(command, fmt.Sprintf("--endpoints=https://%s-local:%d", val.Name, pointer.Int32Deref(val.ClientPort, defaultClientPort)))
		command = append(command, fmt.Sprintf("--service-endpoints=https://%s:%d", val.ClientServiceName, pointer.Int32Deref(val.ClientPort, defaultClientPort)))
	} else {
		command = append(command, "--insecure-transport=true")
		command = append(command, "--insecure-skip-tls-verify=true")
		command = append(command, fmt.Sprintf("--endpoints=http://%s-local:%d", val.Name, pointer.Int32Deref(val.ClientPort, defaultClientPort)))
		command = append(command, fmt.Sprintf("--service-endpoints=http://%s:%d", val.ClientServiceName, pointer.Int32Deref(val.ClientPort, defaultClientPort)))

	}

	if val.BackupTLS != nil {
		command = append(command, "--server-cert=/var/etcd/ssl/client/server/tls.crt")
		command = append(command, "--server-key=/var/etcd/ssl/client/server/tls.key")
	}

	command = append(command, "--etcd-connection-timeout="+defaultEtcdConnectionTimeout)

	if val.DeltaSnapshotPeriod != nil {
		command = append(command, "--delta-snapshot-period="+val.DeltaSnapshotPeriod.Duration.String())
	}

	if val.DeltaSnapshotRetentionPeriod != nil {
		command = append(command, "--delta-snapshot-retention-period="+val.DeltaSnapshotRetentionPeriod.Duration.String())
	}

	var deltaSnapshotMemoryLimit = defaultSnapshotMemoryLimit
	if val.DeltaSnapshotMemoryLimit != nil {
		deltaSnapshotMemoryLimit = val.DeltaSnapshotMemoryLimit.Value()
	}

	command = append(command, "--delta-snapshot-memory-limit="+fmt.Sprint(deltaSnapshotMemoryLimit))

	if val.GarbageCollectionPeriod != nil {
		command = append(command, "--garbage-collection-period="+val.GarbageCollectionPeriod.Duration.String())
	}

	if val.SnapshotCompression != nil {
		if pointer.BoolDeref(val.SnapshotCompression.Enabled, false) {
			command = append(command, "--compress-snapshots="+fmt.Sprint(*val.SnapshotCompression.Enabled))
		}
		if val.SnapshotCompression.Policy != nil {
			command = append(command, "--compression-policy="+string(*val.SnapshotCompression.Policy))
		}
	}

	compactionMode := defaultAutoCompactionMode
	if val.AutoCompactionMode != nil {
		compactionMode = string(*val.AutoCompactionMode)
	}
	command = append(command, "--auto-compaction-mode="+compactionMode)

	compactionRetention := defaultAutoCompactionRetention
	if val.AutoCompactionRetention != nil {
		compactionRetention = *val.AutoCompactionRetention
	}
	command = append(command, "--auto-compaction-retention="+compactionRetention)

	etcdSnapshotTimeout := defaultEtcdSnapshotTimeout
	if val.EtcdSnapshotTimeout != nil {
		etcdSnapshotTimeout = val.EtcdSnapshotTimeout.Duration.String()
	}
	command = append(command, "--etcd-snapshot-timeout="+etcdSnapshotTimeout)

	etcdDefragTimeout := defaultEtcdDefragTimeout
	if val.EtcdDefragTimeout != nil {
		etcdDefragTimeout = val.EtcdDefragTimeout.Duration.String()
	}
	command = append(command, "--etcd-defrag-timeout="+etcdDefragTimeout)

	command = append(command, "--snapstore-temp-directory=/var/etcd/data/temp")
	command = append(command, "--enable-member-lease-renewal=true")

	heartbeatDuration := defaultHeartbeatDuration
	if val.HeartbeatDuration != nil {
		heartbeatDuration = val.HeartbeatDuration.Duration.String()
	}
	command = append(command, "--k8s-heartbeat-duration="+heartbeatDuration)

	if val.LeaderElection != nil {
		if val.LeaderElection.EtcdConnectionTimeout != nil {
			command = append(command, "--etcd-connection-timeout-leader-election="+val.LeaderElection.EtcdConnectionTimeout.Duration.String())
		}

		if val.LeaderElection.ReelectionPeriod != nil {
			command = append(command, "--reelection-period="+val.LeaderElection.ReelectionPeriod.Duration.String())
		}
	}

	return command, nil
}
