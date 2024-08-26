// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
package utils

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/internal/component"

	"github.com/go-logr/logr"

	. "github.com/onsi/gomega"
)

func TestRunConcurrentlyWithAllSuccessfulTasks(t *testing.T) {
	tasks := []OperatorTask{
		createSuccessfulTaskWithDelay("task-1", 5*time.Millisecond),
		createSuccessfulTaskWithDelay("task-2", 15*time.Millisecond),
		createSuccessfulTaskWithDelay("task-3", 10*time.Millisecond),
	}
	g := NewWithT(t)
	opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "1e7632c1-fd2d-4a35-a690-c53fa68e1442")
	g.Expect(RunConcurrently(opCtx, tasks)).To(HaveLen(0))
}

func TestRunConcurrentlyWithOnePanickyTask(t *testing.T) {
	tasks := []OperatorTask{
		createSuccessfulTaskWithDelay("task-1", 5*time.Millisecond),
		createPanickyTaskWithDelay("panicky-task-2", 15*time.Millisecond),
		createSuccessfulTaskWithDelay("task-3", 10*time.Millisecond),
	}
	g := NewWithT(t)
	opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "1e7632c1-fd2d-4a35-a690-c53fa68e1442")
	g.Expect(RunConcurrently(opCtx, tasks)).To(HaveLen(1))
}

func TestRunConcurrentlyWithPanickyAndErringTasks(t *testing.T) {
	tasks := []OperatorTask{
		createSuccessfulTaskWithDelay("task-1", 5*time.Millisecond),
		createPanickyTaskWithDelay("panicky-task-2", 15*time.Millisecond),
		createSuccessfulTaskWithDelay("task-3", 10*time.Millisecond),
		createErringTaskWithDelay("erring-task-4", 50*time.Millisecond),
	}
	g := NewWithT(t)
	opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "1e7632c1-fd2d-4a35-a690-c53fa68e1442")
	g.Expect(RunConcurrently(opCtx, tasks)).To(HaveLen(2))
}

func createSuccessfulTaskWithDelay(name string, delay time.Duration) OperatorTask {
	return OperatorTask{
		Name: name,
		Fn: func(ctx component.OperatorContext) error {
			tick := time.NewTicker(delay)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-tick.C:
					return nil
				}
			}
		},
	}
}

func createPanickyTaskWithDelay(name string, delay time.Duration) OperatorTask {
	return OperatorTask{
		Name: name,
		Fn: func(ctx component.OperatorContext) error {
			tick := time.NewTicker(delay)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-tick.C:
					panic("i panicked")
				}
			}
		},
	}
}

func createErringTaskWithDelay(name string, delay time.Duration) OperatorTask {
	return OperatorTask{
		Name: name,
		Fn: func(ctx component.OperatorContext) error {
			tick := time.NewTicker(delay)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-tick.C:
					return errors.New("this task will never succeed")
				}
			}
		},
	}
}
