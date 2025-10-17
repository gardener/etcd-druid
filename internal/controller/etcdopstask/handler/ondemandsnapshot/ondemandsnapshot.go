// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondemandsnapshot

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetEtcd represents the error in case of fetching etcd object. Reason could be internal error
	ErrGetEtcd druidv1alpha1.ErrorCode = "ERR_GET_ETCD"
	// ErrEtcdNotReady represents the error in case etcd is not ready
	ErrEtcdNotReady druidv1alpha1.ErrorCode = "ERR_ETCD_NOT_READY"
	// ErrBackupNotEnabled represents the error in case backup is not enabled
	ErrBackupNotEnabled druidv1alpha1.ErrorCode = "ERR_BACKUP_NOT_ENABLED"
	// ErrGetSecret represents the error in case of failure in fetching the secret
	ErrGetSecret druidv1alpha1.ErrorCode = "ERR_GET_SECRET"
	// ErrCreateHTTPRequest represents the error in case of failure in creating http request
	ErrCreateHTTPRequest druidv1alpha1.ErrorCode = "ERR_CREATE_HTTP_REQUEST"
	// ErrExecuteHTTPRequest represents the error in case of failure in executing http request
	ErrExecuteHTTPRequest druidv1alpha1.ErrorCode = "ERR_EXECUTE_HTTP_REQUEST"
	// ErrCreateSnapshot represents the error in case of failure in creating snapshot
	ErrCreateSnapshot druidv1alpha1.ErrorCode = "ERR_CREATE_SNAPSHOT"
)

// Handler implements the task.Handler interface for handling on-demand snapshot tasks.
type Handler struct {
	client        client.Client
	etcdReference types.NamespacedName
	httpClient    http.Client
	config        druidv1alpha1.OnDemandSnapshotConfig
}

// New creates a new instance of OnDemandSnapshotTask with an optional HTTP client.
func New(k8sClient client.Client, task *druidv1alpha1.EtcdOpsTask, httpClient *http.Client) (handler.Handler, error) {
	etcdRef, err := task.GetEtcdReference()
	if err != nil {
		return nil, fmt.Errorf("failed to get etcd reference: %w", err)
	}

	// Use provided HTTP client or create default
	cl := ptr.Deref(httpClient, http.Client{Timeout: time.Second * time.Duration(*task.Spec.Config.OnDemandSnapshot.TimeoutSeconds)})

	return &Handler{
		client:        k8sClient,
		etcdReference: etcdRef,
		httpClient:    cl,
		config:        *task.Spec.Config.OnDemandSnapshot,
	}, nil
}

// Admit checks if the task can be admitted for execution.
func (h *Handler) Admit(ctx context.Context) handler.Result {
	var etcd druidv1alpha1.Etcd
	if err := h.client.Get(ctx, h.etcdReference, &etcd); err != nil {
		// Check the type of error and return result. If it's a transport layer issue, requeue. If the object is not found, return reject.
		if apierrors.IsNotFound(err) {
			return handler.Result{
				Description: "Admit Operation: Failed to get etcd object",
				Error:       druiderr.WrapError(err, ErrGetEtcd, string(druidv1alpha1.OperationTypeAdmit), "failed to get etcd object"),
				Completed:   true,
			}
		}
		return handler.Result{
			Description: "Admit Operation: Failed to get etcd object due to internal error",
			Error:       druiderr.WrapError(err, ErrGetEtcd, string(druidv1alpha1.OperationTypeAdmit), "failed to get etcd object due to internal error"),
			Completed:   false,
		}
	}

	if !etcd.IsBackupStoreEnabled() {
		return handler.Result{
			Description: "Admit Operation: Backup is not enabled for etcd",
			Error:       druiderr.WrapError(fmt.Errorf("backup is not enabled for etcd"), ErrBackupNotEnabled, string(druidv1alpha1.OperationTypeAdmit), "backup is not enabled for etcd"),
			Completed:   true,
		}
	}

	if err := etcd.IsReady(); err != nil {
		return handler.Result{
			Description: "Admit Operation: Etcd is not ready",
			Error:       druiderr.WrapError(err, ErrEtcdNotReady, string(druidv1alpha1.OperationTypeAdmit), "etcd is not ready"),
			Completed:   true,
		}
	}
	return handler.Result{
		Description: "Admit check passed",
		Completed:   true,
	}
}

// Run executes the on-demand snapshot task.
func (h *Handler) Run(ctx context.Context) handler.Result {
	etcd := &druidv1alpha1.Etcd{}
	if err := h.client.Get(ctx, h.etcdReference, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return handler.Result{
				Description: "Run Operation: Failed to get etcd object - object not found",
				Error:       druiderr.WrapError(err, ErrGetEtcd, string(druidv1alpha1.OperationTypeRunning), "failed to get etcd object - object not found"),
				Completed:   true,
			}
		}
		return handler.Result{
			Description: "Run Operation: Failed to get etcd object due to transient error",
			Error:       druiderr.WrapError(err, ErrGetEtcd, string(druidv1alpha1.OperationTypeRunning), "failed to get etcd object due to transient error"),
			Completed:   false,
		}
	}
	if err := etcd.IsReady(); err != nil {
		return handler.Result{
			Description: "Run Operation: Etcd is not ready",
			Error:       druiderr.WrapError(err, ErrEtcdNotReady, string(druidv1alpha1.OperationTypeRunning), "etcd is not ready"),
			Completed:   true,
		}
	}
	var httpScheme string
	if tlsConfig := etcd.Spec.Backup.TLS; tlsConfig != nil {
		httpScheme = "https"
		etcdbrCASecret := &v1.Secret{}
		dataKey := ptr.Deref(tlsConfig.TLSCASecretRef.DataKey, "bundle.crt")
		if err := h.client.Get(ctx, types.NamespacedName{Namespace: etcd.Namespace, Name: tlsConfig.TLSCASecretRef.Name}, etcdbrCASecret); err != nil {
			return handler.Result{
				Description: "Run Operation: Failed to get etcdbr CA secret",
				Error:       druiderr.WrapError(err, ErrGetSecret, string(druidv1alpha1.OperationTypeRunning), fmt.Sprintf("failed to get etcdbr CA secret %s/%s", etcd.Namespace, tlsConfig.TLSCASecretRef.Name)),
				Completed:   false,
			}
		}
		certData, ok := etcdbrCASecret.Data[dataKey]
		if !ok {
			return handler.Result{
				Description: "Run Operation: CA cert data key not found in secret",
				Error:       druiderr.WrapError(fmt.Errorf("CA cert data key %q not found in secret %s/%s", dataKey, etcdbrCASecret.Namespace, etcdbrCASecret.Name), ErrGetEtcd, string(druidv1alpha1.OperationTypeRunning), "CA cert data key not found in secret"),
				Completed:   true,
			}
		}
		caCerts := x509.NewCertPool()
		if !caCerts.AppendCertsFromPEM(certData) {
			return handler.Result{
				Description: "Run Operation: Failed to append CA certs from secret",
				Error:       druiderr.WrapError(fmt.Errorf("failed to append CA certs from secret %s/%s", etcdbrCASecret.Namespace, etcdbrCASecret.Name), ErrGetEtcd, string(druidv1alpha1.OperationTypeRunning), "failed to append CA certs from secret"),
				Completed:   true,
			}
		}
		httpTransport := http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caCerts,
				MinVersion: tls.VersionTLS12,
			},
		}
		h.httpClient = http.Client{
			Timeout:   h.httpClient.Timeout,
			Transport: &httpTransport,
		}
	} else {
		httpScheme = "http"
	}
	url := fmt.Sprintf("%s://%s.%s:%d/snapshot/%s", httpScheme, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), etcd.Namespace, ptr.Deref(etcd.Spec.Backup.Port, common.DefaultPortEtcdBackupRestore), h.config.Type)
	if ptr.Deref(h.config.IsFinal, false) {
		url += "?final=true"
	}
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return handler.Result{
			Description: "Run Operation: Failed to create HTTP request",
			Error:       druiderr.WrapError(err, ErrCreateHTTPRequest, string(druidv1alpha1.OperationTypeRunning), "failed to create HTTP request"),
			Completed:   false,
		}
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return handler.Result{
			Description: "Run Operation: Failed to execute HTTP request",
			Error:       druiderr.WrapError(err, ErrExecuteHTTPRequest, string(druidv1alpha1.OperationTypeRunning), "failed to execute HTTP request"),
			Completed:   false,
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return handler.Result{
			Description: "Run Operation: Failed to create snapshot",
			Error:       druiderr.WrapError(fmt.Errorf("failed to create snapshot, status code: %d", resp.StatusCode), ErrCreateSnapshot, string(druidv1alpha1.OperationTypeRunning), "failed to create snapshot"),
			Completed:   true,
		}
	}
	return handler.Result{
		Description: "Snapshot created successfully",
		Completed:   true,
	}

}

// Cleanup performs any necessary cleanup after the task is completed.
func (h *Handler) Cleanup(_ context.Context) handler.Result {
	return handler.Result{
		Description: "Cleanup completed",
		Completed:   true,
	}
}
