package statefulset

import (
	"fmt"
	"strconv"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

type consistencyLevel string

const (
	linearizable                   consistencyLevel = "linearizable"
	serializable                   consistencyLevel = "serializable"
	defaultBackupPort              int32            = 8080
	defaultServerPort              int32            = 2380
	defaultClientPort              int32            = 2379
	defaultWrapperPort             int32            = 9095
	defaultQuota                   int64            = 8 * 1024 * 1024 * 1024 // 8Gi
	defaultSnapshotMemoryLimit     int64            = 100 * 1024 * 1024      // 100Mi
	defaultHeartbeatDuration                        = "10s"
	defaultGbcPolicy                                = "LimitBased"
	defaultAutoCompactionRetention                  = "30m"
	defaultEtcdSnapshotTimeout                      = "15m"
	defaultEtcdDefragTimeout                        = "15m"
	defaultAutoCompactionMode                       = "periodic"
	defaultEtcdConnectionTimeout                    = "5m"
)

var defaultStorageCapacity = apiresource.MustParse("16Gi")
var defaultResourceRequirements = corev1.ResourceRequirements{
	Requests: corev1.ResourceList{
		corev1.ResourceCPU:    apiresource.MustParse("50m"),
		corev1.ResourceMemory: apiresource.MustParse("128Mi"),
	},
}

func extractObjectMetaFromEtcd(etcd *druidv1alpha1.Etcd) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            etcd.Name,
		Namespace:       etcd.Namespace,
		Labels:          etcd.GetDefaultLabels(),
		OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
	}
}
func extractPodTemplateObjectMetaFromEtcd(etcd *druidv1alpha1.Etcd) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels:      utils.MergeStringMaps(make(map[string]string), etcd.Spec.Labels, etcd.GetDefaultLabels()),
		Annotations: etcd.Spec.Annotations,
	}
}

func getEtcdCommandArgs(useEtcdWrapper bool, etcd *druidv1alpha1.Etcd) []string {
	if !useEtcdWrapper {
		// safe to return an empty string array here since etcd-custom-image:v3.4.13-bootstrap-12 (as well as v3.4.26) now uses an entry point that calls bootstrap.sh
		return []string{}
	}
	//TODO @aaronfern: remove this feature gate when UseEtcdWrapper becomes GA
	command := []string{"" + "start-etcd"}
	command = append(command, fmt.Sprintf("--backup-restore-host-port=%s-local:8080", etcd.Name))
	command = append(command, fmt.Sprintf("--etcd-server-name=%s-local", etcd.Name))

	clientURLTLS := etcd.Spec.Etcd.ClientUrlTLS
	if clientURLTLS == nil {
		command = append(command, "--backup-restore-tls-enabled=false")
	} else {
		dataKey := "ca.crt"
		if clientURLTLS.TLSCASecretRef.DataKey != nil {
			dataKey = *clientURLTLS.TLSCASecretRef.DataKey
		}
		command = append(command, "--backup-restore-tls-enabled=true")
		command = append(command, "--etcd-client-cert-path=/var/etcd/ssl/client/client/tls.crt")
		command = append(command, "--etcd-client-key-path=/var/etcd/ssl/client/client/tls.key")
		command = append(command, fmt.Sprintf("--backup-restore-ca-cert-bundle-path=/var/etcd/ssl/client/ca/%s", dataKey))
	}

	return command
}

func getReadinessHandler(useEtcdWrapper bool, etcd *druidv1alpha1.Etcd) corev1.ProbeHandler {
	if etcd.Spec.Replicas > 1 {
		// TODO(timuthy): Special handling for multi-node etcd can be removed as soon as
		// etcd-backup-restore supports `/healthz` for etcd followers, see https://github.com/gardener/etcd-backup-restore/pull/491.
		return getReadinessHandlerForMultiNode(useEtcdWrapper, etcd)
	}
	return getReadinessHandlerForSingleNode(etcd)
}

func getReadinessHandlerForSingleNode(etcd *druidv1alpha1.Etcd) corev1.ProbeHandler {
	scheme := corev1.URISchemeHTTPS
	if etcd.Spec.Backup.TLS == nil {
		scheme = corev1.URISchemeHTTP
	}

	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path:   "/healthz",
			Port:   intstr.FromInt(int(defaultBackupPort)),
			Scheme: scheme,
		},
	}
}

func getReadinessHandlerForMultiNode(useEtcdWrapper bool, etcd *druidv1alpha1.Etcd) corev1.ProbeHandler {
	if useEtcdWrapper {
		//TODO @aaronfern: remove this feature gate when UseEtcdWrapper becomes GA
		scheme := corev1.URISchemeHTTPS
		if etcd.Spec.Backup.TLS == nil {
			scheme = corev1.URISchemeHTTP
		}

		return corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/readyz",
				Port:   intstr.FromInt(int(defaultWrapperPort)),
				Scheme: scheme,
			},
		}
	}

	return corev1.ProbeHandler{
		Exec: &corev1.ExecAction{
			Command: getProbeCommand(etcd, linearizable),
		},
	}
}

func getEtcdPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "server",
			Protocol:      "TCP",
			ContainerPort: defaultServerPort,
		},
		{
			Name:          "client",
			Protocol:      "TCP",
			ContainerPort: defaultClientPort,
		},
	}
}

func getProbeCommand(etcd *druidv1alpha1.Etcd, consistency consistencyLevel) []string {
	var etcdCtlCommand strings.Builder

	etcdCtlCommand.WriteString("ETCDCTL_API=3 etcdctl")

	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		dataKey := "ca.crt"
		if etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey != nil {
			dataKey = *etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey
		}

		etcdCtlCommand.WriteString(" --cacert=/var/etcd/ssl/client/ca/" + dataKey)
		etcdCtlCommand.WriteString(" --cert=/var/etcd/ssl/client/client/tls.crt")
		etcdCtlCommand.WriteString(" --key=/var/etcd/ssl/client/client/tls.key")
		etcdCtlCommand.WriteString(fmt.Sprintf(" --endpoints=https://%s-local:%d", etcd.Name, defaultClientPort))

	} else {
		etcdCtlCommand.WriteString(fmt.Sprintf(" --endpoints=http://%s-local:%d", etcd.Name, defaultClientPort))
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

func getEtcdResources(etcd *druidv1alpha1.Etcd) corev1.ResourceRequirements {
	if etcd.Spec.Etcd.Resources != nil {
		return *etcd.Spec.Etcd.Resources
	}

	return defaultResourceRequirements
}

func getEtcdEnvVars(etcd *druidv1alpha1.Etcd) []corev1.EnvVar {
	protocol := "http"
	if etcd.Spec.Backup.TLS != nil {
		protocol = "https"
	}

	endpoint := fmt.Sprintf("%s://%s-local:%d", protocol, etcd.Name, defaultBackupPort)

	return []corev1.EnvVar{
		getEnvVarFromValue("ENABLE_TLS", strconv.FormatBool(etcd.Spec.Backup.TLS != nil)),
		getEnvVarFromValue("BACKUP_ENDPOINT", endpoint),
	}
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

func getEtcdVolumeMounts(etcd *druidv1alpha1.Etcd) []corev1.VolumeMount {
	volumeClaimTemplateName := etcd.Name
	if etcd.Spec.VolumeClaimTemplate != nil && len(*etcd.Spec.VolumeClaimTemplate) != 0 {
		volumeClaimTemplateName = *etcd.Spec.VolumeClaimTemplate
	}

	vms := []corev1.VolumeMount{
		{
			Name:      volumeClaimTemplateName,
			MountPath: "/var/etcd/data/",
		},
	}

	vms = append(vms, getSecretVolumeMounts(etcd.Spec.Etcd.ClientUrlTLS, etcd.Spec.Etcd.PeerUrlTLS)...)

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

func getBackupRestoreCommandArgs(etcd *druidv1alpha1.Etcd) []string {
	command := []string{"server"}

	if etcd.Spec.Backup.Store != nil {
		command = append(command, "--enable-snapshot-lease-renewal=true")
		command = append(command, "--delta-snapshot-lease-name="+etcd.GetDeltaSnapshotLeaseName())
		command = append(command, "--full-snapshot-lease-name="+etcd.GetFullSnapshotLeaseName())
	}

	if etcd.Spec.Etcd.DefragmentationSchedule != nil {
		command = append(command, "--defragmentation-schedule="+*etcd.Spec.Etcd.DefragmentationSchedule)
	}

	if etcd.Spec.Backup.FullSnapshotSchedule != nil {
		command = append(command, "--schedule="+*etcd.Spec.Backup.FullSnapshotSchedule)
	}

	garbageCollectionPolicy := defaultGbcPolicy
	if etcd.Spec.Backup.GarbageCollectionPolicy != nil {
		garbageCollectionPolicy = string(*etcd.Spec.Backup.GarbageCollectionPolicy)
	}

	command = append(command, "--garbage-collection-policy="+garbageCollectionPolicy)
	if garbageCollectionPolicy == "LimitBased" {
		command = append(command, "--max-backups=7")
	}

	command = append(command, "--data-dir=/var/etcd/data/new.etcd")
	command = append(command, "--restoration-temp-snapshots-dir=/var/etcd/data/restoration.temp")

	if etcd.Spec.Backup.Store != nil {
		store, err := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
		if err != nil {
			return nil
		}
		command = append(command, "--storage-provider="+store)
		command = append(command, "--store-prefix="+string(etcd.Spec.Backup.Store.Prefix))
	}

	var quota = defaultQuota
	if etcd.Spec.Etcd.Quota != nil {
		quota = etcd.Spec.Etcd.Quota.Value()
	}

	command = append(command, "--embedded-etcd-quota-bytes="+fmt.Sprint(quota))

	if pointer.BoolDeref(etcd.Spec.Backup.EnableProfiling, false) {
		command = append(command, "--enable-profiling=true")
	}

	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		command = append(command, "--cert=/var/etcd/ssl/client/client/tls.crt")
		command = append(command, "--key=/var/etcd/ssl/client/client/tls.key")
		command = append(command, "--cacert=/var/etcd/ssl/client/ca/"+pointer.StringDeref(etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey, "ca.crt"))
		command = append(command, "--insecure-transport=false")
		command = append(command, "--insecure-skip-tls-verify=false")
		command = append(command, fmt.Sprintf("--endpoints=https://%s-local:%d", etcd.Name, defaultClientPort))
		command = append(command, fmt.Sprintf("--service-endpoints=https://%s:%d", etcd.GetClientServiceName(), defaultClientPort))
	} else {
		command = append(command, "--insecure-transport=true")
		command = append(command, "--insecure-skip-tls-verify=true")
		command = append(command, fmt.Sprintf("--endpoints=http://%s-local:%d", etcd.Name, defaultClientPort))
		command = append(command, fmt.Sprintf("--service-endpoints=http://%s:%d", etcd.GetClientServiceName(), defaultClientPort))

	}

	if etcd.Spec.Backup.TLS != nil {
		command = append(command, "--server-cert=/var/etcd/ssl/client/server/tls.crt")
		command = append(command, "--server-key=/var/etcd/ssl/client/server/tls.key")
	}

	command = append(command, "--etcd-connection-timeout="+defaultEtcdConnectionTimeout)

	if etcd.Spec.Backup.DeltaSnapshotPeriod != nil {
		command = append(command, "--delta-snapshot-period="+etcd.Spec.Backup.DeltaSnapshotPeriod.Duration.String())
	}

	if etcd.Spec.Backup.DeltaSnapshotRetentionPeriod != nil {
		command = append(command, "--delta-snapshot-retention-period="+etcd.Spec.Backup.DeltaSnapshotRetentionPeriod.Duration.String())
	}

	var deltaSnapshotMemoryLimit = defaultSnapshotMemoryLimit
	if etcd.Spec.Backup.DeltaSnapshotMemoryLimit != nil {
		deltaSnapshotMemoryLimit = etcd.Spec.Backup.DeltaSnapshotMemoryLimit.Value()
	}

	command = append(command, "--delta-snapshot-memory-limit="+fmt.Sprint(deltaSnapshotMemoryLimit))

	if etcd.Spec.Backup.GarbageCollectionPeriod != nil {
		command = append(command, "--garbage-collection-period="+etcd.Spec.Backup.GarbageCollectionPeriod.Duration.String())
	}

	if etcd.Spec.Backup.SnapshotCompression != nil {
		if pointer.BoolDeref(etcd.Spec.Backup.SnapshotCompression.Enabled, false) {
			command = append(command, "--compress-snapshots="+fmt.Sprint(*etcd.Spec.Backup.SnapshotCompression.Enabled))
		}
		if etcd.Spec.Backup.SnapshotCompression.Policy != nil {
			command = append(command, "--compression-policy="+string(*etcd.Spec.Backup.SnapshotCompression.Policy))
		}
	}

	compactionMode := defaultAutoCompactionMode
	if etcd.Spec.Common.AutoCompactionMode != nil {
		compactionMode = string(*etcd.Spec.Common.AutoCompactionMode)
	}
	command = append(command, "--auto-compaction-mode="+compactionMode)

	compactionRetention := defaultAutoCompactionRetention
	if etcd.Spec.Common.AutoCompactionRetention != nil {
		compactionRetention = *etcd.Spec.Common.AutoCompactionRetention
	}
	command = append(command, "--auto-compaction-retention="+compactionRetention)

	etcdSnapshotTimeout := defaultEtcdSnapshotTimeout
	if etcd.Spec.Backup.EtcdSnapshotTimeout != nil {
		etcdSnapshotTimeout = etcd.Spec.Backup.EtcdSnapshotTimeout.Duration.String()
	}
	command = append(command, "--etcd-snapshot-timeout="+etcdSnapshotTimeout)

	etcdDefragTimeout := defaultEtcdDefragTimeout
	if etcd.Spec.Etcd.EtcdDefragTimeout != nil {
		etcdDefragTimeout = etcd.Spec.Etcd.EtcdDefragTimeout.Duration.String()
	}
	command = append(command, "--etcd-defrag-timeout="+etcdDefragTimeout)

	command = append(command, "--snapstore-temp-directory=/var/etcd/data/temp")
	command = append(command, "--enable-member-lease-renewal=true")

	heartbeatDuration := defaultHeartbeatDuration
	if etcd.Spec.Etcd.HeartbeatDuration != nil {
		heartbeatDuration = etcd.Spec.Etcd.HeartbeatDuration.Duration.String()
	}
	command = append(command, "--k8s-heartbeat-duration="+heartbeatDuration)

	if etcd.Spec.Backup.LeaderElection != nil {
		if etcd.Spec.Backup.LeaderElection.EtcdConnectionTimeout != nil {
			command = append(command, "--etcd-connection-timeout-leader-election="+etcd.Spec.Backup.LeaderElection.EtcdConnectionTimeout.Duration.String())
		}

		if etcd.Spec.Backup.LeaderElection.ReelectionPeriod != nil {
			command = append(command, "--reelection-period="+etcd.Spec.Backup.LeaderElection.ReelectionPeriod.Duration.String())
		}
	}

	return command
}

func getBackupPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "server",
			Protocol:      "TCP",
			ContainerPort: defaultBackupPort,
		},
	}
}

func getBackupResources(etcd *druidv1alpha1.Etcd) corev1.ResourceRequirements {
	if etcd.Spec.Backup.Resources != nil {
		return *etcd.Spec.Backup.Resources
	}
	return defaultResourceRequirements
}

func getBackupRestoreEnvVars(etcd *druidv1alpha1.Etcd) []corev1.EnvVar {
	var (
		env              []corev1.EnvVar
		storageContainer string
		storeValues      = etcd.Spec.Backup.Store
	)

	if etcd.Spec.Backup.Store != nil {
		storageContainer = pointer.StringDeref(etcd.Spec.Backup.Store.Container, "")
	}

	// TODO(timuthy, shreyas-s-rao): Move STORAGE_CONTAINER a few lines below so that we can append and exit in one step. This should only be done in a release where a restart of etcd is acceptable.
	env = append(env, getEnvVarFromValue("STORAGE_CONTAINER", storageContainer))
	env = append(env, getEnvVarFromField("POD_NAME", "metadata.name"))
	env = append(env, getEnvVarFromField("POD_NAMESPACE", "metadata.namespace"))

	if storeValues == nil {
		return env
	}

	provider, err := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
	if err != nil {
		return env
	}

	// TODO(timuthy): move this to a non root path when we switch to a rootless distribution
	const credentialsMountPath = "/var/etcd-backup"
	switch provider {
	case utils.S3:
		env = append(env, getEnvVarFromValue("AWS_APPLICATION_CREDENTIALS", credentialsMountPath))

	case utils.ABS:
		env = append(env, getEnvVarFromValue("AZURE_APPLICATION_CREDENTIALS", credentialsMountPath))

	case utils.GCS:
		env = append(env, getEnvVarFromValue("GOOGLE_APPLICATION_CREDENTIALS", "/var/.gcp/serviceaccount.json"))

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

func getBackupRestoreVolumeMounts(useEtcdWrapper bool, etcd *druidv1alpha1.Etcd) []corev1.VolumeMount {
	volumeClaimTemplateName := etcd.Name
	if etcd.Spec.VolumeClaimTemplate != nil && len(*etcd.Spec.VolumeClaimTemplate) != 0 {
		volumeClaimTemplateName = *etcd.Spec.VolumeClaimTemplate
	}
	vms := []corev1.VolumeMount{
		{
			Name:      volumeClaimTemplateName,
			MountPath: "/var/etcd/data",
		},
		{
			Name:      "etcd-config-file",
			MountPath: "/var/etcd/config/",
		},
	}

	vms = append(vms, getSecretVolumeMounts(etcd.Spec.Etcd.ClientUrlTLS, etcd.Spec.Etcd.PeerUrlTLS)...)

	if etcd.Spec.Backup.Store == nil {
		return vms
	}

	provider, err := utils.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
	if err != nil {
		return vms
	}

	switch provider {
	case utils.Local:
		if etcd.Spec.Backup.Store.Container != nil {
			if useEtcdWrapper {
				vms = append(vms, corev1.VolumeMount{
					Name:      "host-storage",
					MountPath: "/home/nonroot/" + pointer.StringDeref(etcd.Spec.Backup.Store.Container, ""),
				})
			} else {
				vms = append(vms, corev1.VolumeMount{
					Name:      "host-storage",
					MountPath: pointer.StringDeref(etcd.Spec.Backup.Store.Container, ""),
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

func getvolumeClaimTemplateName(etcd *druidv1alpha1.Etcd) string {
	volumeClaimTemplateName := etcd.Name
	if etcd.Spec.VolumeClaimTemplate != nil && len(*etcd.Spec.VolumeClaimTemplate) != 0 {
		volumeClaimTemplateName = *etcd.Spec.VolumeClaimTemplate
	}
	return volumeClaimTemplateName
}

func getStorageReq(etcd *druidv1alpha1.Etcd) corev1.ResourceRequirements {
	storageCapacity := defaultStorageCapacity
	if etcd.Spec.StorageCapacity != nil {
		storageCapacity = *etcd.Spec.StorageCapacity
	}

	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceStorage: storageCapacity,
		},
	}
}
