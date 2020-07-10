// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/gardener/etcd-backup-restore/pkg/snapstore"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	backupRestorePortName            = "backuprestore"
	tlsCertFile                      = "tls.crt"
	tlsKeyFile                       = "tls.key"
	caCertFile                       = "ca.crt"
	onDemandFullSnapshotEndpointPath = "snapshot/full"
)

func (r *EtcdReconciler) takeFullSnapshot(ctx context.Context, etcd *druidv1alpha1.Etcd, svc *v1.Service) error {
	httpClient, serviceEndpoint, err := r.getBackupRestoreHTTPClient(ctx, etcd, svc)
	if err != nil {
		return err
	}

	logger.Infof("Taking full snapshot of etcd")

	resp, err := r.makeBackupRestoreHTTPCall(ctx, httpClient, serviceEndpoint, onDemandFullSnapshotEndpointPath)
	if err != nil {
		return err
	}

	var snapshot *snapstore.Snapshot
	if err := json.Unmarshal([]byte(resp), &snapshot); err != nil {
		return err
	}
	if snapshot.Kind != snapstore.SnapshotKindFull {
		return fmt.Errorf("full snapshot failed")
	}

	logger.Infof("Took full snapshot at %s", path.Join(snapshot.SnapDir, snapshot.SnapName))
	return nil
}

func (r *EtcdReconciler) getBackupRestoreHTTPClient(ctx context.Context, etcd *druidv1alpha1.Etcd, svc *v1.Service) (*http.Client, string, error) {
	httpPort := int32(-1)
	for _, port := range svc.Spec.Ports {
		if port.Name == backupRestorePortName {
			httpPort = port.Port
			break
		}
	}
	if httpPort == -1 {
		return nil, "", fmt.Errorf("%s port not found in service %s", backupRestorePortName, svc.Name)
	}
	endpoint := fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace)

	httpClient := &http.Client{
		Timeout: DefaultTimeout,
	}
	httpScheme := "http"

	if etcd.Spec.Etcd.TLS != nil {
		tlsConfig, err := r.getClientTLSConfig(ctx, etcd)
		if err != nil {
			return nil, "", err
		}

		httpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		httpScheme = "https"
	}

	serviceEndpoint := fmt.Sprintf("%s://%s:%d", httpScheme, endpoint, httpPort)
	return httpClient, serviceEndpoint, nil
}

func (r *EtcdReconciler) getClientTLSConfig(ctx context.Context, etcd *druidv1alpha1.Etcd) (*tls.Config, error) {
	var tlsCert, tlsKey, caCert []byte
	etcdClientTLSSecretRef := &etcd.Spec.Etcd.TLS.ClientTLSSecretRef

	etcdClientTLSSecret := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: etcdClientTLSSecretRef.Name, Namespace: etcd.Namespace}, etcdClientTLSSecret); err != nil {
		return nil, err
	}

	// TODO: Should we consider this in validation webhook?
	tlsCert, ok := etcdClientTLSSecret.Data[tlsCertFile]
	if !ok {
		return nil, fmt.Errorf("\"%s\" not present in etcd TLS secret", tlsCertFile)
	}
	tlsKey, ok = etcdClientTLSSecret.Data[tlsKeyFile]
	if !ok {
		return nil, fmt.Errorf("\"%s\" not present in etcd TLS secret", tlsKeyFile)
	}
	caCert, ok = etcdClientTLSSecret.Data[caCertFile]
	if !ok {
		return nil, fmt.Errorf("\"%s\" not present in etcd TLS secret", caCert)
	}

	cert, err := tls.X509KeyPair(tlsCert, tlsKey)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}
	tlsConfig.BuildNameToCertificate()

	return tlsConfig, nil
}

func (r *EtcdReconciler) makeBackupRestoreHTTPCall(ctx context.Context, httpClient *http.Client, serviceEndpoint, path string) ([]byte, error) {
	if deadline, ok := ctx.Deadline(); ok {
		httpClient.Timeout = deadline.Sub(time.Now())
	}

	resp, err := httpClient.Get(fmt.Sprintf("%s/%s", serviceEndpoint, path))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}
