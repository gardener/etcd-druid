// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigureHTTPClientForEtcdBR configures the HTTP client with TLS if backup TLS is enabled.
// It returns the configured HTTP client, the HTTP scheme to use, and any error result.
func ConfigureHTTPClientForEtcdBR(ctx context.Context, k8sClient client.Client, etcd *druidv1alpha1.Etcd, defaultClient http.Client, phase druidapicommon.LastOperationType) (httpClient http.Client, httpScheme string, errResult *taskhandler.Result) {
	tlsConfig := etcd.Spec.Backup.TLS
	if tlsConfig == nil {
		return defaultClient, "http", nil
	}

	httpScheme = "https"
	etcdbrCASecret := &v1.Secret{}
	dataKey := ptr.Deref(tlsConfig.TLSCASecretRef.DataKey, "bundle.crt")

	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: etcd.Namespace, Name: tlsConfig.TLSCASecretRef.Name}, etcdbrCASecret); err != nil {
		requeue := true
		if apierrors.IsNotFound(err) {
			requeue = false
		}
		errResult = &taskhandler.Result{
			Description: "Failed to get etcdbr CA secret",
			Error:       druiderr.WrapError(err, taskhandler.ErrGetCASecret, string(phase), fmt.Sprintf("failed to get etcdbr CA secret %s/%s", etcd.Namespace, tlsConfig.TLSCASecretRef.Name)),
			Requeue:     requeue,
		}
		return
	}

	certData, ok := etcdbrCASecret.Data[dataKey]
	if !ok {
		errResult = &taskhandler.Result{
			Description: "CA cert data key not found in secret",
			Error:       druiderr.WrapError(fmt.Errorf("CA cert data key %q not found in secret %s/%s", dataKey, etcdbrCASecret.Namespace, etcdbrCASecret.Name), taskhandler.ErrCADataKeyNotFound, string(phase), "CA cert data key not found in secret"),
			Requeue:     false,
		}
		return
	}

	caCerts := x509.NewCertPool()
	if !caCerts.AppendCertsFromPEM(certData) {
		errResult = &taskhandler.Result{
			Description: "Failed to append CA certs from secret",
			Error:       druiderr.WrapError(fmt.Errorf("failed to append CA certs from secret %s/%s", etcdbrCASecret.Namespace, etcdbrCASecret.Name), taskhandler.ErrAppendCACerts, string(phase), "failed to append CA certs from secret"),
			Requeue:     false,
		}
		return
	}

	httpTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    caCerts,
			MinVersion: tls.VersionTLS12,
		},
	}

	httpClient = http.Client{
		Timeout:   defaultClient.Timeout,
		Transport: httpTransport,
	}

	return httpClient, httpScheme, nil
}
