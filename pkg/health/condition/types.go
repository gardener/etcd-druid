// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package condition

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

// Checker is an interface to check the etcd resource and to return condition results.
type Checker interface {
	Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result
}

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
