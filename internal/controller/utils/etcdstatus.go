package utils

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/registry/resource"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LastOperationErrorRecorder records etcd.Status.LastOperation and etcd.Status.LastErrors
type LastOperationErrorRecorder interface {
	RecordStart(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidv1alpha1.LastOperationType) error
	RecordSuccess(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidv1alpha1.LastOperationType) error
	RecordError(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidv1alpha1.LastOperationType, description string, errs ...error) error
}

func NewLastOperationErrorRecorder(client client.Client, logger logr.Logger) LastOperationErrorRecorder {
	return &lastOpErrRecorder{
		client: client,
		logger: logger,
	}
}

type lastOpErrRecorder struct {
	client    client.Client
	objectKey client.ObjectKey
	logger    logr.Logger
}

func (l *lastOpErrRecorder) RecordStart(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidv1alpha1.LastOperationType) error {
	const (
		etcdReconcileStarted string = "Etcd cluster reconciliation is in progress"
		etcdDeletionStarted  string = "Etcd cluster deletion is in progress"
	)
	var description string
	switch operationType {
	case druidv1alpha1.LastOperationTypeCreate, druidv1alpha1.LastOperationTypeReconcile:
		description = etcdReconcileStarted
	case druidv1alpha1.LastOperationTypeDelete:
		description = etcdDeletionStarted
	}
	return l.recordLastOperationAndErrors(ctx, etcd, operationType, druidv1alpha1.LastOperationStateProcessing, description)
}

func (l *lastOpErrRecorder) RecordSuccess(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidv1alpha1.LastOperationType) error {
	const (
		etcdReconciledSuccessfully string = "Etcd cluster has been successfully reconciled"
		etcdDeletedSuccessfully    string = "Etcd cluster has been successfully deleted"
	)
	var description string
	switch operationType {
	case druidv1alpha1.LastOperationTypeCreate, druidv1alpha1.LastOperationTypeReconcile:
		description = etcdReconciledSuccessfully
	case druidv1alpha1.LastOperationTypeDelete:
		description = etcdDeletedSuccessfully
	}

	return l.recordLastOperationAndErrors(ctx, etcd, operationType, druidv1alpha1.LastOperationStateSucceeded, description)
}

func (l *lastOpErrRecorder) RecordError(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidv1alpha1.LastOperationType, description string, errs ...error) error {
	description += " Operation will be retried."
	lastErrors := druiderr.MapToLastErrors(errs)
	return l.recordLastOperationAndErrors(ctx, etcd, operationType, druidv1alpha1.LastOperationStateError, description, lastErrors...)
}

func (l *lastOpErrRecorder) recordLastOperationAndErrors(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidv1alpha1.LastOperationType, operationState druidv1alpha1.LastOperationState, description string, lastErrors ...druidv1alpha1.LastError) error {
	etcdPatch := client.StrategicMergeFrom(etcd.DeepCopy())

	// update last operation
	if etcd.Status.LastOperation == nil {
		etcd.Status.LastOperation = &druidv1alpha1.LastOperation{}
	}
	etcd.Status.LastOperation.RunID = ctx.RunID
	etcd.Status.LastOperation.Type = operationType
	etcd.Status.LastOperation.State = operationState
	etcd.Status.LastOperation.LastUpdateTime = metav1.NewTime(time.Now().UTC())
	etcd.Status.LastOperation.Description = description
	// update last errors
	etcd.Status.LastErrors = lastErrors

	err := l.client.Status().Patch(ctx, etcd, etcdPatch)
	if err != nil {
		l.logger.Error(err, "failed to update LastOperation and LastErrors")
	}
	return err
}
