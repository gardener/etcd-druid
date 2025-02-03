// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"sync"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/health/condition"
	"github.com/gardener/etcd-druid/internal/health/etcdmember"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConditionCheckFn is a type alias for a function which returns an implementation of `Check`.
type ConditionCheckFn func(client.Client) condition.Checker

// EtcdMemberCheckFn is a type alias for a function which returns an implementation of `Check`.
type EtcdMemberCheckFn func(client.Client, logr.Logger, time.Duration, time.Duration) etcdmember.Checker

// TimeNow is the function used to get the current time.
var TimeNow = time.Now

var (
	// NewDefaultConditionBuilder is the default condition builder.
	NewDefaultConditionBuilder = condition.NewBuilder
	// NewDefaultEtcdMemberBuilder is the default etcd member builder.
	NewDefaultEtcdMemberBuilder = etcdmember.NewBuilder
	// ConditionChecks Checks are the registered condition checks.
	ConditionChecks = []ConditionCheckFn{
		condition.ReadyCheck,
		condition.AllMembersReadyCheck,
		condition.AllMembersUpdatedCheck,
		condition.BackupReadyCheck,
		condition.DataVolumesReadyCheck,
	}
	// EtcdMemberChecks are the etcd member checks.
	EtcdMemberChecks = []EtcdMemberCheckFn{
		etcdmember.ReadyCheck,
	}
)

// Checker checks Etcd status conditions and the status of the Etcd members.
type Checker struct {
	cl                          client.Client
	etcdMemberNotReadyThreshold time.Duration
	etcdMemberUnknownThreshold  time.Duration
	conditionCheckFns           []ConditionCheckFn
	conditionBuilderFn          func() condition.Builder
	etcdMemberCheckFns          []EtcdMemberCheckFn
	etcdMemberBuilderFn         func() etcdmember.Builder
}

// Check executes the status checks and mutates the passed status object with the corresponding results.
func (c *Checker) Check(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	// First execute the etcd member checks for the status.
	if err := c.executeEtcdMemberChecks(ctx, logger, etcd); err != nil {
		return err
	}

	// Execute condition checks after the etcd member checks because we need their result here.
	return c.executeConditionChecks(ctx, etcd)
}

// executeConditionChecks runs all registered condition checks **in parallel**.
func (c *Checker) executeConditionChecks(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	var (
		resultCh = make(chan condition.Result)

		wg sync.WaitGroup
	)

	// Run condition checks in parallel since each check work independently of each other.
	for _, newCheck := range c.conditionCheckFns {
		c := newCheck(c.cl)
		wg.Add(1)
		go (func() {
			defer wg.Done()
			resultCh <- c.Check(ctx, *etcd)
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
		WithOldConditions(etcd.Status.Conditions).
		WithResults(results).
		Build(etcd.Spec.Replicas)

	etcd.Status.Conditions = conditions
	return nil
}

// executeEtcdMemberChecks runs all registered etcd member checks **sequentially**.
// The result of a check is passed via the `status` sub-resources to the next check.
func (c *Checker) executeEtcdMemberChecks(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	// Run etcd member checks sequentially as most of them act on multiple elements.
	for _, newCheck := range c.etcdMemberCheckFns {
		results := newCheck(c.cl, logger, c.etcdMemberNotReadyThreshold, c.etcdMemberUnknownThreshold).Check(ctx, *etcd)

		// Build and assign the results after each check, so that the next check
		// can act on the latest results.
		memberStatuses := c.etcdMemberBuilderFn().
			WithNowFunc(func() metav1.Time { return metav1.NewTime(TimeNow()) }).
			WithOldMembers(etcd.Status.Members).
			WithResults(results).
			Build()

		etcd.Status.Members = memberStatuses
	}
	return nil
}

// NewChecker creates a new instance for checking the etcd status.
func NewChecker(cl client.Client, etcdMemberNotReadyThreshold, etcdMemberUnknownThreshold time.Duration) *Checker {
	return &Checker{
		cl:                          cl,
		etcdMemberNotReadyThreshold: etcdMemberNotReadyThreshold,
		etcdMemberUnknownThreshold:  etcdMemberUnknownThreshold,
		conditionCheckFns:           ConditionChecks,
		conditionBuilderFn:          NewDefaultConditionBuilder,
		etcdMemberCheckFns:          EtcdMemberChecks,
		etcdMemberBuilderFn:         NewDefaultEtcdMemberBuilder,
	}
}
