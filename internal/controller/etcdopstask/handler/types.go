// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
)

// Result defines the result of a task execution.
type Result struct {
	Description string
	Error       error
	Completed   bool
}

// Handler defines the interface for task execution.
type Handler interface {
	// Admit checks if the task is permitted to run. This is a one-time gate; once passed, it is not checked again for the same task execution.
	Admit(ctx context.Context) Result
	// Run executes the main logic of the task. Is executed after Admit has returned a successful result.
	Run(ctx context.Context) Result
	// Cleanup performs any necessary cleanup after task execution, regardless of success or failure.
	Cleanup(ctx context.Context) Result
}
