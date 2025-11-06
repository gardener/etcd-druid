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
