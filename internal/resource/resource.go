package resource

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/resource/clientservice"
	"github.com/gardener/etcd-druid/internal/resource/configmap"
	"github.com/gardener/etcd-druid/internal/resource/memberlease"
	"github.com/gardener/etcd-druid/internal/resource/peerservice"
	"github.com/gardener/etcd-druid/internal/resource/poddistruptionbudget"
	"github.com/gardener/etcd-druid/internal/resource/role"
	"github.com/gardener/etcd-druid/internal/resource/rolebinding"
	"github.com/gardener/etcd-druid/internal/resource/serviceaccount"
	"github.com/gardener/etcd-druid/internal/resource/snapshotlease"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ConfigMapCheckSumKey = "checksum/etcd-configmap"
)

type OperatorContext struct {
	context.Context
	RunID  string
	Logger logr.Logger
	Data   map[string]string
}

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

type OperatorConfig struct {
	DisableEtcdServiceAccountAutomount bool
}

type OperatorRegistry interface {
	AllOperators() map[Kind]Operator
	StatefulSetOperator() Operator
	ServiceAccountOperator() Operator
	RoleOperator() Operator
	RoleBindingOperator() Operator
	MemberLeaseOperator() Operator
	SnapshotLeaseOperator() Operator
	ConfigMapOperator() Operator
	PeerServiceOperator() Operator
	ClientServiceOperator() Operator
	PodDisruptionBudgetOperator() Operator
}

type Kind string

const (
	StatefulSetKind         Kind = "StatefulSet"
	ServiceAccountKind      Kind = "ServiceAccount"
	RoleKind                Kind = "Role"
	RoleBindingKind         Kind = "RoleBinding"
	MemberLeaseKind         Kind = "MemberLease"
	SnapshotLeaseKind       Kind = "SnapshotLease"
	ConfigMapKind           Kind = "ConfigMap"
	PeerServiceKind         Kind = "PeerService"
	ClientServiceKind       Kind = "ClientService"
	PodDisruptionBudgetKind Kind = "PodDisruptionBudget"
)

type registry struct {
	operators map[Kind]Operator
}

func NewOperatorRegistry(client client.Client, logger logr.Logger, config OperatorConfig) OperatorRegistry {
	operators := make(map[Kind]Operator)
	operators[ConfigMapKind] = configmap.New(client, logger)
	operators[ServiceAccountKind] = serviceaccount.New(client, logger, config.DisableEtcdServiceAccountAutomount)
	operators[MemberLeaseKind] = memberlease.New(client, logger)
	operators[SnapshotLeaseKind] = snapshotlease.New(client, logger)
	operators[ClientServiceKind] = clientservice.New(client, logger)
	operators[PeerServiceKind] = peerservice.New(client, logger)
	operators[PodDisruptionBudgetKind] = poddistruptionbudget.New(client, logger)
	operators[RoleKind] = role.New(client, logger)
	operators[RoleBindingKind] = rolebinding.New(client, logger)
	return nil
}

func (r registry) AllOperators() map[Kind]Operator {
	return r.operators
}
