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
	"github.com/gardener/etcd-druid/internal/utils"
	"k8s.io/utils/pointer"
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

type tlsTarget string

const (
	clientTLS tlsTarget = "client"
	peerTLS   tlsTarget = "peer"
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

func createEtcdConfig(etcd *druidv1alpha1.Etcd) *etcdConfig {
	clientScheme, clientSecurityConfig := getSchemeAndSecurityConfig(etcd.Spec.Etcd.ClientUrlTLS, common.VolumeMountPathEtcdCA, common.VolumeMountPathEtcdServerTLS)
	peerScheme, peerSecurityConfig := getSchemeAndSecurityConfig(etcd.Spec.Etcd.PeerUrlTLS, common.VolumeMountPathEtcdPeerCA, common.VolumeMountPathEtcdPeerServerTLS)
	peerSvcName := druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta)
	cfg := &etcdConfig{
		Name:                    fmt.Sprintf("etcd-%s", etcd.UID[:6]),
		DataDir:                 defaultDataDir,
		Metrics:                 utils.TypeDeref(etcd.Spec.Etcd.Metrics, druidv1alpha1.Basic),
		SnapshotCount:           defaultSnapshotCount,
		EnableV2:                false,
		QuotaBackendBytes:       getDBQuotaBytes(etcd),
		InitialClusterToken:     defaultInitialClusterToken,
		InitialClusterState:     defaultInitialClusterState,
		InitialCluster:          prepareInitialCluster(etcd, peerScheme),
		AutoCompactionMode:      utils.TypeDeref(etcd.Spec.Common.AutoCompactionMode, druidv1alpha1.Periodic),
		AutoCompactionRetention: utils.TypeDeref(etcd.Spec.Common.AutoCompactionRetention, defaultAutoCompactionRetention),
		ListenPeerUrls:          fmt.Sprintf("%s://0.0.0.0:%d", peerScheme, utils.TypeDeref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)),
		ListenClientUrls:        fmt.Sprintf("%s://0.0.0.0:%d", clientScheme, utils.TypeDeref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)),
		AdvertisePeerUrls:       fmt.Sprintf("%s@%s@%s@%d", peerScheme, peerSvcName, etcd.Namespace, utils.TypeDeref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)),
		AdvertiseClientUrls:     fmt.Sprintf("%s@%s@%s@%d", clientScheme, peerSvcName, etcd.Namespace, utils.TypeDeref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)),
	}
	if peerSecurityConfig != nil {
		cfg.PeerSecurity = *peerSecurityConfig
	}
	if clientSecurityConfig != nil {
		cfg.ClientSecurity = *clientSecurityConfig
	}

	return cfg
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
			TrustedCAFile:  fmt.Sprintf("%s/%s", caPath, utils.TypeDeref(tlsConfig.TLSCASecretRef.DataKey, defaultTLSCASecretKey)),
			AutoTLS:        false,
		}
	}
	return "http", nil
}

func prepareInitialCluster(etcd *druidv1alpha1.Etcd, peerScheme string) string {
	domainName := fmt.Sprintf("%s.%s.%s", druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta), etcd.Namespace, "svc")
	serverPort := strconv.Itoa(int(pointer.Int32Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)))
	builder := strings.Builder{}
	for i := 0; i < int(etcd.Spec.Replicas); i++ {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, i)
		builder.WriteString(fmt.Sprintf("%s=%s://%s.%s:%s,", podName, peerScheme, podName, domainName, serverPort))
	}
	return strings.Trim(builder.String(), ",")
}
