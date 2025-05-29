// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"

	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	logzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// MustNewLogger is like NewLogger but panics on invalid input.
func MustNewLogger(devMode bool, level druidconfigv1alpha1.LogLevel, format druidconfigv1alpha1.LogFormat) logr.Logger {
	logger, err := NewLogger(devMode, level, format)
	utilruntime.Must(err)
	return logger
}

// NewLogger creates a new logr.Logger backed by Zap.
func NewLogger(devMode bool, level druidconfigv1alpha1.LogLevel, format druidconfigv1alpha1.LogFormat) (logr.Logger, error) {
	zapOpts, err := buildDefaultLoggerOpts(devMode, level, format)
	if err != nil {
		return logr.Logger{}, err
	}
	return logzap.New(zapOpts...), nil
}

func buildDefaultLoggerOpts(devMode bool, level druidconfigv1alpha1.LogLevel, format druidconfigv1alpha1.LogFormat) ([]logzap.Opts, error) {
	var opts []logzap.Opts
	opts = append(opts, logzap.UseDevMode(devMode))

	// map log levels to zap levels
	var zapLevel zapcore.LevelEnabler
	switch level {
	case druidconfigv1alpha1.LogLevelDebug:
		zapLevel = zapcore.DebugLevel
	case druidconfigv1alpha1.LogLevelInfo, "":
		zapLevel = zapcore.InfoLevel
	case druidconfigv1alpha1.LogLevelError:
		zapLevel = zapcore.ErrorLevel
	default:
		return []logzap.Opts{}, fmt.Errorf("invalid log level %q", level)
	}
	opts = append(opts, logzap.Level(zapLevel))

	// map log format to encoder
	switch format {
	case druidconfigv1alpha1.LogFormatText:
		opts = append(opts, logzap.ConsoleEncoder(setCommonEncoderConfigOptions))
	case druidconfigv1alpha1.LogFormatJSON:
		opts = append(opts, logzap.JSONEncoder(setCommonEncoderConfigOptions))
	default:
		return []logzap.Opts{}, fmt.Errorf("invalid log format %q", format)
	}
	opts = append(opts, logzap.JSONEncoder(func(encoderConfig *zapcore.EncoderConfig) {
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	}))
	return opts, nil
}

func setCommonEncoderConfigOptions(encoderConfig *zapcore.EncoderConfig) {
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
}
