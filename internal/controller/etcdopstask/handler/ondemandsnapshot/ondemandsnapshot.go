// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondemandsnapshot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	utils "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrEtcdNotReady represents the error in case etcd is not ready
	ErrEtcdNotReady druidapicommon.ErrorCode = "ERR_ETCD_NOT_READY"
	// ErrBackupNotEnabled represents the error in case backup is not enabled
	ErrBackupNotEnabled druidapicommon.ErrorCode = "ERR_BACKUP_NOT_ENABLED"
	// ErrCreateHTTPRequest represents the error in case of failure in creating http request
	ErrCreateHTTPRequest druidapicommon.ErrorCode = "ERR_CREATE_HTTP_REQUEST"
	// ErrExecuteHTTPRequest represents the error in case of failure in executing http request
	ErrExecuteHTTPRequest druidapicommon.ErrorCode = "ERR_EXECUTE_HTTP_REQUEST"
	// ErrCreateSnapshot represents the error in case of failure in creating snapshot
	ErrCreateSnapshot druidapicommon.ErrorCode = "ERR_CREATE_SNAPSHOT"
	// ErrOperationCancelled represents the error when snapshot operation is cancelled
	ErrOperationCancelled druidapicommon.ErrorCode = "ERR_OPERATION_CANCELLED"
)

// Handler implements the task.Handler interface for handling on-demand snapshot tasks.
type handler struct {
	k8sClient     client.Client
	etcdReference types.NamespacedName
	httpClient    http.Client
	config        druidv1alpha1.OnDemandSnapshotConfig
}

// New creates a new instance of OnDemandSnapshotTask with an optional HTTP client.
func New(k8sClient client.Client, task *druidv1alpha1.EtcdOpsTask, httpClient *http.Client) (taskhandler.Handler, error) {
	etcdRef := task.GetEtcdReference()

	var snapshotTimeout int32
	if task.Spec.Config.OnDemandSnapshot.Type == druidv1alpha1.OnDemandSnapshotTypeFull {
		snapshotTimeout = *task.Spec.Config.OnDemandSnapshot.TimeoutSecondsFull
	} else {
		snapshotTimeout = *task.Spec.Config.OnDemandSnapshot.TimeoutSecondsDelta
	}

	return &handler{
		k8sClient:     k8sClient,
		etcdReference: etcdRef,
		httpClient:    ptr.Deref(httpClient, http.Client{Timeout: time.Second * time.Duration(snapshotTimeout)}),
		config:        *task.Spec.Config.OnDemandSnapshot,
	}, nil
}

// Admit checks if the task can be admitted for execution.
func (h *handler) Admit(ctx context.Context) taskhandler.Result {
	etcd, errResult := utils.GetEtcd(ctx, h.k8sClient, h.etcdReference, druidv1alpha1.LastOperationTypeAdmit)
	if errResult != nil {
		return *errResult
	}

	result := h.checkPrerequisitesForSnapshot(etcd, druidv1alpha1.LastOperationTypeAdmit)
	if result != nil {
		return *result
	}
	return taskhandler.Result{
		Description: "Admit check passed",
		Requeue:     false,
	}
}

// Run executes the on-demand snapshot task.
func (h *handler) Execute(ctx context.Context) taskhandler.Result {
	etcd, errResult := utils.GetEtcd(ctx, h.k8sClient, h.etcdReference, druidv1alpha1.LastOperationTypeExecution)
	if errResult != nil {
		return *errResult
	}

	errResult = h.checkPrerequisitesForSnapshot(etcd, druidv1alpha1.LastOperationTypeExecution)
	if errResult != nil {
		return *errResult
	}
	
	httpClient, httpScheme, errResult := utils.ConfigureHTTPClientForEtcdBR(ctx, h.k8sClient, etcd, h.httpClient, druidv1alpha1.LastOperationTypeExecution)
	if errResult != nil {
		return *errResult
	}
	h.httpClient = httpClient

	url := fmt.Sprintf("%s://%s.%s:%d/snapshot/%s", httpScheme, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), etcd.Namespace, ptr.Deref(etcd.Spec.Backup.Port, common.DefaultPortEtcdBackupRestore), h.config.Type)
	if ptr.Deref(h.config.IsFinal, false) {
		url += "?final=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return taskhandler.Result{
			Description: "Failed to create HTTP request",
			Error:       druiderr.WrapError(err, ErrCreateHTTPRequest, string(druidv1alpha1.LastOperationTypeExecution), "failed to create HTTP request"),
			Requeue:     true,
		}
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return taskhandler.Result{
			Description: "Failed to execute HTTP request",
			Error:       druiderr.WrapError(err, ErrExecuteHTTPRequest, string(druidv1alpha1.LastOperationTypeExecution), "failed to execute HTTP request"),
			Requeue:     true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return taskhandler.Result{
			Description: "Failed to create snapshot",
			Error:       druiderr.WrapError(fmt.Errorf("failed to create snapshot, status code: %d", resp.StatusCode), ErrCreateSnapshot, string(druidv1alpha1.LastOperationTypeExecution), "failed to create snapshot"),
			Requeue:     false,
		}
	}

	if h.config.Type == druidv1alpha1.OnDemandSnapshotTypeDelta {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return taskhandler.Result{
				Description: "Failed to read response body",
				Error:       druiderr.WrapError(err, ErrCreateSnapshot, string(druidv1alpha1.LastOperationTypeExecution), "failed to read response body"),
				Requeue:     true,
			}
		}

		bodyStr := string(bodyBytes)

		if bodyStr == "null" {
			return taskhandler.Result{
				Description: "Delta snapshot was skipped by backup-restore server",
				Requeue:     false,
			}
		}
	}

	return taskhandler.Result{
		Description: "Snapshot created successfully",
		Requeue:     false,
	}

}

// Cleanup performs any necessary cleanup after the task is completed.
func (h *handler) Cleanup(_ context.Context) taskhandler.Result {
	return taskhandler.Result{
		Description: "Cleanup completed",
		Requeue:     false,
	}
}

// checkPrerequisitesForSnapshot checks whether the etcd meets the prerequisites for taking an on-demand snapshot.
func (h *handler) checkPrerequisitesForSnapshot(etcd *druidv1alpha1.Etcd, phase druidapicommon.LastOperationType) (errResult *taskhandler.Result) {
	if !etcd.IsBackupStoreEnabled() {
		errResult = &taskhandler.Result{
			Description: "Backup is not enabled for etcd",
			Error:       druiderr.WrapError(fmt.Errorf("backup is not enabled for etcd"), ErrBackupNotEnabled, string(phase), "backup is not enabled for etcd"),
			Requeue:     false,
		}
		return
	}

	if !etcd.IsReady() {
		errResult = &taskhandler.Result{
			Description: "Etcd is not ready",
			Error:       druiderr.WrapError(fmt.Errorf("etcd is not ready"), ErrEtcdNotReady, string(phase), "etcd is not ready"),
			Requeue:     false,
		}
		return
	}

	return nil
}
