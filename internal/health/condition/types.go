// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
)

// Checker is an interface to check the etcd resource and to return condition results.
type Checker interface {
	Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result
}

// Result encapsulates a condition result
type Result interface {
	ConditionType() druidv1alpha1.ConditionType
	Status() druidv1alpha1.ConditionStatus
	Reason() string
	Message() string
}

type result struct {
	conType druidv1alpha1.ConditionType
	status  druidv1alpha1.ConditionStatus
	reason  string
	message string
}

func (r *result) ConditionType() druidv1alpha1.ConditionType {
	return r.conType
}

func (r *result) Status() druidv1alpha1.ConditionStatus {
	return r.status
}

func (r *result) Reason() string {
	return r.reason
}

func (r *result) Message() string {
	return r.message
}
