// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component/statefulset"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	kutil "github.com/gardener/etcd-druid/internal/utils/kubernetes"

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
	dataKey := ptr.Deref(tlsConfig.TLSCASecretRef.DataKey, "bundle.crt")

	sts, err := kutil.GetStatefulSet(ctx, k8sClient, etcd)
	if err != nil {
		errResult = &taskhandler.Result{
			Description: "Failed to get StatefulSet for backup-restore CA resolution",
			Error:       druiderr.WrapError(err, statefulset.ErrGetStatefulSet, string(phase), fmt.Sprintf("failed to get StatefulSet for etcd %s/%s", etcd.Namespace, etcd.Name)),
			Requeue:     true,
		}
		return
	}
	if sts == nil {
		errResult = &taskhandler.Result{
			Description: fmt.Sprintf("StatefulSet for etcd %s/%s not found or not owned by etcd", etcd.Namespace, etcd.Name),
			Error:       druiderr.WrapError(fmt.Errorf("StatefulSet for etcd %s/%s not found or not owned", etcd.Namespace, etcd.Name), statefulset.ErrGetStatefulSet, string(phase), "resolve backup-restore CA secret from StatefulSet"),
			Requeue:     false,
		}
		return
	}

	brCASecretName, ok := kutil.GetSecretNameFromVolume(sts, common.VolumeNameBackupRestoreCA)
	if !ok {
		errResult = &taskhandler.Result{
			Description: fmt.Sprintf("backup-restore CA volume %q not found on StatefulSet %s/%s", common.VolumeNameBackupRestoreCA, etcd.Namespace, etcd.Name),
			Error:       druiderr.WrapError(fmt.Errorf("volume %q not found on StatefulSet %s/%s", common.VolumeNameBackupRestoreCA, etcd.Namespace, etcd.Name), taskhandler.ErrGetCASecret, string(phase), "resolve backup-restore CA secret from StatefulSet volumes"),
			Requeue:     false,
		}
		return
	}

	brTLSConfig, err := kutil.BuildBackupRestoreCATLSConfig(ctx, k8sClient, sts, etcd.Namespace, dataKey)
	if err != nil {
		errResult = classifyCAResolutionError(err, string(phase), etcd.Namespace, brCASecretName)
		return
	}

	httpTransport := &http.Transport{
		TLSClientConfig: brTLSConfig,
	}

	httpClient = http.Client{
		Timeout:   defaultClient.Timeout,
		Transport: httpTransport,
	}

	return httpClient, httpScheme, nil
}

// classifyCAResolutionError maps a CA resolution error from the shared TLS builder onto
// the EtcdOpsTask error taxonomy, preserving the DruidError code, message, and requeue
// semantics that callers of ConfigureHTTPClientForEtcdBR rely on.
func classifyCAResolutionError(err error, phase, namespace, secretName string) *taskhandler.Result {
	switch {
	case errors.Is(err, kutil.ErrMissingDataKey):
		return &taskhandler.Result{
			Description: "CA cert data key not found in secret",
			Error:       druiderr.WrapError(err, taskhandler.ErrCADataKeyNotFound, phase, "CA cert data key not found in secret"),
			Requeue:     false,
		}
	case errors.Is(err, kutil.ErrAppendCACerts):
		return &taskhandler.Result{
			Description: "Failed to append CA certs from secret",
			Error:       druiderr.WrapError(err, taskhandler.ErrAppendCACerts, phase, fmt.Sprintf("failed to append CA certs from secret %s/%s", namespace, secretName)),
			Requeue:     false,
		}
	case errors.Is(err, kutil.ErrSecretNotFound):
		return &taskhandler.Result{
			Description: "Failed to get etcdbr CA secret",
			Error:       druiderr.WrapError(err, taskhandler.ErrGetCASecret, phase, fmt.Sprintf("failed to get etcdbr CA secret %s/%s", namespace, secretName)),
			Requeue:     false,
		}
	default:
		return &taskhandler.Result{
			Description: "Failed to get etcdbr CA secret",
			Error:       druiderr.WrapError(err, taskhandler.ErrGetCASecret, phase, fmt.Sprintf("failed to get etcdbr CA secret %s/%s", namespace, secretName)),
			Requeue:     true,
		}
	}
}
