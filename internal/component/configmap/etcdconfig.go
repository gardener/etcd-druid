// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package configmap

import (
	"fmt"
	"strconv"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"

	"k8s.io/utils/ptr"
)

// default values
const (
	defaultDBQuotaBytes            = int64(8 * 1024 * 1024 * 1024) // 8Gi
	defaultAutoCompactionRetention = "30m"
	defaultInitialClusterToken     = "etcd-cluster"
	defaultInitialClusterState     = "new"
	// For more information refer to https://etcd.io/docs/v3.4/op-guide/maintenance/#raft-log-retention
	// TODO: Ideally this should be made configurable via Etcd resource as this has a direct impact on the memory requirements for etcd container.
	// which in turn is influenced by the size of objects that are getting stored in etcd.
	defaultSnapshotCount = 75000
)

var (
	defaultDataDir = fmt.Sprintf("%s/new.etcd", common.VolumeMountPathEtcdData)
)

type etcdConfig struct {
	Name                    string                       `yaml:"name"`
	DataDir                 string                       `yaml:"data-dir"`
	Metrics                 druidv1alpha1.MetricsLevel   `yaml:"metrics"`
	SnapshotCount           int                          `yaml:"snapshot-count"`
	EnableV2                bool                         `yaml:"enable-v2"`
	QuotaBackendBytes       int64                        `yaml:"quota-backend-bytes"`
	InitialClusterToken     string                       `yaml:"initial-cluster-token"`
	InitialClusterState     string                       `yaml:"initial-cluster-state"`
	InitialCluster          string                       `yaml:"initial-cluster"`
	AutoCompactionMode      druidv1alpha1.CompactionMode `yaml:"auto-compaction-mode"`
	AutoCompactionRetention string                       `yaml:"auto-compaction-retention"`
	ListenPeerUrls          string                       `yaml:"listen-peer-urls"`
	ListenClientUrls        string                       `yaml:"listen-client-urls"`
	AdvertisePeerUrls       string                       `yaml:"initial-advertise-peer-urls"`
	AdvertiseClientUrls     string                       `yaml:"advertise-client-urls"`
	ClientSecurity          securityConfig               `yaml:"client-transport-security,omitempty"`
	PeerSecurity            securityConfig               `yaml:"peer-transport-security,omitempty"`
}

type securityConfig struct {
	CertFile       string `yaml:"cert-file,omitempty"`
	KeyFile        string `yaml:"key-file,omitempty"`
	ClientCertAuth bool   `yaml:"client-cert-auth,omitempty"`
	TrustedCAFile  string `yaml:"trusted-ca-file,omitempty"`
	AutoTLS        bool   `yaml:"auto-tls"`
}

// peerConfig encapsulates all configuration related to an etcd peer.
// This is a convenience struct which enabled reuse.
type peerConfig struct {
	serviceName       string
	initialCluster    string
	listenPeerUrls    string
	advertisePeerUrls string
	peerSecurity      *securityConfig
}

func createEtcdConfig(etcd *druidv1alpha1.Etcd) etcdConfig {
	clientScheme, clientSecurityConfig := getSchemeAndSecurityConfig(etcd.Spec.Etcd.ClientUrlTLS, common.VolumeMountPathEtcdCA, common.VolumeMountPathEtcdServerTLS)
	pc := createPeerConfig(etcd, int(etcd.Spec.Replicas))
	cfg := etcdConfig{
		Name:                    fmt.Sprintf("etcd-%s", etcd.UID[:6]),
		DataDir:                 defaultDataDir,
		Metrics:                 ptr.Deref(etcd.Spec.Etcd.Metrics, druidv1alpha1.Basic),
		SnapshotCount:           defaultSnapshotCount,
		EnableV2:                false,
		QuotaBackendBytes:       getDBQuotaBytes(etcd),
		InitialClusterToken:     defaultInitialClusterToken,
		InitialClusterState:     defaultInitialClusterState,
		InitialCluster:          pc.initialCluster,
		AutoCompactionMode:      ptr.Deref(etcd.Spec.Common.AutoCompactionMode, druidv1alpha1.Periodic),
		AutoCompactionRetention: ptr.Deref(etcd.Spec.Common.AutoCompactionRetention, defaultAutoCompactionRetention),
		ListenPeerUrls:          pc.listenPeerUrls,
		AdvertisePeerUrls:       pc.advertisePeerUrls,
		ListenClientUrls:        fmt.Sprintf("%s://0.0.0.0:%d", clientScheme, ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)),
		AdvertiseClientUrls:     fmt.Sprintf("%s@%s@%s@%d", clientScheme, pc.serviceName, etcd.Namespace, ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)),
	}
	if pc.peerSecurity != nil {
		cfg.PeerSecurity = *pc.peerSecurity
	}
	if clientSecurityConfig != nil {
		cfg.ClientSecurity = *clientSecurityConfig
	}
	return cfg
}

func prepareInitialCluster(etcd *druidv1alpha1.Etcd, peerScheme string, clusterSize int) string {
	domainName := fmt.Sprintf("%s.%s.%s", druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta), etcd.Namespace, "svc")
	serverPort := strconv.Itoa(int(ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)))
	builder := strings.Builder{}
	for i := 0; i < clusterSize; i++ {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, i)
		builder.WriteString(fmt.Sprintf("%s=%s://%s.%s:%s,", podName, peerScheme, podName, domainName, serverPort))
	}
	return strings.Trim(builder.String(), ",")
}

func createPeerConfig(etcd *druidv1alpha1.Etcd, clusterSize int) peerConfig {
	peerScheme, peerSecurityConfig := getSchemeAndSecurityConfig(etcd.Spec.Etcd.PeerUrlTLS, common.VolumeMountPathEtcdPeerCA, common.VolumeMountPathEtcdPeerServerTLS)
	peerSvcName := druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta)
	pc := peerConfig{
		serviceName:       peerSvcName,
		initialCluster:    prepareInitialCluster(etcd, peerScheme, clusterSize),
		listenPeerUrls:    fmt.Sprintf("%s://0.0.0.0:%d", peerScheme, ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)),
		advertisePeerUrls: fmt.Sprintf("%s@%s@%s@%d", peerScheme, peerSvcName, etcd.Namespace, ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)),
	}
	if peerSecurityConfig != nil {
		pc.peerSecurity = peerSecurityConfig
	}
	return pc
}

func getDBQuotaBytes(etcd *druidv1alpha1.Etcd) int64 {
	dbQuotaBytes := defaultDBQuotaBytes
	if etcd.Spec.Etcd.Quota != nil {
		dbQuotaBytes = etcd.Spec.Etcd.Quota.Value()
	}
	return dbQuotaBytes
}

func getSchemeAndSecurityConfig(tlsConfig *druidv1alpha1.TLSConfig, caPath, serverTLSPath string) (string, *securityConfig) {
	if tlsConfig != nil {
		const defaultTLSCASecretKey = "ca.crt"
		return "https", &securityConfig{
			CertFile:       fmt.Sprintf("%s/tls.crt", serverTLSPath),
			KeyFile:        fmt.Sprintf("%s/tls.key", serverTLSPath),
			ClientCertAuth: true,
			TrustedCAFile:  fmt.Sprintf("%s/%s", caPath, ptr.Deref(tlsConfig.TLSCASecretRef.DataKey, defaultTLSCASecretKey)),
			AutoTLS:        false,
		}
	}
	return "http", nil
}

func isPeerUrlTLSAlreadyConfigured(etcdCfg etcdConfig) bool {
	initialCluster := etcdCfg.InitialCluster
	splits := strings.Split(initialCluster, ",")
	keyValue := strings.Split(splits[0], "=")
	return strings.HasPrefix(keyValue[1], "https")
}
