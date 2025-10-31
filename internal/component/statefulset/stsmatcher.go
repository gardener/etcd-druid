// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"context"
	"fmt"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druidstore "github.com/gardener/etcd-druid/internal/store"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	gomegatypes "github.com/onsi/gomega/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
	g                      *WithT
	cl                     client.Client
	etcd                   *druidv1alpha1.Etcd
	initContainerImage     string
	etcdImage              string
	etcdBRImage            string
	provider               *string
	clientPort             int32
	serverPort             int32
	backupPort             int32
	wrapperPort            int32
	expectedReplicas       int32
	expectNoServiceAccount bool
	expectNoService        bool
}

// NewStatefulSetMatcher constructs a new instance of StatefulSetMatcher.
func NewStatefulSetMatcher(g *WithT,
	cl client.Client,
	etcd *druidv1alpha1.Etcd,
	replicas int32,
	initContainerImage, etcdImage, etcdBRImage string,
	provider *string,
	expectNoServiceAccount, expectNoService bool) StatefulSetMatcher {
	return StatefulSetMatcher{
		g:                      g,
		cl:                     cl,
		etcd:                   etcd,
		initContainerImage:     initContainerImage,
		etcdImage:              etcdImage,
		etcdBRImage:            etcdBRImage,
		provider:               provider,
		clientPort:             ptr.Deref(etcd.Spec.Etcd.ClientPort, 2379),
		serverPort:             ptr.Deref(etcd.Spec.Etcd.ServerPort, 2380),
		backupPort:             ptr.Deref(etcd.Spec.Backup.Port, 8080),
		wrapperPort:            ptr.Deref(etcd.Spec.Etcd.WrapperPort, 9095),
		expectedReplicas:       replicas,
		expectNoServiceAccount: expectNoServiceAccount,
		expectNoService:        expectNoService,
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
		"OwnerReferences": testutils.MatchEtcdOwnerReference(s.etcd.Name, s.etcd.UID),
		"Labels":          testutils.MatchResourceLabels(getStatefulSetLabels(s.etcd.Name)),
	})
}

func (s StatefulSetMatcher) matchSpec() gomegatypes.GomegaMatcher {
	fields := map[string]gomegatypes.GomegaMatcher{
		"Replicas":            PointTo(Equal(s.expectedReplicas)),
		"Selector":            testutils.MatchSpecLabelSelector(druidv1alpha1.GetDefaultLabels(s.etcd.ObjectMeta)),
		"PodManagementPolicy": Equal(appsv1.ParallelPodManagement),
		"UpdateStrategy": MatchFields(IgnoreExtras, Fields{
			"Type": Equal(appsv1.RollingUpdateStatefulSetStrategyType),
		}),
		"VolumeClaimTemplates": s.matchVolumeClaimTemplates(),
		"Template":             s.matchPodTemplateSpec(),
	}
	if !s.expectNoService {
		fields["ServiceName"] = Equal(druidv1alpha1.GetPeerServiceName(s.etcd.ObjectMeta))
	}
	if druidconfigv1alpha1.DefaultFeatureGates.IsEnabled(druidconfigv1alpha1.AllowEmptyDir) {
		delete(fields, "VolumeClaimTemplates")
	}
	return MatchFields(IgnoreExtras, fields)
}

func (s StatefulSetMatcher) matchVolumeClaimTemplates() gomegatypes.GomegaMatcher {
	defaultStorageCapacity := apiresource.MustParse("16Gi")
	return ConsistOf(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name": Equal(ptr.Deref(s.etcd.Spec.VolumeClaimTemplate, s.etcd.Name)),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"AccessModes": ConsistOf(corev1.ReadWriteOnce),
			"Resources": MatchFields(IgnoreExtras, Fields{
				"Requests": MatchKeys(IgnoreExtras, Keys{
					corev1.ResourceStorage: Equal(ptr.Deref(s.etcd.Spec.StorageCapacity, defaultStorageCapacity)),
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
		"Labels": testutils.MatchResourceLabels(utils.MergeMaps(getStatefulSetLabels(s.etcd.Name), s.etcd.Spec.Labels)),
		"Annotations": testutils.MatchResourceAnnotations(utils.MergeMaps(s.etcd.Spec.Annotations, map[string]string{
			"checksum/etcd-configmap": testutils.TestConfigMapCheckSum,
		})),
	})
}

func (s StatefulSetMatcher) matchPodSpec() gomegatypes.GomegaMatcher {
	// NOTE: currently this matcher does not check affinity and TSC since these are seldom used. If these are used in future then this matcher should be enhanced.
	fields := map[string]gomegatypes.GomegaMatcher{
		"HostAliases":           s.matchPodHostAliases(),
		"ShareProcessNamespace": PointTo(Equal(true)),
		"InitContainers":        s.matchPodInitContainers(),
		"Containers":            s.matchContainers(),
		"SecurityContext":       s.matchEtcdPodSecurityContext(),
		"Volumes":               s.matchPodVolumes(),
		"PriorityClassName":     Equal(ptr.Deref(s.etcd.Spec.PriorityClassName, "")),
	}
	if !s.expectNoServiceAccount {
		fields["ServiceAccountName"] = Equal(druidv1alpha1.GetServiceAccountName(s.etcd.ObjectMeta))
	}
	return MatchFields(IgnoreExtras, fields)
}

func (s StatefulSetMatcher) matchPodHostAliases() gomegatypes.GomegaMatcher {
	return ConsistOf(MatchFields(IgnoreExtras, Fields{
		"IP":        Equal("127.0.0.1"),
		"Hostnames": ConsistOf(fmt.Sprintf("%s-local", s.etcd.Name)),
	}))
}

func (s StatefulSetMatcher) matchPodInitContainers() gomegatypes.GomegaMatcher {
	initContainerMatcherElements := make(map[string]gomegatypes.GomegaMatcher)
	if s.etcd.IsBackupStoreEnabled() && s.provider != nil && *s.provider == druidstore.Local && !ptr.Deref(s.etcd.Spec.RunAsRoot, false) {
		changeBackupBucketPermissionsMatcher := MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(common.InitContainerNameChangeBackupBucketPermissions),
			"Image":           Equal(s.initContainerImage),
			"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
			"Command":         HaveExactElements("sh", "-c", "--"),
			"Args":            HaveExactElements(fmt.Sprintf("chown -R 65532:65532 /home/nonroot/%s", *s.etcd.Spec.Backup.Store.Container)),
			"VolumeMounts":    ConsistOf(s.getEtcdBackupVolumeMountMatcher()),
			"SecurityContext": matchPodInitContainerSecurityContext(),
		})
		initContainerMatcherElements[common.InitContainerNameChangeBackupBucketPermissions] = changeBackupBucketPermissionsMatcher
	}
	return MatchAllElements(containerIdentifier, initContainerMatcherElements)
}

func (s StatefulSetMatcher) matchContainers() gomegatypes.GomegaMatcher {
	return MatchAllElements(containerIdentifier, Elements{
		common.ContainerNameEtcd:              s.matchEtcdContainer(),
		common.ContainerNameEtcdBackupRestore: s.matchBackupRestoreContainer(),
	})
}

func (s StatefulSetMatcher) matchEtcdContainer() gomegatypes.GomegaMatcher {
	etcdContainerResources := ptr.Deref(s.etcd.Spec.Etcd.Resources, defaultTestContainerResources)
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
	containerResources := ptr.Deref(s.etcd.Spec.Backup.Resources, defaultTestContainerResources)
	return MatchFields(IgnoreExtras, Fields{
		"Name":            Equal("backup-restore"),
		"Image":           Equal(s.etcdBRImage),
		"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
		// This is quite painful and therefore skipped for now. Independent unit test for command line args should be written instead.
		// "Args":            Equal(s.matchBackupRestoreContainerCmdArgs()),
		"Ports": ConsistOf(
			MatchFields(IgnoreExtras, Fields{
				"Name":          Equal(serverPortName),
				"Protocol":      Equal(corev1.ProtocolTCP),
				"ContainerPort": Equal(s.backupPort),
			}),
		),
		"Resources":    Equal(containerResources),
		"VolumeMounts": s.matchBackupRestoreContainerVolMounts(),
	})
}

func (s StatefulSetMatcher) matchEtcdContainerReadinessHandler() gomegatypes.GomegaMatcher {
	scheme := utils.IfConditionOr(s.etcd.Spec.Backup.TLS == nil, corev1.URISchemeHTTP, corev1.URISchemeHTTPS)
	path := utils.IfConditionOr(s.etcd.Spec.Replicas > 1, "/readyz", "/healthz")
	port := utils.IfConditionOr(s.etcd.Spec.Replicas > 1, s.wrapperPort, s.backupPort)
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

func (s StatefulSetMatcher) matchEtcdContainerCmdArgs() gomegatypes.GomegaMatcher {
	cmdArgs := make([]string, 0, 8)
	cmdArgs = append(cmdArgs, "start-etcd")
	cmdArgs = append(cmdArgs, fmt.Sprintf("--backup-restore-host-port=%s-local:%d", s.etcd.Name, s.backupPort))
	cmdArgs = append(cmdArgs, fmt.Sprintf("--etcd-server-name=%s-local", s.etcd.Name))
	// backup-restore tls specific configuration
	if s.etcd.Spec.Backup.TLS == nil {
		cmdArgs = append(cmdArgs, "--backup-restore-tls-enabled=false")
	} else {
		dataKey := ptr.Deref(s.etcd.Spec.Backup.TLS.TLSCASecretRef.DataKey, "ca.crt")
		cmdArgs = append(cmdArgs, "--backup-restore-tls-enabled=true")
		cmdArgs = append(cmdArgs, fmt.Sprintf("--backup-restore-ca-cert-bundle-path=/var/etcdbr/ssl/ca/%s", dataKey))
	}
	// etcd client url tls specific configuration
	if s.etcd.Spec.Etcd.ClientUrlTLS != nil {
		cmdArgs = append(cmdArgs, "--etcd-client-cert-path=/var/etcd/ssl/client/tls.crt")
		cmdArgs = append(cmdArgs, "--etcd-client-key-path=/var/etcd/ssl/client/tls.key")
	}
	cmdArgs = append(cmdArgs, fmt.Sprintf("--etcd-client-port=%d", s.clientPort))
	cmdArgs = append(cmdArgs, fmt.Sprintf("--etcd-wrapper-port=%d", s.wrapperPort))
	return HaveExactElements(cmdArgs)
}

func (s StatefulSetMatcher) matchEtcdDataVolMount() gomegatypes.GomegaMatcher {
	volumeMountName := ptr.Deref(s.etcd.Spec.VolumeClaimTemplate, s.etcd.Name)
	if druidconfigv1alpha1.DefaultFeatureGates.IsEnabled(druidconfigv1alpha1.AllowEmptyDir) {
		if s.etcd.Spec.EmptyDirVolumeSource != nil {
			volumeMountName = fmt.Sprintf("%s-emptydir", s.etcd.Name)
		}
	}
	return matchVolMount(volumeMountName, common.VolumeMountPathEtcdData)
}

func (s StatefulSetMatcher) matchBackupRestoreContainerVolMounts() gomegatypes.GomegaMatcher {
	volMountMatchers := make([]gomegatypes.GomegaMatcher, 0, 6)
	volMountMatchers = append(volMountMatchers, s.matchEtcdDataVolMount())
	volMountMatchers = append(volMountMatchers, matchVolMount(common.VolumeNameEtcdConfig, etcdConfigFileMountPath))
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
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(common.VolumeNameBackupRestoreCA, common.VolumeMountPathBackupRestoreCA))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(common.VolumeNameBackupRestoreServerTLS, common.VolumeMountPathBackupRestoreServerTLS))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(common.VolumeNameBackupRestoreClientTLS, common.VolumeMountPathBackupRestoreClientTLS))
	}
	return secretVolMountMatchers
}

func (s StatefulSetMatcher) getEtcdSecretVolMountsMatchers() []gomegatypes.GomegaMatcher {
	secretVolMountMatchers := make([]gomegatypes.GomegaMatcher, 0, 5)
	if s.etcd.Spec.Etcd.ClientUrlTLS != nil {
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(common.VolumeNameEtcdCA, common.VolumeMountPathEtcdCA))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(common.VolumeNameEtcdServerTLS, common.VolumeMountPathEtcdServerTLS))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(common.VolumeNameEtcdClientTLS, common.VolumeMountPathEtcdClientTLS))
	}
	if s.etcd.Spec.Etcd.PeerUrlTLS != nil {
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(common.VolumeNameEtcdPeerCA, common.VolumeMountPathEtcdPeerCA))
		secretVolMountMatchers = append(secretVolMountMatchers, matchVolMount(common.VolumeNameEtcdPeerServerTLS, common.VolumeMountPathEtcdPeerServerTLS))
	}
	return secretVolMountMatchers
}

func (s StatefulSetMatcher) getEtcdBackupVolumeMountMatcher() gomegatypes.GomegaMatcher {
	switch *s.provider {
	case druidstore.Local:
		if s.etcd.Spec.Backup.Store.Container != nil {
			if ptr.Deref(s.etcd.Spec.RunAsRoot, false) {
				return matchVolMount(common.VolumeNameLocalBackup, fmt.Sprintf("/root/%s", *s.etcd.Spec.Backup.Store.Container))
			}
			return matchVolMount(common.VolumeNameLocalBackup, fmt.Sprintf("/home/nonroot/%s", *s.etcd.Spec.Backup.Store.Container))
		}
	case druidstore.GCS:
		return matchVolMount(common.VolumeNameProviderBackupSecret, common.VolumeMountPathGCSBackupSecret)
	case druidstore.S3, druidstore.ABS, druidstore.OSS, druidstore.Swift, druidstore.OCS:
		return matchVolMount(common.VolumeNameProviderBackupSecret, common.VolumeMountPathNonGCSProviderBackupSecret)
	}
	return nil
}

func (s StatefulSetMatcher) matchEtcdContainerEnvVars() gomegatypes.GomegaMatcher {
	return BeEmpty()
}

func (s StatefulSetMatcher) matchEtcdPodSecurityContext() gomegatypes.GomegaMatcher {
	if ptr.Deref(s.etcd.Spec.RunAsRoot, false) {
		return PointTo(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"RunAsGroup":   PointTo(Equal(int64(0))),
			"RunAsNonRoot": PointTo(Equal(false)),
			"RunAsUser":    PointTo(Equal(int64(0))),
			"FSGroup":      PointTo(Equal(int64(0))),
		}))
	}

	return PointTo(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
		"RunAsGroup":   PointTo(Equal(int64(65532))),
		"RunAsNonRoot": PointTo(Equal(true)),
		"RunAsUser":    PointTo(Equal(int64(65532))),
		"FSGroup":      PointTo(Equal(int64(65532))),
	}))
}

func (s StatefulSetMatcher) matchPodVolumes() gomegatypes.GomegaMatcher {
	volMatchers := make([]gomegatypes.GomegaMatcher, 0, 8)
	if druidconfigv1alpha1.DefaultFeatureGates.IsEnabled(druidconfigv1alpha1.AllowEmptyDir) {
		emptyDirMatcher := s.getEmptyDirVolumeMatcher()
		if emptyDirMatcher != nil {
			volMatchers = append(volMatchers, emptyDirMatcher)
		}
	}
	etcdConfigFileVolMountMatcher := MatchFields(IgnoreExtras, Fields{
		"Name": Equal(common.VolumeNameEtcdConfig),
		"VolumeSource": MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
				"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
					"Name": Equal(druidv1alpha1.GetConfigMapName(s.etcd.ObjectMeta)),
				}),
				"Items": HaveExactElements(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
					"Key":  Equal(common.EtcdConfigFileName),
					"Path": Equal(common.EtcdConfigFileName),
				})),
				"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
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
			"Name": Equal(common.VolumeNameEtcdCA),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(s.etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
				})),
			}),
		}))
		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(common.VolumeNameEtcdServerTLS),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(s.etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
				})),
			}),
		}))
		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(common.VolumeNameEtcdClientTLS),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(s.etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
				})),
			}),
		}))
	}
	if s.etcd.Spec.Etcd.PeerUrlTLS != nil {
		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(common.VolumeNameEtcdPeerCA),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(s.etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
				})),
			}),
		}))

		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(common.VolumeNameEtcdPeerServerTLS),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(s.etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
				})),
			}),
		}))
	}
	if s.etcd.Spec.Backup.TLS != nil {
		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(common.VolumeNameBackupRestoreCA),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(s.etcd.Spec.Backup.TLS.TLSCASecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
				})),
			}),
		}))

		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(common.VolumeNameBackupRestoreServerTLS),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(s.etcd.Spec.Backup.TLS.ServerTLSSecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
				})),
			}),
		}))

		volMatchers = append(volMatchers, MatchFields(IgnoreExtras, Fields{
			"Name": Equal(common.VolumeNameBackupRestoreClientTLS),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(s.etcd.Spec.Backup.TLS.ClientTLSSecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
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
	case druidstore.Local:
		hostPath, err := druidstore.GetHostMountPathFromSecretRef(context.Background(), s.cl, logr.Discard(), s.etcd.Spec.Backup.Store, s.etcd.Namespace)
		s.g.Expect(err).ToNot(HaveOccurred())
		return MatchFields(IgnoreExtras, Fields{
			"Name": Equal(common.VolumeNameLocalBackup),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"HostPath": PointTo(MatchFields(IgnoreExtras, Fields{
					"Path": Equal(fmt.Sprintf("%s/%s", hostPath, ptr.Deref(s.etcd.Spec.Backup.Store.Container, ""))),
					"Type": PointTo(Equal(corev1.HostPathDirectoryOrCreate)),
				})),
			}),
		})

	case druidstore.GCS, druidstore.S3, druidstore.OSS, druidstore.ABS, druidstore.Swift, druidstore.OCS:
		s.g.Expect(s.etcd.Spec.Backup.Store.SecretRef).ToNot(BeNil())
		return MatchFields(IgnoreExtras, Fields{
			"Name": Equal(common.VolumeNameProviderBackupSecret),
			"VolumeSource": MatchFields(IgnoreExtras, Fields{
				"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
					"SecretName":  Equal(s.etcd.Spec.Backup.Store.SecretRef.Name),
					"DefaultMode": PointTo(Equal(common.ModeOwnerReadWriteGroupRead)),
				})),
			}),
		})
	}
	return nil
}

func (s StatefulSetMatcher) getEmptyDirVolumeMatcher() gomegatypes.GomegaMatcher {
	if s.etcd.Spec.EmptyDirVolumeSource == nil {
		return nil
	}
	storageCapacity := ptr.Deref(&s.etcd.Spec.EmptyDirVolumeSource.SizeLimit, &defaultStorageCapacity)
	return MatchFields(IgnoreExtras|IgnoreMissing, Fields{
		"Name": Equal(fmt.Sprintf("%s-emptydir", s.etcd.Name)),
		"VolumeSource": MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"EmptyDir": PointTo(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
				"Medium":    Equal(s.etcd.Spec.EmptyDirVolumeSource.Medium),
				"SizeLimit": PointTo(Equal(*storageCapacity)),
			})),
		}),
	})
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
		druidv1alpha1.LabelComponentKey: common.ComponentNameStatefulSet,
		druidv1alpha1.LabelAppNameKey:   etcdName,
		druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
		druidv1alpha1.LabelPartOfKey:    etcdName,
	}
}

func containerIdentifier(element interface{}) string {
	return (element.(corev1.Container)).Name
}
