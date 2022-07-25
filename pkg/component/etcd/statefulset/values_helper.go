// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v.2 except as noted otherwise in the LICENSE file
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
	"fmt"
	"strconv"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"
)

var (
	defaultBackupPort              int32 = 8080
	defaultServerPort              int32 = 2380
	defaultClientPort              int32 = 2379
	defaultheartbeatDuration             = "10s"
	defaultGbcPolicy                     = "LimitBased"
	defaultAutoCompactionRetention       = "30m"
	defaultEtcdSnapshotTimeout           = "15m"
	defaultEtcdDefragTimeout             = "15m"
	defaultAutoCompactionMode            = "periodic"
	defaultEtcdConnectionTimeout         = "5m"
	defaultStorageCapacity               = resource.MustParse("16Gi")
	defaultLocalPrefix                   = "/etc/gardener/local-backupbuckets"
)

// GenerateValues generates `statefulset.Values` for the statefulset component with the given parameters.
func GenerateValues(
	etcd *druidv1alpha1.Etcd,
	clientPort, serverPort, backupPort *int32,
	etcdImage, backupImage string,
	checksumAnnotations map[string]string,
) Values {
	volumeClaimTemplateName := etcd.Name
	if etcd.Spec.VolumeClaimTemplate != nil && len(*etcd.Spec.VolumeClaimTemplate) != 0 {
		volumeClaimTemplateName = *etcd.Spec.VolumeClaimTemplate
	}

	values := Values{
		Name:                      etcd.Name,
		Namespace:                 etcd.Namespace,
		EtcdUID:                   etcd.UID,
		Replicas:                  etcd.Spec.Replicas,
		StatusReplicas:            etcd.Status.Replicas,
		Annotations:               utils.MergeStringMaps(checksumAnnotations, etcd.Spec.Annotations),
		Labels:                    etcd.Spec.Labels,
		EtcdImage:                 etcdImage,
		BackupImage:               backupImage,
		PriorityClassName:         etcd.Spec.PriorityClassName,
		ServiceName:               utils.GetPeerServiceName(etcd),
		ServiceAccountName:        utils.GetServiceAccountName(etcd),
		Affinity:                  etcd.Spec.SchedulingConstraints.Affinity,
		TopologySpreadConstraints: etcd.Spec.SchedulingConstraints.TopologySpreadConstraints,

		EtcdResources:   etcd.Spec.Etcd.Resources,
		BackupResources: etcd.Spec.Backup.Resources,

		VolumeClaimTemplateName: volumeClaimTemplateName,

		FullSnapLeaseName:  utils.GetFullSnapshotLeaseName(etcd),
		DeltaSnapLeaseName: utils.GetDeltaSnapshotLeaseName(etcd),

		StorageCapacity: etcd.Spec.StorageCapacity,
		StorageClass:    etcd.Spec.StorageClass,

		ClientUrlTLS: etcd.Spec.Etcd.ClientUrlTLS,
		PeerUrlTLS:   etcd.Spec.Etcd.PeerUrlTLS,
		BackupTLS:    etcd.Spec.Backup.TLS,

		LeaderElection: etcd.Spec.Backup.LeaderElection,

		BackupStore:     etcd.Spec.Backup.Store,
		EnableProfiling: etcd.Spec.Backup.EnableProfiling,

		DeltaSnapshotPeriod:      etcd.Spec.Backup.DeltaSnapshotPeriod,
		DeltaSnapshotMemoryLimit: etcd.Spec.Backup.DeltaSnapshotMemoryLimit,

		DefragmentationSchedule: etcd.Spec.Etcd.DefragmentationSchedule,
		FullSnapshotSchedule:    etcd.Spec.Backup.FullSnapshotSchedule,

		EtcdSnapshotTimeout: etcd.Spec.Backup.EtcdSnapshotTimeout,
		EtcdDefragTimeout:   etcd.Spec.Etcd.EtcdDefragTimeout,

		GarbageCollectionPolicy: etcd.Spec.Backup.GarbageCollectionPolicy,
		GarbageCollectionPeriod: etcd.Spec.Backup.GarbageCollectionPeriod,

		SnapshotCompression: etcd.Spec.Backup.SnapshotCompression,
		HeartbeatDuration:   etcd.Spec.Etcd.HeartbeatDuration,

		Metrics:           etcd.Spec.Etcd.Metrics,
		Quota:             etcd.Spec.Etcd.Quota,
		ClientServiceName: utils.GetClientServiceName(etcd),
		ClientPort:        clientPort,
		PeerServiceName:   utils.GetPeerServiceName(etcd),
		ServerPort:        serverPort,
		BackupPort:        backupPort,

		OwnerCheck: etcd.Spec.Backup.OwnerCheck,

		AutoCompactionMode:      etcd.Spec.Common.AutoCompactionMode,
		AutoCompactionRetention: etcd.Spec.Common.AutoCompactionRetention,
		ConfigMapName:           utils.GetConfigmapName(etcd),
	}

	values.EtcdCommand = getEtcdCommand()
	values.ReadinessProbeCommand = getReadinessProbeCommand(values)
	values.LivenessProbCommand = getLivenessProbeCommand(values)
	values.EtcdBackupCommand = getEtcdBackupCommand(values)

	return values
}

func getEtcdCommand() []string {
	command := []string{"" + "/var/etcd/bin/bootstrap.sh"}

	return command
}

func getReadinessProbeCommand(val Values) []string {
	command := []string{"" + "/usr/bin/curl"}

	protocol := "http"
	if (val.Replicas == 1 && val.BackupTLS != nil) || (val.Replicas != 1 && val.ClientUrlTLS != nil) {
		protocol = "https"
		command = append(command, "--cert")
		command = append(command, "/var/etcd/ssl/client/client/tls.crt")
		command = append(command, "--key")
		command = append(command, "/var/etcd/ssl/client/client/tls.key")
		if dataKey := val.ClientUrlTLS.TLSCASecretRef.DataKey; dataKey != nil {
			command = append(command, "--cacert")
			command = append(command, "/var/etcd/ssl/client/ca/"+*dataKey)
		}
	}

	if val.Replicas == 1 {
		command = append(command, fmt.Sprintf("%s://%s-local:%d/healthz", protocol, val.Name, pointer.Int32Deref(val.BackupPort, defaultBackupPort)))
	} else {
		command = append(command, fmt.Sprintf("%s://%s-local:%d/health", protocol, val.Name, pointer.Int32Deref(val.ClientPort, defaultClientPort)))
	}

	return command
}

func getLivenessProbeCommand(val Values) []string {
	command := []string{"" + "/bin/sh"}
	command = append(command, "-ec")
	command = append(command, "ETCDCTL_API=3")
	command = append(command, "etcdctl")

	if val.ClientUrlTLS != nil {
		command = append(command, "--cert=/var/etcd/ssl/client/client/tls.crt")
		command = append(command, "--key=/var/etcd/ssl/client/client/tls.key")

		if dataKey := val.ClientUrlTLS.TLSCASecretRef.DataKey; dataKey != nil {
			command = append(command, "--cacert=/var/etcd/ssl/client/ca/"+*dataKey)
		}
		command = append(command, fmt.Sprintf("--endpoints=https://%s-local:%d", val.Name, pointer.Int32Deref(val.ClientPort, defaultClientPort)))
	} else {
		command = append(command, fmt.Sprintf("--endpoints=http://%s-local:%d", val.Name, pointer.Int32Deref(val.ClientPort, defaultClientPort)))
	}
	command = append(command, "get")
	command = append(command, "foo")
	command = append(command, "--consistency=s")
	return command
}

func getEtcdBackupCommand(val Values) []string {
	command := []string{"" + "etcdbrctl"}
	command = append(command, "server")

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

	if val.GarbageCollectionPolicy != nil {

		gbc := string(*val.GarbageCollectionPolicy)
		command = append(command, "--garbage-collection-policy="+gbc)

		if gbc == "LimitBased" {
			command = append(command, "--max-backups=7")
		}
	} else {
		command = append(command, "--garbage-collection-policy="+defaultGbcPolicy)
		command = append(command, "--max-backups=7")
	}

	command = append(command, "--data-dir=/var/etcd/data/new.etcd")

	if val.BackupStore != nil {
		store, _ := utils.StorageProviderFromInfraProvider(val.BackupStore.Provider)
		command = append(command, "--storage-provider="+store)
		command = append(command, "--store-prefix="+string(val.BackupStore.Prefix))
	}

	var quota int64 = 8 * 1024 * 1024 * 1024 // 8Gi
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
		if dataKey := val.ClientUrlTLS.TLSCASecretRef.DataKey; dataKey != nil {
			command = append(command, "--cacert=/var/etcd/ssl/client/ca/"+*dataKey)
		}
		command = append(command, "--insecure-transport=false")
		command = append(command, "--insecure-skip-tls-verify=false")

		command = append(command, fmt.Sprintf("--endpoints=https://%s-local:%d", val.Name, pointer.Int32Deref(val.ClientPort, defaultClientPort)))

		command = append(command, "--server-cert=/var/etcd/ssl/client/server/tls.crt")
		command = append(command, "--server-key=/var/etcd/ssl/client/server/tls.key")
	} else {
		command = append(command, "--insecure-transport=true")
		command = append(command, "--insecure-skip-tls-verify=true")
		command = append(command, fmt.Sprintf("--endpoints=http://%s-local:%d", val.Name, pointer.Int32Deref(val.ClientPort, defaultClientPort)))
	}

	if val.LeaderElection != nil {
		if val.LeaderElection.EtcdConnectionTimeout != nil {
			command = append(command, "--etcd-connection-timeout-leader-election="+val.LeaderElection.EtcdConnectionTimeout.Duration.String())
		}

		if val.LeaderElection.ReelectionPeriod != nil {
			command = append(command, "--reelection-period="+val.LeaderElection.ReelectionPeriod.Duration.String())
		}
	}

	command = append(command, "--etcd-connection-timeout="+defaultEtcdConnectionTimeout)

	if val.DeltaSnapshotPeriod != nil {
		command = append(command, "--delta-snapshot-period="+val.DeltaSnapshotPeriod.Duration.String())
	}

	var deltaSnapshotMemoryLimit int64 = 100 * 1024 * 1024 // 100Mi
	if val.DeltaSnapshotMemoryLimit != nil {
		deltaSnapshotMemoryLimit = val.DeltaSnapshotMemoryLimit.Value()
	}

	command = append(command, "--delta-snapshot-memory-limit="+fmt.Sprint(deltaSnapshotMemoryLimit))

	if val.SnapshotCompression != nil {
		if pointer.BoolPtrDerefOr(val.SnapshotCompression.Enabled, false) {
			command = append(command, "--compress-snapshots="+strconv.FormatBool(pointer.BoolPtrDerefOr(val.SnapshotCompression.Enabled, false)))
		}
		if val.SnapshotCompression.Policy != nil {
			command = append(command, "--compression-policy="+string(*val.SnapshotCompression.Policy))
		}
	}

	if val.GarbageCollectionPeriod != nil {
		command = append(command, "--garbage-collection-period="+val.GarbageCollectionPeriod.Duration.String())
	}

	if val.OwnerCheck != nil {
		command = append(command, "--owner-name="+val.OwnerCheck.Name)
		command = append(command, "--owner-id="+val.OwnerCheck.ID)

		if val.OwnerCheck.Interval != nil {
			command = append(command, "--owner-check-interval="+val.OwnerCheck.Interval.Duration.String())
		}
		if val.OwnerCheck.Timeout != nil {
			command = append(command, "--owner-check-timeout="+val.OwnerCheck.Timeout.Duration.String())
		}
		if val.OwnerCheck.DNSCacheTTL != nil {
			command = append(command, "--owner-check-dns-cache-ttl="+val.OwnerCheck.DNSCacheTTL.Duration.String())
		}
	}

	if val.AutoCompactionMode != nil {
		command = append(command, "--auto-compaction-mode="+string(*val.AutoCompactionMode))
	} else {
		command = append(command, "--auto-compaction-mode="+defaultAutoCompactionMode)
	}

	if val.AutoCompactionRetention != nil {
		command = append(command, "--auto-compaction-retention="+string(*val.AutoCompactionRetention))
	} else {
		command = append(command, "--auto-compaction-retention="+defaultAutoCompactionRetention)
	}

	if val.EtcdSnapshotTimeout != nil {
		command = append(command, "--etcd-snapshot-timeout="+val.EtcdSnapshotTimeout.Duration.String())
	} else {
		command = append(command, "--etcd-snapshot-timeout="+defaultEtcdSnapshotTimeout)
	}

	if val.EtcdDefragTimeout != nil {
		command = append(command, "--etcd-defrag-timeout="+val.EtcdDefragTimeout.Duration.String())
	} else {
		command = append(command, "--etcd-defrag-timeout="+defaultEtcdDefragTimeout)
	}

	command = append(command, "--snapstore-temp-directory=/var/etcd/data/temp")
	command = append(command, "--enable-member-lease-renewal=true")
	command = append(command, "--etcd-process-name=etcd")

	if heartBeatDuration := val.HeartbeatDuration; heartBeatDuration != nil {
		command = append(command, "--k8s-heartbeat-duration="+heartBeatDuration.Duration.String())
	} else {
		command = append(command, "--k8s-heartbeat-duration="+defaultheartbeatDuration)
	}

	return command
}

func getEtcdEnvVar(val Values) []corev1.EnvVar {
	protocol := "http"

	if val.BackupTLS != nil {
		protocol = "https"
	}

	endpoint := fmt.Sprintf("%s://%s-local:%d", protocol, val.Name, pointer.Int32Deref(val.BackupPort, defaultBackupPort))

	var env []corev1.EnvVar
	env = append(env, getEnvVarFromValues("ENABLE_TLS", strconv.FormatBool(val.BackupTLS != nil)))
	env = append(env, getEnvVarFromValues("BACKUP_ENDPOINT", endpoint))

	return env
}

func getBackupRestoreEnvVar(val Values) []corev1.EnvVar {
	var env []corev1.EnvVar
	env = append(env, getEnvVarFromFields("POD_NAME", "metadata.name"))
	env = append(env, getEnvVarFromFields("POD_NAMESPACE", "metadata.namespace"))

	if val.BackupStore == nil {
		env = append(env, getEnvVarFromValues("STORAGE_CONTAINER", ""))
		return env
	}

	storeValues := val.BackupStore

	env = append(env, getEnvVarFromValues("STORAGE_CONTAINER", *storeValues.Container))

	provider, err := utils.StorageProviderFromInfraProvider(val.BackupStore.Provider)
	if err != nil {
		return env
	}

	switch provider {
	case "S3":
		env = append(env, getEnvVarFromValues("AWS_APPLICATION_CREDENTIALS", "/root/etcd-backup"))

	case "ABS":
		env = append(env, getEnvVarFromValues("AZURE_APPLICATION_CREDENTIALS", "/root/etcd-backup"))

	case "GCS":
		env = append(env, getEnvVarFromValues("GOOGLE_APPLICATION_CREDENTIALS", "/root/.gcp/serviceaccount.json"))

	case "Swift":
		env = append(env, getEnvVarFromValues("OPENSTACK_APPLICATION_CREDENTIALS", "/root/etcd-backup"))

	case "OSS":
		env = append(env, getEnvVarFromValues("ALICLOUD_APPLICATION_CREDENTIALS", "/root/etcd-backup"))

	case "ECS":
		env = append(env, getEnvVarFromSecrets("ECS_ENDPOINT", storeValues.SecretRef.Name, "endpoint"))
		env = append(env, getEnvVarFromSecrets("ECS_ACCESS_KEY_ID", storeValues.SecretRef.Name, "accessKeyID"))
		env = append(env, getEnvVarFromSecrets("ECS_SECRET_ACCESS_KEY", storeValues.SecretRef.Name, "secretAccessKey"))

	case "OCS":
		env = append(env, getEnvVarFromValues("OPENSHIFT_APPLICATION_CREDENTIALS", "/root/etcd-backup"))
	}

	return env
}

func getEnvVarFromValues(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}

func getEnvVarFromFields(name, fieldPath string) corev1.EnvVar {
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

func getBackupRestoreVolumeMounts(val Values) []corev1.VolumeMount {
	vms := []corev1.VolumeMount{
		{
			Name:      "etcd-config-file",
			MountPath: "/var/etcd/config/",
		},
	}

	vms = append(vms, corev1.VolumeMount{
		Name:      val.VolumeClaimTemplateName,
		MountPath: "/var/etcd/data",
	})

	vms = append(vms, getSecretVolumeMounts(val)...)

	if val.BackupStore == nil {
		return vms
	}

	provider, err := utils.StorageProviderFromInfraProvider(val.BackupStore.Provider)
	if err != nil {
		return vms
	}

	if provider == "Local" && val.BackupStore.Container != nil {
		vms = append(vms, corev1.VolumeMount{
			Name:      "host-storage",
			MountPath: *val.BackupStore.Container,
		})
	}

	if provider == "GCS" {
		vms = append(vms, corev1.VolumeMount{
			Name:      "etcd-backup",
			MountPath: "/root/.gcp/",
		})
	} else if provider == "S3" || provider == "ABS" || provider == "OSS" || provider == "Swift" || provider == "OCS" {
		vms = append(vms, corev1.VolumeMount{
			Name:      "etcd-backup",
			MountPath: "/root/etcd-backup/",
		})
	}

	return vms
}

func getBackupRestoreVolumes(val Values) []corev1.Volume {
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
		return vs
	}

	storeValues := val.BackupStore
	provider, err := utils.StorageProviderFromInfraProvider(storeValues.Provider)
	if err != nil {
		return vs
	}

	if provider == "Local" {
		hpt := corev1.HostPathDirectory
		vs = append(vs, corev1.Volume{
			Name: "host-storage",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: defaultLocalPrefix + "/" + *storeValues.Container,
					Type: &hpt,
				},
			},
		})
	}

	if provider == utils.GCS || provider == utils.S3 || provider == utils.OSS || provider == utils.ABS || provider == utils.Swift || provider == utils.OCS {
		vs = append(vs, corev1.Volume{
			Name: "etcd-backup",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: storeValues.SecretRef.Name,
				},
			},
		})
	}

	return vs
}

func getEtcdVolumeMounts(val Values) []corev1.VolumeMount {
	vms := []corev1.VolumeMount{
		{
			Name:      val.VolumeClaimTemplateName,
			MountPath: "/var/etcd/data/",
		},
	}

	vms = append(vms, getSecretVolumeMounts(val)...)

	return vms
}

func getSecretVolumeMounts(val Values) []corev1.VolumeMount {
	vms := []corev1.VolumeMount{}

	if val.ClientUrlTLS != nil {
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

	if val.PeerUrlTLS != nil {
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

func getStorageReq(val Values) corev1.ResourceRequirements {
	if val.StorageCapacity != nil {
		return corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: *val.StorageCapacity,
			},
		}
	}

	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceStorage: defaultStorageCapacity,
		},
	}
}

func getEtcdPorts(val Values) []corev1.ContainerPort {
	ports := []corev1.ContainerPort{}

	ports = append(ports, corev1.ContainerPort{
		Name:          "server",
		Protocol:      "TCP",
		ContainerPort: pointer.Int32Deref(val.ServerPort, defaultServerPort),
	})

	ports = append(ports, corev1.ContainerPort{
		Name:          "client",
		Protocol:      "TCP",
		ContainerPort: pointer.Int32Deref(val.ClientPort, defaultClientPort),
	})

	return ports
}

func getBackupPorts(val Values) []corev1.ContainerPort {
	ports := []corev1.ContainerPort{}

	ports = append(ports, corev1.ContainerPort{
		Name:          "server",
		Protocol:      "TCP",
		ContainerPort: pointer.Int32Deref(val.BackupPort, defaultBackupPort),
	})

	return ports
}

func getEtcdResources(val Values) corev1.ResourceRequirements {
	if val.EtcdResources != nil {
		return *val.EtcdResources
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
}

func getBackupResources(val Values) corev1.ResourceRequirements {
	if val.EtcdResources != nil {
		return *val.EtcdResources
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
}
