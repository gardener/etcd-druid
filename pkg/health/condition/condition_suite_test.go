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

package condition_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

func TestCondition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Condition Suite")
}

type result struct {
	ConType    druidv1alpha1.ConditionType
	ConStatus  druidv1alpha1.ConditionStatus
	ConReason  string
	ConMessage string
}

func (r *result) ConditionType() druidv1alpha1.ConditionType {
	return r.ConType
}

func (r *result) Status() druidv1alpha1.ConditionStatus {
	return r.ConStatus
}

func (r *result) Reason() string {
	return r.ConReason
}

func (r *result) Message() string {
	return r.ConMessage
}
