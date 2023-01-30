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

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	"github.com/gardener/gardener/pkg/controllerutils"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	cm := c.emptyConfigmap(c.values.ConfigMapName)

	if err := c.syncConfigmap(ctx, cm); err != nil {
		return err
	}

	return nil
}

func (c *component) syncConfigmap(ctx context.Context, cm *corev1.ConfigMap) error {
	etcdConfigYaml := `# Human-readable name for this member.
    name: ` + fmt.Sprintf("etcd-%s", c.values.EtcdUID[:6]) + `
    # Path to the data directory.
    data-dir: /var/etcd/data/new.etcd`
	metricsLevel := druidv1alpha1.Basic
	if c.values.Metrics != nil {
		metricsLevel = *c.values.Metrics
	}

	etcdConfigYaml = etcdConfigYaml + `
    # metrics configuration
    metrics: ` + string(metricsLevel) + `
    # Number of committed transactions to trigger a snapshot to disk.
    snapshot-count: 75000

    # Accept etcd V2 client requests
    enable-v2: false`
	var quota int64 = 8 * 1024 * 1024 * 1024 // 8Gi
	if c.values.Quota != nil {
		quota = c.values.Quota.Value()
	}

	etcdConfigYaml = etcdConfigYaml + `
    # Raise alarms when backend size exceeds the given quota. 0 means use the
    # default quota.
    quota-backend-bytes: ` + fmt.Sprint(quota)

	enableTLS := (c.values.ClientUrlTLS != nil)

	protocol := "http"
	if enableTLS {
		protocol = "https"
		tlsCASecretKey := "ca.crt"
		if dataKey := c.values.ClientUrlTLS.TLSCASecretRef.DataKey; dataKey != nil {
			tlsCASecretKey = *dataKey
		}
		etcdConfigYaml = etcdConfigYaml + `
    client-transport-security:
        # Path to the client server TLS cert file.
        cert-file: /var/etcd/ssl/client/server/tls.crt

        # Path to the client server TLS key file.
        key-file: /var/etcd/ssl/client/server/tls.key

        # Enable client cert authentication.
        client-cert-auth: true

        # Path to the client server TLS trusted CA cert file.
        trusted-ca-file: /var/etcd/ssl/client/ca/` + tlsCASecretKey + `

        # Client TLS using generated certificates
        auto-tls: false`
	}

	clientPort := strconv.Itoa(int(pointer.Int32Deref(c.values.ClientPort, defaultClientPort)))

	serverPort := strconv.Itoa(int(pointer.Int32Deref(c.values.ServerPort, defaultServerPort)))

	etcdConfigYaml = etcdConfigYaml + `
    # List of comma separated URLs to listen on for client traffic.
    listen-client-urls: ` + protocol + `://0.0.0.0:` + clientPort + `

    # List of this member's client URLs to advertise to the public.
    # The URLs needed to be a comma-separated list.
    advertise-client-urls: ` + protocol + `@` + c.values.PeerServiceName + `@` + c.values.EtcdNameSpace + `@` + clientPort

	enableTLS = (c.values.PeerUrlTLS != nil)
	protocol = "http"
	if enableTLS {
		protocol = "https"
		tlsCASecretKey := "ca.crt"
		if dataKey := c.values.PeerUrlTLS.TLSCASecretRef.DataKey; dataKey != nil {
			tlsCASecretKey = *dataKey
		}
		etcdConfigYaml = etcdConfigYaml + `
    peer-transport-security:
        # Path to the peer server TLS cert file.
        cert-file: /var/etcd/ssl/peer/server/tls.crt

        # Path to the peer server TLS key file.
        key-file: /var/etcd/ssl/peer/server/tls.key

        # Enable peer cert authentication.
        client-cert-auth: true

        # Path to the peer server TLS trusted CA cert file.
        trusted-ca-file: /var/etcd/ssl/peer/ca/` + tlsCASecretKey + `

        # Peer TLS using generated certificates
        auto-tls: false`
	}

	etcdConfigYaml = etcdConfigYaml + `
    # List of comma separated URLs to listen on for peer traffic.
    listen-peer-urls: ` + protocol + `://0.0.0.0:` + serverPort + `

    # List of this member's peer URLs to advertise to the public.
    # The URLs needed to be a comma-separated list.
    initial-advertise-peer-urls: ` + protocol + `@` + c.values.PeerServiceName + `@` + c.values.EtcdNameSpace + `@` + serverPort + `

    # Initial cluster token for the etcd cluster during bootstrap.
    initial-cluster-token: etcd-cluster

    # Initial cluster state ('new' or 'existing').
    initial-cluster-state: new

    # Initial cluster
    initial-cluster: ` + c.values.InitialCluster + `

    # auto-compaction-mode ("periodic" or "revision").`

	autoCompactionMode := "periodic"
	if c.values.AutoCompactionMode != nil {
		autoCompactionMode = string(*c.values.AutoCompactionMode)
	}
	etcdConfigYaml = etcdConfigYaml + `
    auto-compaction-mode: ` + autoCompactionMode

	autoCompactionRetention := "30m"
	if c.values.AutoCompactionRetention != nil {
		autoCompactionRetention = string(*c.values.AutoCompactionRetention)
	}
	etcdConfigYaml = etcdConfigYaml + `
    # auto-compaction-retention defines Auto compaction retention length for etcd.
    auto-compaction-retention: ` + autoCompactionRetention

	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, c.client, cm, func() error {
		cm.ObjectMeta = getObjectMeta(c.values)
		cm.OwnerReferences = getOwnerReferences(c.values)
		cm.Data = map[string]string{"etcd.conf.yaml": etcdConfigYaml}
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
	configMap := c.emptyConfigmap(c.values.ConfigMapName)

	if err := c.deleteConfigmap(ctx, configMap); err != nil {
		return err
	}
	return nil
}

func (c *component) deleteConfigmap(ctx context.Context, cm *corev1.ConfigMap) error {
	return client.IgnoreNotFound(c.client.Delete(ctx, cm))
}

func (c *component) emptyConfigmap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.namespace,
		},
	}
}

func getObjectMeta(val *Values) metav1.ObjectMeta {
	labels := map[string]string{"name": "etcd", "instance": val.EtcdName}
	for key, value := range val.Labels {
		labels[key] = value
	}
	return metav1.ObjectMeta{
		Name:      val.ConfigMapName,
		Namespace: val.EtcdNameSpace,
		Labels:    labels,
	}
}

func getOwnerReferences(val *Values) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         druidv1alpha1.GroupVersion.String(),
			Kind:               "Etcd",
			Name:               val.EtcdName,
			UID:                val.EtcdUID,
			Controller:         pointer.Bool(true),
			BlockOwnerDeletion: pointer.Bool(true),
		},
	}
}
