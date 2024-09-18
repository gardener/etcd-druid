// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// constants for operations that an Operator can perform. This is used across components to provide context when reporting error.
// However, it can also be used where ever operation context is required to be specified.
const (
	// OperationPreSync is the PreSync operation of the Operator.
	OperationPreSync = "PreSync"
	// OperationSync is the Sync operation of the Operator.
	OperationSync = "Sync"
	// OperationTriggerDelete is the TriggerDelete operation of the Operator.
	OperationTriggerDelete = "TriggerDelete"
	// OperationGetExistingResourceNames is the GetExistingResourceNames operation of the Operator.
	OperationGetExistingResourceNames = "GetExistingResourceNames"
)

// OperatorContext holds the underline context.Context along with additional data that needs to be passed from one reconcile-step to another in a multistep reconciliation run.
type OperatorContext struct {
	context.Context
	// RunID is unique ID identifying a single reconciliation run.
	RunID string
	// Logger is the logger that can be used by a reconcile flow or sub-flow.
	Logger logr.Logger
	// Data is place-holder for steps to record data that can be accessed by steps ahead in the reconcile flow.
	Data map[string]string
}

// NewOperatorContext creates a new instance of OperatorContext.
func NewOperatorContext(ctx context.Context, logger logr.Logger, runID string) OperatorContext {
	return OperatorContext{
		Context: ctx,
		RunID:   runID,
		Logger:  logger,
		Data:    make(map[string]string),
	}
}

// SetLogger sets the logger for the OperatorContext.
func (o *OperatorContext) SetLogger(logger logr.Logger) {
	o.Logger = logger
}

// Operator manages one or more resources of a specific Kind which are provisioned for an etcd cluster.
type Operator interface {
	// GetExistingResourceNames gets all resources that currently exist that this Operator manages.
	GetExistingResourceNames(ctx OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error)
	// TriggerDelete triggers the deletion of all resources that this Operator manages.
	TriggerDelete(ctx OperatorContext, etcdObjMeta metav1.ObjectMeta) error
	// Sync synchronizes all resources that this Operator manages. If a component does not exist then it will
	// create it. If there are changes in the owning Etcd resource that transpires changes to one or more resources
	// managed by this Operator then those component(s) will be either be updated or a deletion is triggered.
	Sync(ctx OperatorContext, etcd *druidv1alpha1.Etcd) error
	// PreSync performs any preparatory operations that are required before the actual sync operation is performed,
	// to bring the component to a sync-ready state. If the sync-ready state is already satisfied,
	// then this method will be a no-op.
	PreSync(ctx OperatorContext, etcd *druidv1alpha1.Etcd) error
}
