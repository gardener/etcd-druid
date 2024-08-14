package utils

import (
	"fmt"

	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	logzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// LogFormat is the format of the log.
type LogFormat string

const (
	// LogFormatJSON is the JSON log format.
	LogFormatJSON LogFormat = "json"
	// LogFormatText is the text log format.
	LogFormatText LogFormat = "text"
)

// MustNewLogger is like NewLogger but panics on invalid input.
func MustNewLogger(devMode bool, format LogFormat) logr.Logger {
	logger, err := NewLogger(devMode, format)
	utilruntime.Must(err)
	return logger
}

// NewLogger creates a new logr.Logger backed by Zap.
func NewLogger(devMode bool, format LogFormat) (logr.Logger, error) {
	zapOpts, err := buildDefaultLoggerOpts(devMode, format)
	if err != nil {
		return logr.Logger{}, err
	}
	return logzap.New(zapOpts...), nil
}

func buildDefaultLoggerOpts(devMode bool, format LogFormat) ([]logzap.Opts, error) {
	var opts []logzap.Opts
	opts = append(opts, logzap.UseDevMode(devMode))
	switch format {
	case LogFormatText:
		opts = append(opts, logzap.ConsoleEncoder(setCommonEncoderConfigOptions))
	case LogFormatJSON:
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
