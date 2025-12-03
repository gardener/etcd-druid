// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LastOperationAndLastErrorsRecorder records etcd.Status.LastOperation and etcd.Status.LastErrors.
type LastOperationAndLastErrorsRecorder interface {
	// RecordStart records the start of an operation in the Etcd status.
	RecordStart(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidapicommon.LastOperationType) error
	// RecordSuccess records the success of an operation in the Etcd status.
	RecordSuccess(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidapicommon.LastOperationType) error
	// RecordErrors records errors encountered in the last operation in the Etcd status.
	RecordErrors(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidapicommon.LastOperationType, operationResult ReconcileStepResult) error
}

// NewLastOperationAndLastErrorsRecorder returns a new LastOperationAndLastErrorsRecorder.
func NewLastOperationAndLastErrorsRecorder(client client.Client, logger logr.Logger) LastOperationAndLastErrorsRecorder {
	return &lastOpErrRecorder{
		client: client,
		logger: logger,
	}
}

type lastOpErrRecorder struct {
	client client.Client
	logger logr.Logger
}

func (l *lastOpErrRecorder) RecordStart(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidapicommon.LastOperationType) error {
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

func (l *lastOpErrRecorder) RecordSuccess(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidapicommon.LastOperationType) error {
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

func (l *lastOpErrRecorder) RecordErrors(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidapicommon.LastOperationType, operationResult ReconcileStepResult) error {
	description := fmt.Sprintf("%s Operation will be retried.", operationResult.description)
	lastErrors := druiderr.MapToLastErrors(operationResult.GetErrors())
	lastOpState := utils.IfConditionOr(operationResult.HasErrors(), druidv1alpha1.LastOperationStateError, druidv1alpha1.LastOperationStateRequeue)
	return l.recordLastOperationAndErrors(ctx, etcd, operationType, lastOpState, description, lastErrors...)
}

func (l *lastOpErrRecorder) recordLastOperationAndErrors(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, operationType druidapicommon.LastOperationType, operationState druidapicommon.LastOperationState, description string, lastErrors ...druidapicommon.LastError) error {
	originalEtcd := etcd.DeepCopy()
	etcd.Status.LastOperation = &druidapicommon.LastOperation{
		RunID:          ctx.RunID,
		Type:           operationType,
		State:          operationState,
		LastUpdateTime: metav1.NewTime(time.Now().UTC()),
		Description:    description,
	}
	etcd.Status.LastErrors = lastErrors
	return l.client.Status().Patch(ctx, etcd, client.MergeFrom(originalEtcd))
}
