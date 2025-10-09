// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

// Ensure the fake fulfils the interface.
var _ handler.Handler = (*FakeEtcdOpsTaskHandler)(nil)

// FakeEtcdOpsTaskHandler returns the same result every time its methods are called.
// *Not* safe for concurrent mutation, but fine for typical single‑threaded
// unit‑test flows.
//
// Each field has a sensible default (Completed=true) so tests can often create
// one with `NewFakeEtcdOpsTaskHandler(...)` and be done.
//
// If you need different behaviour set the fields directly or use the fluent
// helpers (WithAdmit, WithRun, WithCleanup).

type FakeEtcdOpsTaskHandler struct {
	etcdRef       types.NamespacedName
	AdmitResult   handler.Result
	RunResult     handler.Result
	CleanupResult handler.Result
}

// NewFakeEtcdOpsTaskHandler returns a fake whose Admit, Run, and Cleanup all immediately
// signal completion.  You can override any of the results via the fluent
// helper methods below.
func NewFakeEtcdOpsTaskHandler(name string, etcdRef types.NamespacedName, logger logr.Logger) *FakeEtcdOpsTaskHandler {
	completed := handler.Result{Completed: true}
	return &FakeEtcdOpsTaskHandler{
		etcdRef:       etcdRef,
		AdmitResult:   completed,
		RunResult:     completed,
		CleanupResult: completed,
	}
}

// ----- helpers -------------------------------------------------

func (f *FakeEtcdOpsTaskHandler) WithAdmit(r handler.Result) *FakeEtcdOpsTaskHandler {
	f.AdmitResult = r
	return f
}

func (f *FakeEtcdOpsTaskHandler) WithRun(r handler.Result) *FakeEtcdOpsTaskHandler {
	f.RunResult = r
	return f
}

func (f *FakeEtcdOpsTaskHandler) WithCleanup(r handler.Result) *FakeEtcdOpsTaskHandler {
	f.CleanupResult = r
	return f
}

// ----- handler.Handler implementation ------------------------------------------
func (f *FakeEtcdOpsTaskHandler) Admit(ctx context.Context) handler.Result { return f.AdmitResult }

func (f *FakeEtcdOpsTaskHandler) Run(ctx context.Context) handler.Result { return f.RunResult }

func (f *FakeEtcdOpsTaskHandler) Cleanup(ctx context.Context) handler.Result { return f.CleanupResult }
