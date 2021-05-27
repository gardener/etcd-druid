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

package etcdmember

import druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

// Checker is an interface to check the members of a etcd cluster.
type Checker interface {
	Check(status druidv1alpha1.EtcdStatus) []Result
}

type Result interface {
	ID() string
	Status() druidv1alpha1.EtcdMemberConditionStatus
	Reason() string
}

type result struct {
	id     string
	status druidv1alpha1.EtcdMemberConditionStatus
	reason string
}

func (r *result) ID() string {
	return r.id
}

func (r *result) Status() druidv1alpha1.EtcdMemberConditionStatus {
	return r.status
}

func (r *result) Reason() string {
	return r.reason
}
