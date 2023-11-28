package resource

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
)

const (
	// ConfigMapCheckSumKey is the key which stores the latest checksum value of the ConfigMap changes as part of an etcd spec reconciliation.
	ConfigMapCheckSumKey = "checksum/etcd-configmap"
)

type Config struct {
	DisableEtcdServiceAccountAutomount bool
	ImageVector                        imagevector.ImageVector
	UseEtcdWrapper                     bool
}

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

// Operator manages one or more resources of a specific Kind which are provisioned for an etcd cluster.
type Operator interface {
	// GetExistingResourceNames gets all resources that currently exist that this Operator manages.
	GetExistingResourceNames(ctx OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error)
	// TriggerDelete triggers the deletion of all resources that this Operator manages.
	TriggerDelete(ctx OperatorContext, etcd *druidv1alpha1.Etcd) error
	// Sync synchronizes all resources that this Operator manages. If a resource does not exist then it will
	// create it. If there are changes in the owning Etcd resource that transpires changes to one or more resources
	// managed by this Operator then those resource(s) will be either be updated or a deletion is triggered.
	Sync(ctx OperatorContext, etcd *druidv1alpha1.Etcd) error
}
