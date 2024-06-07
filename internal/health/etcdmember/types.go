// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdmember

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

// Checker is an interface to check the members of an etcd cluster.
type Checker interface {
	Check(ctx context.Context, etcd druidv1alpha1.Etcd) []Result
}

// Result is an interface to capture the result of checks on etcd members.
type Result interface {
	ID() *string
	Name() string
	Role() *druidv1alpha1.EtcdRole
	Status() druidv1alpha1.EtcdMemberConditionStatus
	Reason() string
}

type result struct {
	id     *string
	name   string
	role   *druidv1alpha1.EtcdRole
	status druidv1alpha1.EtcdMemberConditionStatus
	reason string
}

func (r *result) ID() *string {
	return r.id
}

func (r *result) Name() string {
	return r.name
}

func (r *result) Role() *druidv1alpha1.EtcdRole {
	return r.role
}

func (r *result) Status() druidv1alpha1.EtcdMemberConditionStatus {
	return r.status
}

func (r *result) Reason() string {
	return r.reason
}
