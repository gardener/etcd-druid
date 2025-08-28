package utils

import (
	"context"

	"github.com/gardener/etcd-druid/internal/task"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

// Ensure the fake fulfils the interface.
var _ task.Handler = (*FakeHandler)(nil)

// FakeHandler returns the same result every time its methods are called.
// *Not* safe for concurrent mutation, but fine for typical single‑threaded
// unit‑test flows.
//
// Each field has a sensible default (Completed=true) so tests can often create
// one with `NewFakeHandler(...)` and be done.
//
// If you need different behaviour set the fields directly or use the fluent
// helpers (WithAdmit, WithRun, WithCleanup).

type FakeHandler struct {
	etcdRef types.NamespacedName
	name    string
	logger  logr.Logger

	AdmitResult   *task.Result
	RunResult     *task.Result
	CleanupResult *task.Result
}

// NewFakeHandler returns a fake whose Admit, Run, and Cleanup all immediately
// signal completion.  You can override any of the results via the fluent
// helper methods below.
func NewFakeHandler(name string, etcdRef types.NamespacedName, logger logr.Logger) *FakeHandler {
	completed := &task.Result{Completed: true}
	return &FakeHandler{
		etcdRef:       etcdRef,
		name:          name,
		logger:        logger,
		AdmitResult:   completed,
		RunResult:     completed,
		CleanupResult: completed,
	}
}

// ----- helpers -------------------------------------------------

func (f *FakeHandler) WithAdmit(r *task.Result) *FakeHandler {
	f.AdmitResult = r
	return f
}

func (f *FakeHandler) WithRun(r *task.Result) *FakeHandler {
	if r != nil {
		f.RunResult = r
	}
	return f
}

func (f *FakeHandler) WithCleanup(r *task.Result) *FakeHandler {
	if r != nil {
		f.CleanupResult = r
	}
	return f
}

// ----- task.Handler implementation ------------------------------------------

func (f *FakeHandler) EtcdReference() types.NamespacedName { return f.etcdRef }

func (f *FakeHandler) Name() string { return f.name }

// TODO:
// Add a check/ logic to ensure that if the result is set as false, then the oepration such as Admit, Run, Cleanup should not be called ie mocking the failure case.
// IN each test case, set it so that either failure case or success case is tested.

// Logger returns a no‑op logger suitable for tests.
func (f *FakeHandler) Logger() logr.Logger { return f.logger }

func (f *FakeHandler) Admit(ctx context.Context) *task.Result { return f.AdmitResult }

func (f *FakeHandler) Run(ctx context.Context) *task.Result { return f.RunResult }

func (f *FakeHandler) Cleanup(ctx context.Context) *task.Result { return f.CleanupResult }
