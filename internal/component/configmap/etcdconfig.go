// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package configmap

import (
	"fmt"
	"strconv"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
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
	defaultSnapshotCount   = int64(10000)
	advertiseURLTypePeer   = "peer"
	advertiseURLTypeClient = "client"
)

var (
	defaultDataDir = fmt.Sprintf("%s/new.etcd", common.VolumeMountPathEtcdData)
)

type etcdConfig struct {
	Name                    string                       `json:"name"`
	DataDir                 string                       `json:"data-dir"`
	Metrics                 druidv1alpha1.MetricsLevel   `json:"metrics"`
	SnapshotCount           int64                        `json:"snapshot-count"`
	EnableV2                bool                         `json:"enable-v2"`
	EnableGRPCGateway       bool                         `json:"enable-grpc-gateway"`
	QuotaBackendBytes       int64                        `json:"quota-backend-bytes"`
	InitialClusterToken     string                       `json:"initial-cluster-token"`
	InitialClusterState     string                       `json:"initial-cluster-state"`
	InitialCluster          string                       `json:"initial-cluster"`
	AutoCompactionMode      druidv1alpha1.CompactionMode `json:"auto-compaction-mode"`
	AutoCompactionRetention string                       `json:"auto-compaction-retention"`
	ListenPeerUrls          string                       `json:"listen-peer-urls"`
	ListenClientUrls        string                       `json:"listen-client-urls"`
	AdvertisePeerUrls       map[string][]string          `json:"initial-advertise-peer-urls"`
	AdvertiseClientUrls     map[string][]string          `json:"advertise-client-urls"`
	ClientSecurity          *securityConfig              `json:"client-transport-security,omitempty"`
	PeerSecurity            *securityConfig              `json:"peer-transport-security,omitempty"`
	//TODO: (@Shreyas-s14): remove this field once etcd 3.5.26 is the minimum supported version.
	NextClusterVersionCompatible bool `json:"next-cluster-version-compatible,omitempty"`
}

type securityConfig struct {
	CertFile       string `json:"cert-file,omitempty"`
	KeyFile        string `json:"key-file,omitempty"`
	ClientCertAuth bool   `json:"client-cert-auth,omitempty"`
	TrustedCAFile  string `json:"trusted-ca-file,omitempty"`
	AutoTLS        bool   `json:"auto-tls"`
}

func createEtcdConfig(etcd *druidv1alpha1.Etcd) *etcdConfig {
	clientScheme, clientSecurityConfig := getSchemeAndSecurityConfig(etcd.Spec.Etcd.ClientUrlTLS, common.VolumeMountPathEtcdCA, common.VolumeMountPathEtcdServerTLS)
	peerScheme, peerSecurityConfig := getSchemeAndSecurityConfig(etcd.Spec.Etcd.PeerUrlTLS, common.VolumeMountPathEtcdPeerCA, common.VolumeMountPathEtcdPeerServerTLS)
	peerSvcName := druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta)
	cfg := &etcdConfig{
		Name:                         "etcd-config",
		DataDir:                      defaultDataDir,
		Metrics:                      ptr.Deref(etcd.Spec.Etcd.Metrics, druidv1alpha1.Basic),
		SnapshotCount:                getSnapshotCount(etcd),
		EnableV2:                     false,
		EnableGRPCGateway:            ptr.Deref(etcd.Spec.Etcd.EnableGRPCGateway, false),
		QuotaBackendBytes:            getDBQuotaBytes(etcd),
		InitialClusterToken:          defaultInitialClusterToken,
		InitialClusterState:          defaultInitialClusterState,
		InitialCluster:               prepareInitialCluster(etcd, peerScheme),
		AutoCompactionMode:           ptr.Deref(etcd.Spec.Common.AutoCompactionMode, druidv1alpha1.Periodic),
		AutoCompactionRetention:      ptr.Deref(etcd.Spec.Common.AutoCompactionRetention, defaultAutoCompactionRetention),
		ListenPeerUrls:               fmt.Sprintf("%s://0.0.0.0:%d", peerScheme, ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)),
		ListenClientUrls:             fmt.Sprintf("%s://0.0.0.0:%d", clientScheme, ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)),
		AdvertisePeerUrls:            getAdvertiseURLs(etcd, advertiseURLTypePeer, peerScheme, peerSvcName),
		AdvertiseClientUrls:          getAdvertiseURLs(etcd, advertiseURLTypeClient, clientScheme, peerSvcName),
		NextClusterVersionCompatible: true,
	}
	cfg.PeerSecurity = peerSecurityConfig
	cfg.ClientSecurity = clientSecurityConfig

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
	for i := range int(etcd.Spec.Replicas) {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, i)
		builder.WriteString(fmt.Sprintf("%s=%s://%s.%s:%s,", podName, peerScheme, podName, domainName, serverPort))
	}
	return strings.Trim(builder.String(), ",")
}

func getAdvertiseURLs(etcd *druidv1alpha1.Etcd, advertiseURLType, scheme, peerSvcName string) map[string][]string {
	var port int32
	switch advertiseURLType {
	case advertiseURLTypePeer:
		port = ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)
	case advertiseURLTypeClient:
		port = ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)
	default:
		return nil
	}
	advUrlsMap := make(map[string][]string)
	for i := range int(etcd.Spec.Replicas) {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, i)
		advUrlsMap[podName] = []string{fmt.Sprintf("%s://%s.%s.%s.svc:%d", scheme, podName, peerSvcName, etcd.Namespace, port)}
	}
	return advUrlsMap
}
