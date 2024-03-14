// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/component"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LastOperationErrorRecorder records etcd.Status.LastOperation and etcd.Status.LastErrors
type LastOperationErrorRecorder interface {
	RecordStart(ctx component.OperatorContext, etcdObjectKey client.ObjectKey, operationType druidv1alpha1.LastOperationType) error
	RecordSuccess(ctx component.OperatorContext, etcdObjectKey client.ObjectKey, operationType druidv1alpha1.LastOperationType) error
	RecordError(ctx component.OperatorContext, etcdObjectKey client.ObjectKey, operationType druidv1alpha1.LastOperationType, description string, errs ...error) error
}

func NewLastOperationErrorRecorder(client client.Client, logger logr.Logger) LastOperationErrorRecorder {
	return &lastOpErrRecorder{
		client: client,
		logger: logger,
	}
}

type lastOpErrRecorder struct {
	client client.Client
	logger logr.Logger
}

func (l *lastOpErrRecorder) RecordStart(ctx component.OperatorContext, etcdObjectKey client.ObjectKey, operationType druidv1alpha1.LastOperationType) error {
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
	return l.recordLastOperationAndErrors(ctx, etcdObjectKey, operationType, druidv1alpha1.LastOperationStateProcessing, description)
}

func (l *lastOpErrRecorder) RecordSuccess(ctx component.OperatorContext, etcdObjectKey client.ObjectKey, operationType druidv1alpha1.LastOperationType) error {
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

	return l.recordLastOperationAndErrors(ctx, etcdObjectKey, operationType, druidv1alpha1.LastOperationStateSucceeded, description)
}

func (l *lastOpErrRecorder) RecordError(ctx component.OperatorContext, etcdObjectKey client.ObjectKey, operationType druidv1alpha1.LastOperationType, description string, errs ...error) error {
	description += " Operation will be retried."
	lastErrors := druiderr.MapToLastErrors(errs)
	return l.recordLastOperationAndErrors(ctx, etcdObjectKey, operationType, druidv1alpha1.LastOperationStateError, description, lastErrors...)
}

func (l *lastOpErrRecorder) recordLastOperationAndErrors(ctx component.OperatorContext, etcdObjectKey client.ObjectKey, operationType druidv1alpha1.LastOperationType, operationState druidv1alpha1.LastOperationState, description string, lastErrors ...druidv1alpha1.LastError) error {
	etcd := &druidv1alpha1.Etcd{}
	if err := l.client.Get(ctx, etcdObjectKey, etcd); err != nil {
		return err
	}
	originalEtcd := etcd.DeepCopy()
	etcd.Status.LastOperation = &druidv1alpha1.LastOperation{
		RunID:          ctx.RunID,
		Type:           operationType,
		State:          operationState,
		LastUpdateTime: metav1.NewTime(time.Now().UTC()),
		Description:    description,
	}
	etcd.Status.LastErrors = lastErrors
	return l.client.Status().Patch(ctx, etcd, client.MergeFrom(originalEtcd))
}
