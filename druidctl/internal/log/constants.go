// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package log

// LogType defines the type for logging service, e.g., Charm
type LogType string

const (
	// LogTypeCharm selects the Charm logger implementation.
	LogTypeCharm LogType = "Charm"
)
