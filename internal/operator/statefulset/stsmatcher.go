// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"context"
	"fmt"
	"strconv"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/utils"
	utils2 "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	defaultTestContainerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    apiresource.MustParse("50m"),
			corev1.ResourceMemory: apiresource.MustParse("128Mi"),
		},
	}
)

// StatefulSetMatcher is the type used for matching StatefulSets. It holds relevant information required for matching.
type StatefulSetMatcher struct {
	g                  *WithT
	cl                 client.Client
	replicas           int32
	etcd               *druidv1alpha1.Etcd
	useEtcdWrapper     bool
	initContainerImage string
	etcdImage          string
	etcdBRImage        string
	provider           *string
	clientPort         int32
	serverPort         int32
	backupPort         int32
}

// NewStatefulSetMatcher constructs a new instance of StatefulSetMatcher.
func NewStatefulSetMatcher(g *WithT,
	cl client.Client,
	etcd *druidv1alpha1.Etcd,
	replicas int32,
	useEtcdWrapper bool,
	initContainerImage, etcdImage, etcdBRImage string,
	provider *string) StatefulSetMatcher {
	return StatefulSetMatcher{
		g:                  g,
		cl:                 cl,
		replicas:           replicas,
		etcd:               etcd,
		useEtcdWrapper:     useEtcdWrapper,
		initContainerImage: initContainerImage,
		etcdImage:          etcdImage,
		etcdBRImage:        etcdBRImage,
		provider:           provider,
		clientPort:         pointer.Int32Deref(etcd.Spec.Etcd.ClientPort, 2379),
		serverPort:         pointer.Int32Deref(etcd.Spec.Etcd.ServerPort, 2380),
		backupPort:         pointer.Int32Deref(etcd.Spec.Backup.Port, 8080),
	}
}

// MatchStatefulSet returns a custom gomega matcher that will match both the ObjectMeta and Spec of a StatefulSet.
func (s StatefulSetMatcher) MatchStatefulSet() gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": s.matchSTSObjectMeta(),
		"Spec":       s.matchSpec(),
	})
}

func (s StatefulSetMatcher) matchSTSObjectMeta() gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"Name":            Equal(s.etcd.Name),
		"Namespace":       Equal(s.etcd.Namespace),
		"OwnerReferences": utils2.MatchEtcdOwnerReference(s.etcd.Name, s.etcd.UID),
		"Labels":          utils2.MatchResourceLabels(getStatefulSetLabels(s.etcd.Name)),
	})
}

func (s StatefulSetMatcher) matchSpec() gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"Replicas":            PointTo(Equal(s.replicas)),
		"Selector":            utils2.MatchSpecLabelSelector(s.etcd.GetDefaultLabels()),
		"PodManagementPolicy": Equal(appsv1.ParallelPodManagement),
		"UpdateStrategy": MatchFields(IgnoreExtras, Fields{
			"Type": Equal(appsv1.RollingUpdateStatefulSetStrategyType),
		}),
		"VolumeClaimTemplates": s.matchVolumeClaimTemplates(),
		"ServiceName":          Equal(s.etcd.GetPeerServiceName()),
		"Template":             s.matchPodTemplateSpec(),
	})
}

func (s StatefulSetMatcher) matchVolumeClaimTemplates() gomegatypes.GomegaMatcher {
	defaultStorageCapacity := apiresource.MustParse("16Gi")
	return ConsistOf(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name": Equal(utils.TypeDeref[string](s.etcd.Spec.VolumeClaimTemplate, s.etcd.Name)),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"AccessModes": ConsistOf(corev1.ReadWriteOnce),
			"Resources": MatchFields(IgnoreExtras, Fields{
				"Requests": MatchKeys(IgnoreExtras, Keys{
					corev1.ResourceStorage: Equal(utils.TypeDeref[apiresource.Quantity](s.etcd.Spec.StorageCapacity, defaultStorageCapacity)),
				}),
			}),
			"StorageClassName": Equal(s.etcd.Spec.StorageClass),
		}),
	}))
}

func (s StatefulSetMatcher) matchPodTemplateSpec() gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": s.matchPodObjectMeta(),
		"Spec":       s.matchPodSpec(),
	})
}

func (s StatefulSetMatcher) matchPodObjectMeta() gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"Labels": utils2.MatchResourceLabels(utils.MergeMaps[string, string](getStatefulSetLabels(s.etcd.Name), s.etcd.Spec.Labels)),
		"Annotations": utils2.MatchResourceAnnotations(utils.MergeMaps[string, string](s.etcd.Spec.Annotations, map[string]string{
			"checksum/etcd-configmap": utils2.TestConfigMapCheckSum,
		})),
	})
}

func (s StatefulSetMatcher) matchPodSpec() gomegatypes.GomegaMatcher {
	// NOTE: currently this matcher does not check affinity and TSC since these are seldom used. If these are used in future then this matcher should be enhanced.
	return MatchFields(IgnoreExtras, Fields{
		"HostAliases":           s.matchPodHostAliases(),
		"ServiceAccountName":    Equal(s.etcd.GetServiceAccountName()),
		"ShareProcessNamespace": PointTo(Equal(true)),
		"InitContainers":        s.matchPodInitContainers(),
		"Containers":            s.matchContainers(),
		"SecurityContext":       s.matchEtcdPodSecurityContext(),
		"Volumes":               s.matchPodVolumes(),
		"PriorityClassName":     Equal(utils.TypeDeref[string](s.etcd.Spec.PriorityClassName, "")),
	})
}

func (s StatefulSetMatcher) matchPodHostAliases() gomegatypes.GomegaMatcher {
	return ConsistOf(MatchFields(IgnoreExtras, Fields{
		"IP":        Equal("127.0.0.1"),
		"Hostnames": ConsistOf(fmt.Sprintf("%s-local", s.etcd.Name)),
	}))
}

func (s StatefulSetMatcher) matchPodInitContainers() gomegatypes.GomegaMatcher {
	if !s.useEtcdWrapper {
		return BeEmpty()
	}
	initContainerMatcherElements := make(map[string]gomegatypes.GomegaMatcher)
	changePermissionsInitContainerMatcher := MatchFields(IgnoreExtras, Fields{
		"Name":            Equal(common.ChangePermissionsInitContainerName),
		"Image":           Equal(s.initContainerImage),
		"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
		"Command":         HaveExactElements("sh", "-c", "--"),
		"Args":            HaveExactElements("chown -R 65532:65532 /var/etcd/data"),
		"VolumeMounts":    ConsistOf(s.matchEtcdDataVolMount()),
		"SecurityContext": matchPodInitContainerSecurityContext(),
	})
	initContainerMatcherElements[common.ChangePermissionsInitContainerName] = changePermissionsInitContainerMatcher
	if s.etcd.IsBackupStoreEnabled() && s.provider != nil && *s.provider == utils.Local {
		changeBackupBucketPermissionsMatcher := MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(common.ChangeBackupBucketPermissionsInitContainerName),
			"Image":           Equal(s.initContainerImage),
			"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
			"Command":         HaveExactElements("sh", "-c", "--"),
			"Args":            HaveExactElements(fmt.Sprintf("chown -R 65532:65532 /home/nonroot/%s", *s.etcd.Spec.Backup.Store.Container)),
			"VolumeMounts":    s.matchBackupRestoreContainerVolMounts(),
			"SecurityContext": matchPodInitContainerSecurityContext(),
		})
		initContainerMatcherElements[common.ChangeBackupBucketPermissionsInitContainerName] = changeBackupBucketPermissionsMatcher
	}
	return MatchAllElements(containerIdentifier, initContainerMatcherElements)
}

func (s StatefulSetMatcher) matchContainers() gomegatypes.GomegaMatcher {
	return MatchAllElements(containerIdentifier, Elements{
		common.EtcdContainerName:              s.matchEtcdContainer(),
		common.EtcdBackupRestoreContainerName: s.matchBackupRestoreContainer(),
	})
}

func (s StatefulSetMatcher) matchEtcdContainer() gomegatypes.GomegaMatcher {
	etcdContainerResources := utils.TypeDeref[corev1.ResourceRequirements](s.etcd.Spec.Etcd.Resources, defaultTestContainerResources)
	return MatchFields(IgnoreExtras|IgnoreMissing, Fields{
		"Name":            Equal("etcd"),
		"Image":           Equal(s.etcdImage),
		"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
		"Args":            s.matchEtcdContainerCmdArgs(),
		"ReadinessProbe": PointTo(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"ProbeHandler":        s.matchEtcdContainerReadinessHandler(),
			"InitialDelaySeconds": Equal(int32(15)),
			"PeriodSeconds":       Equal(int32(5)),
			"FailureThreshold":    Equal(int32(5)),
		})),
		"Ports": ConsistOf(
			MatchFields(IgnoreExtras, Fields{
				"Name":          Equal(serverPortName),
				"Protocol":      Equal(corev1.ProtocolTCP),
				"ContainerPort": Equal(s.serverPort),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Name":          Equal(clientPortName),
				"Protocol":      Equal(corev1.ProtocolTCP),
				"ContainerPort": Equal(s.clientPort),
			}),
		),
		"Resources":    Equal(etcdContainerResources),
		"Env":          s.matchEtcdContainerEnvVars(),
		"VolumeMounts": s.matchEtcdContainerVolMounts(),
	})
}

func (s StatefulSetMatcher) matchEtcdContainerVolMounts() gomegatypes.GomegaMatcher {
	volMountMatchers := make([]gomegatypes.GomegaMatcher, 0, 6)
	volMountMatchers = append(volMountMatchers, s.matchEtcdDataVolMount())
	secretVolMountMatchers := s.getEtcdSecretVolMountsMatchers()
	if len(secretVolMountMatchers) > 0 {
		volMountMatchers = append(volMountMatchers, secretVolMountMatchers...)
	}
	return ConsistOf(volMountMatchers)
}

func (s StatefulSetMatcher) matchBackupRestoreContainer() gomegatypes.GomegaMatcher {
	containerResources := utils2.TypeDeref(s.etcd.Spec.Backup.Resources, defaultTestContainerResources)
	return MatchFields(IgnoreExtras, Fields{
		"Name":            Equal("backup-restore"),
		"Image":           Equal(s.etcdBRImage),
		"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
		// This is quite painful and therefore skipped for now. Independent unit test for command line args should be written instead.
		//"Args":            Equal(s.matchBackupRestoreContainerCmdArgs()),
		"Ports": ConsistOf(
			MatchFields(IgnoreExtras, Fields{
				"Name":          Equal(serverPortName),
				"Protocol":      Equal(corev1.ProtocolTCP),
				"ContainerPort": Equal(s.backupPort),
			}),
		),
		"Resources":    Equal(containerResources),
		"VolumeMounts": s.matchBackupRestoreContainerVolMounts(),
		"SecurityContext": PointTo(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"Capabilities": PointTo(MatchFields(IgnoreExtras, Fields{
				"Add": ConsistOf(corev1.Capability("SYS_PTRACE")),
			})),
		})),
	})
}

func (s StatefulSetMatcher) matchEtcdContainerReadinessHandler() gomegatypes.GomegaMatcher {
	if s.etcd.Spec.Replicas > 1 && !s.useEtcdWrapper {
		return MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"Exec": PointTo(MatchFields(IgnoreExtras, Fields{
				"Command": s.matchEtcdContainerReadinessProbeCmd(),
			})),
		})
	}
	scheme := utils.IfConditionOr[corev1.URIScheme](s.etcd.Spec.Backup.TLS == nil, corev1.URISchemeHTTP, corev1.URISchemeHTTPS)
	path := utils.IfConditionOr[string](s.etcd.Spec.Replicas > 1, "/readyz", "/healthz")
	port := utils.IfConditionOr[int32](s.etcd.Spec.Replicas > 1, 9095, 8080)
	return MatchFields(IgnoreExtras|IgnoreMissing, Fields{
		"HTTPGet": PointTo(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"Path": Equal(path),
			"Port": MatchFields(IgnoreExtras|IgnoreMissing, Fields{
				"Type":   Equal(intstr.Int),
				"IntVal": Equal(port),
			}),
			"Scheme": Equal(scheme),
		})),
	})
}

func (s StatefulSetMatcher) matchEtcdContainerReadinessProbeCmd() gomegatypes.GomegaMatcher {
	if s.etcd.Spec.Etcd.ClientUrlTLS != nil {
		dataKey := utils.TypeDeref(s.etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey, "ca.crt")
		return HaveExactElements(
			"/bin/sh",
			"-ec",
			fmt.Sprintf("ETCDCTL_API=3 etcdctl --cacert=/var/etcd/ssl/client/ca/%s --cert=/var/etcd/ssl/client/client/tls.crt --key=/var/etcd/ssl/client/client/tls.key --endpoints=https://%s-local:%d get foo --consistency=l", dataKey, s.etcd.Name, s.clientPort),
		)
	} else {
		return HaveExactElements(
			"/bin/sh",
			"-ec",
			fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=http://%s-local:%d get foo --consistency=l", s.etcd.Name, s.clientPort),
		)
	}
}

func (s StatefulSetMatcher) matchEtcdContainerCmdArgs() gomegatypes.GomegaMatcher {
	if !s.useEtcdWrapper {
		return BeEmpty()
	}
	cmdArgs := make([]string, 0, 8)
	cmdArgs = append(cmdArgs, "start-etcd")
	cmdArgs = append(cmdArgs, fmt.Sprintf("--backup-restore-host-port=%s-local:8080", s.etcd.Name))
	cmdArgs = append(cmdArgs, fmt.Sprintf("--etcd-server-name=%s-local", s.etcd.Name))
	if s.etcd.Spec.Etcd.ClientUrlTLS == nil {
		cmdArgs = append(cmdArgs, "--backup-restore-tls-enabled=false")
	} else {
		dataKey := utils.TypeDeref(s.etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey, "ca.crt")
		cmdArgs = append(cmdArgs, "--backup-restore-tls-enabled=true")
		cmdArgs = append(cmdArgs, "--etcd-client-cert-path=/var/etcd/ssl/client/client/tls.crt")
		cmdArgs = append(cmdArgs, "--etcd-client-key-path=/var/etcd/ssl/client/client/tls.key")
		cmdArgs = append(cmdArgs, fmt.Sprintf("--backup-restore-ca-cert-bundle-path=/var/etcd/ssl/client/ca/%s", dataKey))
	}
	return HaveExactElements(cmdArgs)
}

func (s StatefulSetMatcher) matchEtcdDataVolMount() gomegatypes.GomegaMatcher {
	volumeClaimTemplateName := utils.TypeDeref[string](s.etcd.Spec.VolumeClaimTemplate, s.etcd.Name)
	return matchVolMount(volumeClaimTemplateName, etcdDataVolumeMountPath)
}

func (s StatefulSetMatcher) matchBackupRestoreContainerVolMounts() gomegatypes.GomegaMatcher {
	volMountMatchers := make([]gomegatypes.GomegaMatcher, 0, 6)
	volMountMatchers = append(volMountMatchers, s.matchEtcdDataVolMount())
	volMountMatchers = append(volMountMatchers, matchVolMount(etcdConfigVolumeName, etcdConfigFileMountPath))
	volMountMatchers = append(volMountMatchers, s.getBackupRestoreSecretVolMountMatchers()...)
	if s.etcd.IsBackupStoreEnabled() {
		etcdBackupVolMountMatcher := s.getEtcdBackupVolumeMountMatcher()
		if etcdBackupVolMountMatcher != nil {
			volMountMatchers = append(volMountMatchers, etcdBackupVolMountMatcher)
		}
	}
	return ConsistOf(volMountMatchers)
}

func (s StatefulSetMatcher) getBackupRestoreSecretVolMountMatchers() []gomegatypes.GomegaMatcher {
	secretVolMountMatchers := make([]gomegatypes.GomegaMatcher, 0, 3)
	if s.etcd.Spec.Backup.TLS != nil {
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(backRestoreCAVolumeName, backupRestoreCAVolumeMountPath))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(backRestoreServerTLSVolumeName, backupRestoreServerTLSVolumeMountPath))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(backRestoreClientTLSVolumeName, backupRestoreClientTLSVolumeMountPath))
	}
	return secretVolMountMatchers
}

func (s StatefulSetMatcher) getEtcdSecretVolMountsMatchers() []gomegatypes.GomegaMatcher {
	secretVolMountMatchers := make([]gomegatypes.GomegaMatcher, 0, 5)
	if s.etcd.Spec.Etcd.ClientUrlTLS != nil {
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(clientCAVolumeName, etcdCAVolumeMountPath))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(serverTLSVolumeName, etcdServerTLSVolumeMountPath))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(clientTLSVolumeName, etcdClientTLSVolumeMountPath))
	}
	if s.etcd.Spec.Etcd.PeerUrlTLS != nil {
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(peerCAVolumeName, etcdPeerCAVolumeMountPath))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(peerServerTLSVolumeName, etcdPeerServerTLSVolumeMountPath))
	}
	return secretVolMountMatchers
}

func (s StatefulSetMatcher) getEtcdBackupVolumeMountMatcher() gomegatypes.GomegaMatcher {
	switch *s.provider {
	case utils.Local:
		if s.etcd.Spec.Backup.Store.Container != nil {
			if s.useEtcdWrapper {
				return matchVolMount(localBackupVolumeName, fmt.Sprintf("/home/nonroot/%s", pointer.StringDeref(s.etcd.Spec.Backup.Store.Container, "")))
			} else {
				return matchVolMount(localBackupVolumeName, pointer.StringDeref(s.etcd.Spec.Backup.Store.Container, ""))
			}
		}
	case utils.GCS:
		return matchVolMount(providerBackupVolumeName, gcsBackupVolumeMountPath)
	case utils.S3, utils.ABS, utils.OSS, utils.Swift, utils.OCS:
		return matchVolMount(providerBackupVolumeName, nonGCSProviderBackupVolumeMountPath)
	}
	return nil
}

func (s StatefulSetMatcher) matchEtcdContainerEnvVars() gomegatypes.GomegaMatcher {
	if s.useEtcdWrapper {
		return BeEmpty()
	}
	scheme := utils.IfConditionOr[string](s.etcd.Spec.Backup.TLS != nil, "https", "http")
	return ConsistOf(
		MatchFields(IgnoreExtras, Fields{
			"Name":  Equal("ENABLE_TLS"),
			"Value": Equal(strconv.FormatBool(s.etcd.Spec.Backup.TLS != nil)),
		}),
		MatchFields(IgnoreExtras, Fields{
			"Name":  Equal("BACKUP_ENDPOINT"),
			"Value": Equal(fmt.Sprintf("%s://%s-local:%d", scheme, s.etcd.Name, s.backupPort)),
		}),
	)
}

func (s StatefulSetMatcher) matchEtcdPodSecurityContext() gomegatypes.GomegaMatcher {
	if !s.useEtcdWrapper {
		Equal(nil)
	}
	return PointTo(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
		"RunAsGroup":   PointTo(Equal(int64(65532))),
		"RunAsNonRoot": PointTo(Equal(true)),
		"RunAsUser":    PointTo(Equal(int64(65532))),
	}))
}

func (s StatefulSetMatcher) matchPodVolumes() gomegatypes.GomegaMatcher {
	volMatchers := make([]gomegatypes.GomegaMatcher, 0, 7)
	etcdConfigFileVolMountMatcher := MatchFields(IgnoreExtras, Fields{
		"Name": Equal(etcdConfigVolumeName),
		"VolumeSource": MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
				"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
					"Name": Equal(s.etcd.GetConfigMapName()),
				}),
				"Items": HaveExactElements(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
					"Key":  Equal(etcdConfigFileName),
					"Path": Equal(etcdConfigFileName),
				})),
				"DefaultMode": PointTo(Equal(int32(0644))),
			})),
		}),
	})
	volMatchers = append(volMatchers, etcdConfigFileVolMountMatcher)
	secretVolMatchers := s.getPodSecurityVolumeMatchers()
	if len(secretVolMatchers) > 0 {
		volMatchers = append(volMatchers, secretVolMatchers...)
	}
	if s.etcd.IsBackupStoreEnabled() {
		backupVolMatcher := s.getBackupVolumeMatcher()
		if backupVolMatcher != nil {
			volMatchers = append(volMatchers, backupVolMatcher)
		}
	}
	return ConsistOf(volMatchers)
}

func (s StatefulSetMatcher) getPodSecurityVolumeMatchers() []gomegatypes.GomegaMatcher {
	volMatchers := make([]gomegatypes.GomegaMatcher, 0, 5)
	if s.etcd.Spec.Etcd.ClientUrlTLS != nil {
		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(clientCAVolumeName),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName": Equal(s.etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name),
				})),
			}),
		}))
		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(serverTLSVolumeName),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName": Equal(s.etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name),
				})),
			}),
		}))
		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(clientTLSVolumeName),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName": Equal(s.etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name),
				})),
			}),
		}))
	}
	if s.etcd.Spec.Etcd.PeerUrlTLS != nil {
		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(peerCAVolumeName),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName": Equal(s.etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name),
				})),
			}),
		}))

		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(peerServerTLSVolumeName),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName": Equal(s.etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name),
				})),
			}),
		}))
	}
	if s.etcd.Spec.Backup.TLS != nil {
		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(backRestoreCAVolumeName),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName": Equal(s.etcd.Spec.Backup.TLS.TLSCASecretRef.Name),
				})),
			}),
		}))

		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(backRestoreServerTLSVolumeName),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName": Equal(s.etcd.Spec.Backup.TLS.ServerTLSSecretRef.Name),
				})),
			}),
		}))

		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(backRestoreClientTLSVolumeName),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName": Equal(s.etcd.Spec.Backup.TLS.ClientTLSSecretRef.Name),
				})),
			}),
		}))
	}
	return volMatchers
}

func (s StatefulSetMatcher) getBackupVolumeMatcher() gomegatypes.GomegaMatcher {
	if s.provider == nil {
		return nil
	}
	switch *s.provider {
	case utils.Local:
		hostPath, err := utils.GetHostMountPathFromSecretRef(context.Background(), s.cl, logr.Discard(), s.etcd.Spec.Backup.Store, s.etcd.Namespace)
		s.g.Expect(err).ToNot(HaveOccurred())
		return MatchFields(IgnoreExtras, Fields{
			"Name": Equal(localBackupVolumeName),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"HostPath": PointTo(MatchFields(IgnoreExtras, Fields{
					"Path": Equal(fmt.Sprintf("%s/%s", hostPath, pointer.StringDeref(s.etcd.Spec.Backup.Store.Container, ""))),
					"Type": PointTo(Equal(corev1.HostPathDirectory)),
				})),
			}),
		})

	case utils.GCS, utils.S3, utils.OSS, utils.ABS, utils.Swift, utils.OCS:
		s.g.Expect(s.etcd.Spec.Backup.Store.SecretRef).ToNot(BeNil())
		return MatchFields(IgnoreExtras, Fields{
			"Name": Equal("etcd-backup"),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName": Equal(s.etcd.Spec.Backup.Store.SecretRef.Name),
				})),
			}),
		})
	}
	return nil
}

func matchVolMount(name, mountPath string) gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras|IgnoreMissing, Fields{
		"Name":      Equal(name),
		"MountPath": Equal(mountPath),
	})
}

func matchPodInitContainerSecurityContext() gomegatypes.GomegaMatcher {
	return PointTo(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
		"RunAsGroup":   PointTo(Equal(int64(0))),
		"RunAsNonRoot": PointTo(Equal(false)),
		"RunAsUser":    PointTo(Equal(int64(0))),
	}))
}

func getStatefulSetLabels(etcdName string) map[string]string {
	return map[string]string{
		druidv1alpha1.LabelComponentKey: common.StatefulSetComponentName,
		druidv1alpha1.LabelAppNameKey:   etcdName,
		druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
		druidv1alpha1.LabelPartOfKey:    etcdName,
	}
}

func containerIdentifier(element interface{}) string {
	return (element.(corev1.Container)).Name
}
