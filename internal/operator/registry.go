package operator

import (
	"github.com/gardener/etcd-druid/internal/operator/clientservice"
	"github.com/gardener/etcd-druid/internal/operator/configmap"
	"github.com/gardener/etcd-druid/internal/operator/memberlease"
	"github.com/gardener/etcd-druid/internal/operator/peerservice"
	"github.com/gardener/etcd-druid/internal/operator/poddistruptionbudget"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/operator/role"
	"github.com/gardener/etcd-druid/internal/operator/rolebinding"
	"github.com/gardener/etcd-druid/internal/operator/serviceaccount"
	"github.com/gardener/etcd-druid/internal/operator/snapshotlease"
	"github.com/gardener/etcd-druid/internal/operator/statefulset"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Registry is a facade which gives access to all resource operators.
type Registry interface {
	// AllOperators gives a map, where the key is the Kind of resource that an operator manages and the value is an Operator itself.
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
	// StatefulSetKind indicates that the kind of resource is a StatefulSet.
	StatefulSetKind Kind = "StatefulSet"
	// ServiceAccountKind indicates that the kind of resource is a ServiceAccount.
	ServiceAccountKind Kind = "ServiceAccount"
	// RoleKind indicates that the kind of resource is a Role.
	RoleKind Kind = "Role"
	// RoleBindingKind indicates that the kind of resource is RoleBinding
	RoleBindingKind Kind = "RoleBinding"
	// MemberLeaseKind indicates that the kind of resource is a Lease used for an etcd member heartbeat.
	MemberLeaseKind Kind = "MemberLease"
	// SnapshotLeaseKind indicates that the kind of resource is a Lease used to capture snapshot information.
	SnapshotLeaseKind Kind = "SnapshotLease"
	// ConfigMapKind indicates that the kind of resource is a ConfigMap.
	ConfigMapKind Kind = "ConfigMap"
	// PeerServiceKind indicates that the kind of resource is a Service used for etcd peer communication.
	PeerServiceKind Kind = "PeerService"
	// ClientServiceKind indicates that the kind of resource is a Service used for etcd client communication.
	ClientServiceKind Kind = "ClientService"
	// PodDisruptionBudgetKind indicates that the kind of resource is a PodDisruptionBudget.
	PodDisruptionBudgetKind Kind = "PodDisruptionBudget"
)

type registry struct {
	operators map[Kind]resource.Operator
}

// NewRegistry creates a new instance of a Registry.
func NewRegistry(client client.Client, logger logr.Logger, config resource.Config) Registry {
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
	operators[StatefulSetKind] = statefulset.New(client, logger, config)
	return registry{
		operators: operators,
	}
}

func (r registry) AllOperators() map[Kind]resource.Operator {
	return r.operators
}

func (r registry) StatefulSetOperator() resource.Operator {
	return r.operators[StatefulSetKind]
}

func (r registry) ServiceAccountOperator() resource.Operator {
	return r.operators[ServiceAccountKind]
}

func (r registry) RoleOperator() resource.Operator {
	return r.operators[RoleKind]
}

func (r registry) RoleBindingOperator() resource.Operator {
	return r.operators[RoleBindingKind]
}

func (r registry) MemberLeaseOperator() resource.Operator {
	return r.operators[MemberLeaseKind]
}

func (r registry) SnapshotLeaseOperator() resource.Operator {
	return r.operators[SnapshotLeaseKind]
}

func (r registry) ConfigMapOperator() resource.Operator {
	return r.operators[ConfigMapKind]
}

func (r registry) PeerServiceOperator() resource.Operator {
	return r.operators[PeerServiceKind]
}

func (r registry) ClientServiceOperator() resource.Operator {
	return r.operators[ClientServiceKind]
}

func (r registry) PodDisruptionBudgetOperator() resource.Operator {
	return r.operators[PodDisruptionBudgetKind]
}
