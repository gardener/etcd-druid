// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package configmap

import (
	"context"
	"encoding/json"
	"fmt"

	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils"
)

type component struct {
	client    client.Client
	namespace string

	values *Values
}

// New creates a new configmap deployer instance.
func New(c client.Client, namespace string, values *Values) gardenercomponent.Deployer {
	return &component{
		client:    c,
		namespace: namespace,
		values:    values,
	}
}

func (c *component) Deploy(ctx context.Context) error {
	cm := c.emptyConfigmap()
	if err := c.syncConfigmap(ctx, cm); err != nil {
		return err
	}
	return nil
}

func (c *component) Destroy(ctx context.Context) error {
	configMap := c.emptyConfigmap()
	if err := c.deleteConfigmap(ctx, configMap); err != nil {
		return err
	}
	return nil
}

type etcdTLSTarget string

const (
	clientTLS                  etcdTLSTarget = "client"
	peerTLS                    etcdTLSTarget = "peer"
	defaultQuota                             = int64(8 * 1024 * 1024 * 1024) // 8Gi
	defaultSnapshotCount                     = 75000                         // for more information refer to https://etcd.io/docs/v3.4/op-guide/maintenance/#raft-log-retention
	defaultInitialClusterState               = "new"
	defaultInitialClusterToken               = "etcd-cluster"
)

func (c *component) syncConfigmap(ctx context.Context, cm *corev1.ConfigMap) error {
	peerScheme, peerSecurity := getSchemeAndSecurityConfig(c.values.PeerUrlTLS, peerTLS)
	clientScheme, clientSecurity := getSchemeAndSecurityConfig(c.values.ClientUrlTLS, clientTLS)

	quota := defaultQuota
	if c.values.Quota != nil {
		quota = c.values.Quota.Value()
	}

	autoCompactionMode := string(druidv1alpha1.Periodic)
	if c.values.AutoCompactionMode != nil {
		autoCompactionMode = string(*c.values.AutoCompactionMode)
	}

	etcdConfig := &etcdConfig{
		Name:                    fmt.Sprintf("etcd-%s", c.values.EtcdUID[:6]),
		DataDir:                 "/var/etcd/data/new.etcd",
		Metrics:                 string(druidv1alpha1.Basic),
		SnapshotCount:           defaultSnapshotCount,
		EnableV2:                false,
		QuotaBackendBytes:       quota,
		InitialClusterToken:     defaultInitialClusterToken,
		InitialClusterState:     defaultInitialClusterState,
		InitialCluster:          c.values.InitialCluster,
		AutoCompactionMode:      autoCompactionMode,
		AutoCompactionRetention: pointer.StringDeref(c.values.AutoCompactionRetention, "30m"),

		ListenPeerUrls:      fmt.Sprintf("%s://0.0.0.0:%d", peerScheme, pointer.Int32Deref(c.values.ServerPort, defaultServerPort)),
		ListenClientUrls:    fmt.Sprintf("%s://0.0.0.0:%d", clientScheme, pointer.Int32Deref(c.values.ClientPort, defaultClientPort)),
		AdvertisePeerUrls:   fmt.Sprintf("%s@%s@%s@%d", peerScheme, c.values.PeerServiceName, c.namespace, pointer.Int32Deref(c.values.ServerPort, defaultServerPort)),
		AdvertiseClientUrls: fmt.Sprintf("%s@%s@%s@%d", clientScheme, c.values.PeerServiceName, c.namespace, pointer.Int32Deref(c.values.ClientPort, defaultClientPort)),
	}

	if clientSecurity != nil {
		etcdConfig.ClientSecurity = *clientSecurity
	}

	if peerSecurity != nil {
		etcdConfig.PeerSecurity = *peerSecurity
	}

	etcdConfigYaml, err := yaml.Marshal(etcdConfig)
	if err != nil {
		return err
	}

	_, err = controllerutils.GetAndCreateOrMergePatch(ctx, c.client, cm, func() error {
		cm.Name = c.values.Name
		cm.Namespace = c.namespace
		cm.Labels = c.values.Labels
		cm.OwnerReferences = []metav1.OwnerReference{c.values.OwnerReference}
		cm.Data = map[string]string{"etcd.conf.yaml": string(etcdConfigYaml)}
		return nil
	})
	if err != nil {
		return err
	}

	// save the checksum value of generated etcd config
	jsonString, err := json.Marshal(cm.Data)
	if err != nil {
		return err
	}
	c.values.ConfigMapChecksum = utils.ComputeSHA256Hex(jsonString)
	return nil
}

func (c *component) deleteConfigmap(ctx context.Context, cm *corev1.ConfigMap) error {
	return client.IgnoreNotFound(c.client.Delete(ctx, cm))
}

func (c *component) emptyConfigmap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.values.Name,
			Namespace: c.namespace,
		},
	}
}

type etcdConfig struct {
	Name                    string         `yaml:"name"`
	DataDir                 string         `yaml:"data-dir"`
	Metrics                 string         `yaml:"metrics"`
	SnapshotCount           int            `yaml:"snapshot-count"`
	EnableV2                bool           `yaml:"enable-v2"`
	QuotaBackendBytes       int64          `yaml:"quota-backend-bytes"`
	InitialClusterToken     string         `yaml:"initial-cluster-token"`
	InitialClusterState     string         `yaml:"initial-cluster-state"`
	InitialCluster          string         `yaml:"initial-cluster"`
	AutoCompactionMode      string         `yaml:"auto-compaction-mode"`
	AutoCompactionRetention string         `yaml:"auto-compaction-retention"`
	ListenPeerUrls          string         `yaml:"listen-peer-urls"`
	ListenClientUrls        string         `yaml:"listen-client-urls"`
	AdvertisePeerUrls       string         `yaml:"initial-advertise-peer-urls"`
	AdvertiseClientUrls     string         `yaml:"advertise-client-urls"`
	ClientSecurity          securityConfig `yaml:"client-transport-security,omitempty"`
	PeerSecurity            securityConfig `yaml:"peer-transport-security,omitempty"`
}

type securityConfig struct {
	CertFile       string `yaml:"cert-file,omitempty"`
	KeyFile        string `yaml:"key-file,omitempty"`
	ClientCertAuth bool   `yaml:"client-cert-auth,omitempty"`
	TrustedCAFile  string `yaml:"trusted-ca-file,omitempty"`
	AutoTLS        bool   `yaml:"auto-tls"`
}

func getSchemeAndSecurityConfig(tlsConfig *druidv1alpha1.TLSConfig, tlsTarget etcdTLSTarget) (string, *securityConfig) {
	if tlsConfig != nil {
		tlsCASecretKey := "ca.crt"
		if dataKey := tlsConfig.TLSCASecretRef.DataKey; dataKey != nil {
			tlsCASecretKey = *dataKey
		}
		return "https", &securityConfig{
			CertFile:       fmt.Sprintf("/var/etcd/ssl/%s/server/tls.crt", tlsTarget),
			KeyFile:        fmt.Sprintf("/var/etcd/ssl/%s/server/tls.key", tlsTarget),
			ClientCertAuth: true,
			TrustedCAFile:  fmt.Sprintf("/var/etcd/ssl/%s/ca/%s", tlsTarget, tlsCASecretKey),
			AutoTLS:        false,
		}
	}
	return "http", nil
}
