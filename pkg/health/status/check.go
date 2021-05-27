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

package status

import (
	"context"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/etcd-druid/controllers/config"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/health/condition"
	"github.com/gardener/etcd-druid/pkg/health/etcdmember"
)

// ConditionCheckFn is a type alias for a function which returns an implementation of `Check`.
type ConditionCheckFn func() condition.Checker

// EtcdMemberCheckFn is a type alias for a function which returns an implementation of `Check`.
type EtcdMemberCheckFn func(config.EtcdCustodianController) etcdmember.Checker

// TimeNow is the function used to get the current time.
var TimeNow = time.Now

var (
	// NewDefaultConditionBuilder is the default condition builder.
	NewDefaultConditionBuilder = condition.NewBuilder
	// NewDefaultEtcdMemberBuilder is the default etcd member builder.
	NewDefaultEtcdMemberBuilder = etcdmember.NewBuilder
	// Checks are the registered condition checks.
	ConditionChecks = []ConditionCheckFn{
		condition.AllMembersCheck,
		condition.ReadyCheck,
	}
	// EtcdMemberChecks are the etcd member checks.
	EtcdMemberChecks = []EtcdMemberCheckFn{
		etcdmember.ReadyCheck,
	}
)

type checker struct {
	config              config.EtcdCustodianController
	conditionCheckFns   []ConditionCheckFn
	conditionBuilderFn  func() condition.Builder
	etcdMemberCheckFns  []EtcdMemberCheckFn
	etcdMemberBuilderFn func() etcdmember.Builder
}

// Check executes the status checks and mutates the passed status object with the corresponding results.
func (c *checker) Check(ctx context.Context, status *druidv1alpha1.EtcdStatus) error {
	// First execute the etcd member checks for the status.
	if err := c.executeEtcdMemberChecks(status); err != nil {
		return err
	}

	// Execute condition checks after the etcd member checks because we need their result here.
	if err := c.executeConditionChecks(status); err != nil {
		return err
	}
	return nil
}

// executeConditionChecks runs all registered condition checks **in parallel**.
func (c *checker) executeConditionChecks(status *druidv1alpha1.EtcdStatus) error {
	var (
		resultCh = make(chan condition.Result)

		wg sync.WaitGroup
	)

	// Run condition checks in parallel since they work independently from each other.
	for _, newCheck := range c.conditionCheckFns {
		c := newCheck()
		wg.Add(1)
		go (func() {
			defer wg.Done()
			resultCh <- c.Check(*status)
		})()
	}

	go (func() {
		defer close(resultCh)
		wg.Wait()
	})()

	results := make([]condition.Result, 0, len(ConditionChecks))
	for r := range resultCh {
		results = append(results, r)
	}

	conditions := c.conditionBuilderFn().
		WithNowFunc(func() metav1.Time { return metav1.NewTime(TimeNow()) }).
		WithOldConditions(status.Conditions).
		WithResults(results).
		Build()

	status.Conditions = conditions
	return nil
}

// executeEtcdMemberChecks runs all registered etcd member checks **sequentially**.
// The result of a check is passed via the `status` sub-resources to the next check.
func (c *checker) executeEtcdMemberChecks(status *druidv1alpha1.EtcdStatus) error {
	// Run etcd member checks sequentially as most of them act on multiple elements.
	for _, newCheck := range c.etcdMemberCheckFns {
		results := newCheck(c.config).Check(*status)

		// Build and assign the results after each check, so that the next check
		// can act on the latest results.
		memberStatuses := c.etcdMemberBuilderFn().
			WithNowFunc(func() metav1.Time { return metav1.NewTime(TimeNow()) }).
			WithOldMembers(status.Members).
			WithResults(results).
			Build()

		status.Members = memberStatuses
	}
	return nil
}

// NewChecker creates a new instance for checking the etcd status.
func NewChecker(config config.EtcdCustodianController) *checker {
	return &checker{
		config:              config,
		conditionCheckFns:   ConditionChecks,
		conditionBuilderFn:  NewDefaultConditionBuilder,
		etcdMemberCheckFns:  EtcdMemberChecks,
		etcdMemberBuilderFn: NewDefaultEtcdMemberBuilder,
	}
}
