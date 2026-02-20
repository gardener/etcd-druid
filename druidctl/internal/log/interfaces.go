// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package log

import "io"

// Formatter handles styling text for different log types
type Formatter interface {
	FormatSuccess(message string) string
	FormatError(message string, err error) string
	FormatInfo(message string) string
	FormatWarning(message string) string
	FormatHeader(message string) string
	FormatProgress(message string) string
	FormatStart(message string) string
	FormatEtcdOperation(operation, etcdName, namespace string, allNamespaces bool) string
}

// Writer handles log delivery to the underlying logger mechanism
type Writer interface {
	LogInfo(writer io.Writer, message string)
	LogError(writer io.Writer, message string, keyvals ...any)
	LogWarn(writer io.Writer, message string)
	WriteRaw(writer io.Writer, message string)

	// Configuration
	SetVerbose(verbose bool)
	SetOutput(w io.Writer)
	IsVerbose() bool
}

// Logger combines formatting and writing for complete log handling
type Logger interface {
	Success(writer io.Writer, message string, params ...string)
	Error(writer io.Writer, message string, err error, params ...string)
	Info(writer io.Writer, message string, params ...string)
	Warning(writer io.Writer, message string, params ...string)
	Header(writer io.Writer, message string, params ...string)
	RawHeader(writer io.Writer, message string)
	Progress(writer io.Writer, message string, params ...string)
	Start(writer io.Writer, message string, params ...string)

	// Configuration
	SetVerbose(verbose bool)
	SetOutput(w io.Writer)
	IsVerbose() bool
}
