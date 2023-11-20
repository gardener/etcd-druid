package registry

import (
	"github.com/gardener/etcd-druid/internal/registry/resource"
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

type OperatorConfig struct {
	DisableEtcdServiceAccountAutomount bool
}

type OperatorRegistry interface {
	AllOperators() map[Kind]resource.Operator
	StatefulSetOperator() resource.Operator
	ServiceAccountOperator() resource.Operator
	RoleOperator() resource.Operator
	RoleBindingOperator() resource.Operator
	MemberLeaseOperator() resource.Operator
	SnapshotLeaseOperator() resource.Operator
	ConfigMapOperator() resource.Operator
	PeerServiceOperator() resource.Operator
	ClientServiceOperator() resource.Operator
	PodDisruptionBudgetOperator() resource.Operator
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
	operators map[Kind]resource.Operator
}

func NewOperatorRegistry(client client.Client, logger logr.Logger, config OperatorConfig) OperatorRegistry {
	operators := make(map[Kind]resource.Operator)
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

func (r registry) AllOperators() map[Kind]resource.Operator {
	return r.operators
}
