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
	defaultSnapshotCount = int64(75000)
	commTypePeer         = "peer" // communication type
	commTypeClient       = "client"
)

var (
	defaultDataDir = fmt.Sprintf("%s/new.etcd", common.VolumeMountPathEtcdData)
)

type advertiseURLs map[string][]string

type etcdConfig struct {
	Name                    string                       `yaml:"name"`
	DataDir                 string                       `yaml:"data-dir"`
	Metrics                 druidv1alpha1.MetricsLevel   `yaml:"metrics"`
	SnapshotCount           int64                        `yaml:"snapshot-count"`
	EnableV2                bool                         `yaml:"enable-v2"`
	QuotaBackendBytes       int64                        `yaml:"quota-backend-bytes"`
	InitialClusterToken     string                       `yaml:"initial-cluster-token"`
	InitialClusterState     string                       `yaml:"initial-cluster-state"`
	InitialCluster          string                       `yaml:"initial-cluster"`
	AutoCompactionMode      druidv1alpha1.CompactionMode `yaml:"auto-compaction-mode"`
	AutoCompactionRetention string                       `yaml:"auto-compaction-retention"`
	ListenPeerUrls          string                       `yaml:"listen-peer-urls"`
	ListenClientUrls        string                       `yaml:"listen-client-urls"`
	AdvertisePeerUrls       advertiseURLs                `yaml:"initial-advertise-peer-urls"`
	AdvertiseClientUrls     advertiseURLs                `yaml:"advertise-client-urls"`
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

func createEtcdConfig(etcd *druidv1alpha1.Etcd) *etcdConfig {
	clientScheme, clientSecurityConfig := getSchemeAndSecurityConfig(etcd.Spec.Etcd.ClientUrlTLS, common.VolumeMountPathEtcdCA, common.VolumeMountPathEtcdServerTLS)
	peerScheme, peerSecurityConfig := getSchemeAndSecurityConfig(etcd.Spec.Etcd.PeerUrlTLS, common.VolumeMountPathEtcdPeerCA, common.VolumeMountPathEtcdPeerServerTLS)
	peerSvcName := druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta)
	cfg := &etcdConfig{
		Name:                    fmt.Sprintf("etcd-%s", etcd.UID[:6]),
		DataDir:                 defaultDataDir,
		Metrics:                 ptr.Deref(etcd.Spec.Etcd.Metrics, druidv1alpha1.Basic),
		SnapshotCount:           getSnapshotCount(etcd),
		EnableV2:                false,
		QuotaBackendBytes:       getDBQuotaBytes(etcd),
		InitialClusterToken:     defaultInitialClusterToken,
		InitialClusterState:     defaultInitialClusterState,
		InitialCluster:          prepareInitialCluster(etcd, peerScheme),
		AutoCompactionMode:      ptr.Deref(etcd.Spec.Common.AutoCompactionMode, druidv1alpha1.Periodic),
		AutoCompactionRetention: ptr.Deref(etcd.Spec.Common.AutoCompactionRetention, defaultAutoCompactionRetention),
		ListenPeerUrls:          fmt.Sprintf("%s://0.0.0.0:%d", peerScheme, ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)),
		ListenClientUrls:        fmt.Sprintf("%s://0.0.0.0:%d", clientScheme, ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)),
		AdvertisePeerUrls:       getAdvertiseURLs(etcd, commTypePeer, peerScheme, peerSvcName),
		AdvertiseClientUrls:     getAdvertiseURLs(etcd, commTypeClient, clientScheme, peerSvcName),
	}
	if peerSecurityConfig != nil {
		cfg.PeerSecurity = *peerSecurityConfig
	}
	if clientSecurityConfig != nil {
		cfg.ClientSecurity = *clientSecurityConfig
	}

	return cfg
}

func getSnapshotCount(etcd *druidv1alpha1.Etcd) int64 {
	if etcd.Spec.Etcd.SnapshotCount != nil {
		return *etcd.Spec.Etcd.SnapshotCount
	}
	return defaultSnapshotCount
}

func getDBQuotaBytes(etcd *druidv1alpha1.Etcd) int64 {
	if etcd.Spec.Etcd.Quota != nil {
		return etcd.Spec.Etcd.Quota.Value()
	}
	return defaultDBQuotaBytes
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

func prepareInitialCluster(etcd *druidv1alpha1.Etcd, peerScheme string) string {
	domainName := fmt.Sprintf("%s.%s.%s", druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta), etcd.Namespace, "svc")
	serverPort := strconv.Itoa(int(ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)))
	builder := strings.Builder{}
	for i := 0; i < int(etcd.Spec.Replicas); i++ {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, i)
		builder.WriteString(fmt.Sprintf("%s=%s://%s.%s:%s,", podName, peerScheme, podName, domainName, serverPort))
	}
	return strings.Trim(builder.String(), ",")
}

func getAdvertiseURLs(etcd *druidv1alpha1.Etcd, commType, scheme, peerSvcName string) advertiseURLs {
	var port int32
	switch commType {
	case commTypePeer:
		port = ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)
	case commTypeClient:
		port = ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)
	default:
		return nil
	}
	advUrlsMap := make(map[string][]string)
	for i := 0; i < int(etcd.Spec.Replicas); i++ {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, i)
		advUrlsMap[podName] = []string{fmt.Sprintf("%s://%s.%s.%s.svc:%d", scheme, podName, peerSvcName, etcd.Namespace, port)}
	}
	return advUrlsMap
}
