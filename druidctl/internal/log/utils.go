// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"github.com/gardener/etcd-druid/druidctl/internal/log/charm"
)

// DefaultLogger returns the default logger implementation used by the CLI.
func DefaultLogger() Logger {
	return charm.NewCharmLogger()
}

// NewLogger constructs a logger based on the provided LoggerKind.
func NewLogger(loggerKind LoggerKind) Logger {
	switch loggerKind {
	case LoggerKindCharm:
		return charm.NewCharmLogger()
	default:
		return DefaultLogger()
	}
}
