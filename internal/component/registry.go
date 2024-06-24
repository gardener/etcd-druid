// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package component

// Registry is a facade which gives access to all component operators.
type Registry interface {
	// Register provides consumers to register a component operator against the kind of component it operates on.
	Register(kind Kind, operator Operator)
	// AllOperators gives a map, where the key is the Kind of component that an operator manages and the value is an Operator itself.
	AllOperators() map[Kind]Operator
	// GetOperator gets a component operator that operates on the kind.
	// Returns the component operator if found, else nil will be returned.
	GetOperator(kind Kind) Operator
}

// Kind represents the kind of component that an operator manages.
type Kind string

const (
	// StatefulSetKind indicates that the kind of component is a StatefulSet.
	StatefulSetKind Kind = "StatefulSet"
	// ServiceAccountKind indicates that the kind of component is a ServiceAccount.
	ServiceAccountKind Kind = "ServiceAccount"
	// RoleKind indicates that the kind of component is a Role.
	RoleKind Kind = "Role"
	// RoleBindingKind indicates that the kind of component is RoleBinding
	RoleBindingKind Kind = "RoleBinding"
	// MemberLeaseKind indicates that the kind of component is a Lease used for an etcd member heartbeat.
	MemberLeaseKind Kind = "MemberLease"
	// SnapshotLeaseKind indicates that the kind of component is a Lease used to capture snapshot information.
	SnapshotLeaseKind Kind = "SnapshotLease"
	// ConfigMapKind indicates that the kind of component is a ConfigMap.
	ConfigMapKind Kind = "ConfigMap"
	// PeerServiceKind indicates that the kind of component is a Service used for etcd peer communication.
	PeerServiceKind Kind = "PeerService"
	// ClientServiceKind indicates that the kind of component is a Service used for etcd client communication.
	ClientServiceKind Kind = "ClientService"
	// PodDisruptionBudgetKind indicates that the kind of component is a PodDisruptionBudget.
	PodDisruptionBudgetKind Kind = "PodDisruptionBudget"
)

type registry struct {
	operators map[Kind]Operator
}

// NewRegistry creates a new instance of a Registry.
func NewRegistry() Registry {
	operators := make(map[Kind]Operator)
	return registry{
		operators: operators,
	}
}

func (r registry) Register(kind Kind, operator Operator) {
	r.operators[kind] = operator
}

func (r registry) GetOperator(kind Kind) Operator {
	return r.operators[kind]
}

func (r registry) AllOperators() map[Kind]Operator {
	return r.operators
}
