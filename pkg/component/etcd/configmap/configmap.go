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
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/gardener/pkg/controllerutils"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/utils"
)

type component struct {
	client    client.Client
	namespace string

	values *Values
}

const (
	clientServerType = "client"
	peerServerType   = "peer"
)

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

func generateTLSConfig(serverType, namespace, svcName, port string, tlsConfig *druidv1alpha1.TLSConfig) (etcdConfigYaml string) {
	var protocol = "http"
	if tlsConfig != nil {
		protocol = "https"
		tlsCASecretKey := "ca.crt"
		if dataKey := tlsConfig.TLSCASecretRef.DataKey; dataKey != nil {
			tlsCASecretKey = *dataKey
		}
		etcdConfigYaml = `
    ` + serverType + `-transport-security:
        # Path to the ` + serverType + ` server TLS cert file.
        cert-file: /var/etcd/ssl/` + serverType + `/server/tls.crt

        # Path to the ` + serverType + ` server TLS key file.
        key-file: /var/etcd/ssl/` + serverType + `/server/tls.key

        # Enable ` + serverType + ` cert authentication.
        client-cert-auth: true

        # Path to the ` + serverType + ` server TLS trusted CA cert file.
        trusted-ca-file: /var/etcd/ssl/` + serverType + `/ca/` + tlsCASecretKey + `

        # ` + strings.Title(serverType) + ` TLS using generated certificates
        auto-tls: false`

	}
	advertiseURL := `advertise-` + serverType
	if serverType == peerServerType {
		advertiseURL = "initial-" + advertiseURL
	}
	etcdConfigYaml += `
    # List of comma separated URLs to listen on for ` + serverType + ` traffic.
    listen-` + serverType + `-urls: ` + protocol + `://0.0.0.0:` + port + `

    # List of this member's ` + serverType + ` URLs to advertise to the public.
    # The URLs needed to be a comma-separated list.
    ` + advertiseURL + `-urls: ` + protocol + `@` + svcName + `@` + namespace + `@` + port
	return
}

func (c *component) getEtcdConfigYaml() (etcdConfigYaml string) {
	metricsLevel := druidv1alpha1.Basic
	if c.values.Metrics != nil {
		metricsLevel = *c.values.Metrics
	}

	var quota int64 = 8 * 1024 * 1024 * 1024 // 8Gi
	if c.values.Quota != nil {
		quota = c.values.Quota.Value()
	}

	clientPort := strconv.Itoa(int(pointer.Int32Deref(c.values.ClientPort, defaultClientPort)))
	serverPort := strconv.Itoa(int(pointer.Int32Deref(c.values.ServerPort, defaultServerPort)))

	autoCompactionMode := "periodic"
	if c.values.AutoCompactionMode != nil {
		autoCompactionMode = string(*c.values.AutoCompactionMode)
	}

	autoCompactionRetention := "30m"
	if c.values.AutoCompactionRetention != nil {
		autoCompactionRetention = string(*c.values.AutoCompactionRetention)
	}

	etcdConfigYaml = `# Human-readable name for this member.
    name: ` + fmt.Sprintf("etcd-%s", c.values.EtcdUID[:6]) + `
    # Path to the data directory.
    data-dir: /var/etcd/data/new.etcd
    # metrics configuration
    metrics: ` + string(metricsLevel) + `
    # Number of committed transactions to trigger a snapshot to disk.
    snapshot-count: 75000

    # Accept etcd V2 client requests
    enable-v2: false
    # Raise alarms when backend size exceeds the given quota. 0 means use the
    # default quota.
    quota-backend-bytes: ` + fmt.Sprint(quota) +
		generateTLSConfig(clientServerType, c.namespace, c.values.PeerServiceName, clientPort, c.values.ClientUrlTLS) +
		generateTLSConfig(peerServerType, c.namespace, c.values.PeerServiceName, serverPort, c.values.PeerUrlTLS) + `

    # Initial cluster token for the etcd cluster during bootstrap.
    initial-cluster-token: etcd-cluster

    # Initial cluster state ('new' or 'existing').
    initial-cluster-state: new

    # Initial cluster
    initial-cluster: ` + c.values.InitialCluster + `

    # auto-compaction-mode ("periodic" or "revision").
    auto-compaction-mode: ` + autoCompactionMode + `
    # auto-compaction-retention defines Auto compaction retention length for etcd.
    auto-compaction-retention: ` + autoCompactionRetention
	return
}

func (c *component) syncConfigmap(ctx context.Context, cm *corev1.ConfigMap) error {
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, c.client, cm, func() error {
		cm.Name = c.values.Name
		cm.Namespace = c.namespace
		cm.Labels = c.values.Labels
		cm.OwnerReferences = []metav1.OwnerReference{c.values.OwnerReference}
		cm.Data = map[string]string{"etcd.conf.yaml": c.getEtcdConfigYaml()}
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

func (c *component) Destroy(ctx context.Context) error {
	configMap := c.emptyConfigmap()

	if err := c.deleteConfigmap(ctx, configMap); err != nil {
		return err
	}
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
