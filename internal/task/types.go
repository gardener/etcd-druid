package task

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// AdmitOperation is the operation name for the Admit phase of a task lifecycle.
	AdmitOperation = "Admit"
	// RunOperation is the operation name for the Run phase of a task lifecycle.
	RunOperation = "Run"
	// CleanupOperation is the operation name for the Cleanup phase of a task lifecycle.
	CleanupOperation = "Cleanup"
)

// Result defines the result of a task execution.
type Result struct {
	Description  string
	Error        error
	RequeueAfter time.Duration // Duration to requeue the task
	Completed    bool
}

// Handler defines the interface for task execution.
type Handler interface {
	EtcdReference() types.NamespacedName
	Name() string
	Logger() logr.Logger
	// Checks if the task is permitted to run. This is a one-time gate; once passed, it is not checked again for the same task execution.
	Admit(ctx context.Context) *Result
	Run(ctx context.Context) *Result     // The Run method will have to check if the necessary pre conditions hold true upon each call.
	Cleanup(ctx context.Context) *Result // Will be triggered once the task is in a completed state.
}
