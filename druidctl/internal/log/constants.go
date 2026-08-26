// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package log

// LoggerKind defines the logger implementation to use, e.g., Charm
type LoggerKind string

const (
	// LoggerKindCharm selects the Charm logger implementation.
	LoggerKindCharm LoggerKind = "Charm"
)
