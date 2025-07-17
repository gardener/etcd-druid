package ondemandsnapshot

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/task"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetEtcd represents the error in case of fetching etcd object. Reason could be internal error
	ErrGetEtcd v1alpha1.ErrorCode = "ERR_GET_ETCD"
	//  ErrEtcdNotReady represents the error in case etcd is not ready
	ErrEtcdNotReady v1alpha1.ErrorCode = "ERR_ETCD_NOT_READY"
	//  ErrBackupNotEnabled represents the error in case backup is not enabled
	ErrBackupNotEnabled v1alpha1.ErrorCode = "ERR_BACKUP_NOT_ENABLED"
	// ErrCreateHTTPRequest represents the error in case of failure in creating http request
	ErrCreateHTTPRequest v1alpha1.ErrorCode = "ERR_CREATE_HTTP_REQUEST"
	// ErrExecuteHTTPRequest represents the error in case of failure in executing http request
	ErrExecuteHTTPRequest v1alpha1.ErrorCode = "ERR_EXECUTE_HTTP_REQUEST"
	//  ErrCreateSnapshot represents the error in case of failure in creating snapshot
	ErrCreateSnapshot v1alpha1.ErrorCode = "ERR_CREATE_SNAPSHOT"
)

// OnDemandSnapshotTask implements the task.Handler interface for handling on-demand snapshot tasks.
type OnDemandSnapshotTask struct {
	client        client.Client
	logger        logr.Logger
	name          string
	etcdReference v1alpha1.EtcdReference
	httpClient    http.Client
	config        v1alpha1.OnDemandSnapshotConfig
}

// New creates a new instance of OnDemandSnapshotTask.
func New(k8sclient client.Client, logger logr.Logger, task *v1alpha1.EtcdOpsTask) (task.Handler, error) {

	return &OnDemandSnapshotTask{
		client:        k8sclient,
		logger:        logger,
		name:          task.Name,
		etcdReference: *task.Spec.EtcdRef,
		httpClient:    http.Client{Timeout: time.Second * time.Duration(*task.Spec.Config.OnDemandSnapshot.TimeoutSeconds)},
		config:        *task.Spec.Config.OnDemandSnapshot,
	}, nil
}

// EtcdReference returns the NamespacedName of the etcd object referenced by the task.
func (o *OnDemandSnapshotTask) EtcdReference() types.NamespacedName {
	return types.NamespacedName{
		Name:      o.etcdReference.Name,
		Namespace: o.etcdReference.Namespace,
	}
}

// Name returns the name of the task.
func (o *OnDemandSnapshotTask) Name() string {
	return o.name
}

// Logger returns the logger for the task.
func (o *OnDemandSnapshotTask) Logger() logr.Logger {
	return o.logger
}

// Admit checks if the task can be admitted for execution.
func (o *OnDemandSnapshotTask) Admit(ctx context.Context) *task.Result {
	var etcd v1alpha1.Etcd
	if err := o.client.Get(ctx, o.EtcdReference(), &etcd); err != nil {
		// Check the type of error and return result. If it's a transport layer issue, requeue. If the object is not found, return reject.
		if apierrors.IsNotFound(err) {
			return &task.Result{
				Description: "Admit Operation: Failed to get etcd object",
				Error:       druiderr.WrapError(err, ErrGetEtcd, task.AdmitOperation, "failed to get etcd object"),
				Completed:   true,
			}
		}
		return &task.Result{
			Description: "Admit Operation: Failed to get etcd object due to internal error",
			Error:       druiderr.WrapError(err, ErrGetEtcd, task.AdmitOperation, "failed to get etcd object due to internal error"),
			Completed:   false,
		}
	}

	isBackupEnabled := etcd.IsBackupStoreEnabled()
	if !isBackupEnabled {
		return &task.Result{
			Description: "Admit Operation: Backup is not enabled for etcd",
			Error:       druiderr.WrapError(fmt.Errorf("backup is not enabled for etcd"), ErrBackupNotEnabled, task.AdmitOperation, "backup is not enabled for etcd"),
			Completed:   true,
		}
	}

	if err := CheckEtcdReadiness(ctx, &etcd); err != nil {
		return &task.Result{
			Description: "Admit Operation: Etcd is not ready",
			Error:       druiderr.WrapError(err, ErrEtcdNotReady, task.AdmitOperation, "etcd is not ready"),
			Completed:   true,
		}
	}
	return &task.Result{
		Description: "Admit check passed",
		Completed:   true,
	}
}

// Run executes the on-demand snapshot task.
func (o *OnDemandSnapshotTask) Run(ctx context.Context) *task.Result {
	etcd := &v1alpha1.Etcd{}
	if err := o.client.Get(ctx, o.EtcdReference(), etcd); err != nil {
		return &task.Result{
			Description: "Run Operation: Failed to get etcd object",
			Error:       druiderr.WrapError(err, ErrGetEtcd, task.RunOperation, "failed to get etcd object"),
			Completed:   false,
		}
	}
	if err := CheckEtcdReadiness(ctx, etcd); err != nil {
		return &task.Result{
			Description: "Run Operation: Etcd is not ready",
			Error:       druiderr.WrapError(err, ErrEtcdNotReady, task.RunOperation, "etcd is not ready"),
			Completed:   true,
		}
	}

	url := fmt.Sprintf("http://%s.%s:%d/snapshot/%s", v1alpha1.GetClientServiceName(etcd.ObjectMeta), etcd.Namespace, ptr.Deref(etcd.Spec.Backup.Port, common.DefaultPortEtcdBackupRestore), o.config.Type)
	if ptr.Deref(o.config.IsFinal, false) {
		url += "?final=true"
	}
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return &task.Result{
			Description: "Run Operation: Failed to create HTTP request",
			Error:       druiderr.WrapError(err, ErrCreateHTTPRequest, task.RunOperation, "failed to create HTTP request"),
			Completed:   false,
		}
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return &task.Result{
			Description: "Run Operation: Failed to execute HTTP request",
			Error:       druiderr.WrapError(err, ErrExecuteHTTPRequest, task.RunOperation, "failed to execute HTTP request"),
			Completed:   false,
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &task.Result{
			Description: "Run Operation: Failed to create snapshot",
			Error:       druiderr.WrapError(fmt.Errorf("failed to create snapshot, status code: %d", resp.StatusCode), ErrCreateSnapshot, task.RunOperation, "failed to create snapshot"),
			Completed:   true,
		}
	}
	return &task.Result{
		Description: "Snapshot created successfully",
		Completed:   true,
	}

}

// Cleanup performs any necessary cleanup after the task is completed.
func (o *OnDemandSnapshotTask) Cleanup(_ context.Context) *task.Result {
	return &task.Result{
		Description: "Cleanup completed",
		Completed:   true,
	}
}

// CheckEtcdReadiness checks if the etcd object is ready.
func CheckEtcdReadiness(_ context.Context, etcd *v1alpha1.Etcd) error {
	for _, condition := range etcd.Status.Conditions {
		if condition.Type == v1alpha1.ConditionTypeReady {
			if condition.Status == v1alpha1.ConditionTrue {
				return nil
			}
			return fmt.Errorf("etcd is not ready, condition: %s", condition.Message)
		}
	}
	return fmt.Errorf("etcd is not ready")
}
