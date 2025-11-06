// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"github.com/gardener/etcd-druid/druidctl/internal/log/charm"
)

func DefaultLogger() Logger {
	return charm.NewCharmLogger()
}

func NewLogger(logType LogType) Logger {
	switch logType {
	case LogTypeCharm:
		return charm.NewCharmLogger()
	default:
		return DefaultLogger()
	}
}
