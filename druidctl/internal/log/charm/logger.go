// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package charm provides log output formatting and writing implementations using charmbracelet libraries
package charm

import (
	"fmt"
	"io"
)

// CharmLogger implements log.Logger using CharmFormatter and CharmWriter
type CharmLogger struct {
	formatter *CharmFormatter
	writer    *CharmWriter
}

// NewCharmLogger creates a new CharmService with default settings
func NewCharmLogger() *CharmLogger {
	return &CharmLogger{
		formatter: NewCharmFormatter(),
		writer:    NewCharmWriter(),
	}
}

func constructPrefixFromParams(params ...string) string {
	prefix := ""
	// Check if both etcdName and namespace are provided
	if len(params) >= 2 {
		etcdName := params[0]
		namespace := params[1]
		prefix = fmt.Sprintf("[%s/%s] ", namespace, etcdName)
	}
	return prefix
}

// Success displays a success message
func (s *CharmLogger) Success(writer io.Writer, message string, params ...string) {
	s.writer.LogInfo(writer, s.formatter.FormatHeader(constructPrefixFromParams(params...))+s.formatter.FormatSuccess(message))
}

// Error displays an error message
func (s *CharmLogger) Error(writer io.Writer, message string, err error, params ...string) {
	s.writer.LogError(writer, s.formatter.FormatHeader(constructPrefixFromParams(params...))+s.formatter.FormatError(message, err), "error", err)
}

// Info displays an informational message
func (s *CharmLogger) Info(writer io.Writer, message string, params ...string) {
	s.writer.LogInfo(writer, s.formatter.FormatHeader(constructPrefixFromParams(params...))+s.formatter.FormatInfo(message))
}

// Warning displays a warning message
func (s *CharmLogger) Warning(writer io.Writer, message string, params ...string) {
	s.writer.LogWarn(writer, s.formatter.FormatHeader(constructPrefixFromParams(params...))+s.formatter.FormatWarning(message))
}

// Header displays a header message
func (s *CharmLogger) Header(writer io.Writer, message string, params ...string) {
	s.writer.LogInfo(writer, s.formatter.FormatHeader(constructPrefixFromParams(params...))+s.formatter.FormatHeader(message))
}

// RawHeader writes a formatted header line directly without log prefixes.
func (s *CharmLogger) RawHeader(writer io.Writer, message string) {
	s.writer.WriteRaw(writer, s.formatter.FormatHeader(message))
}

// Progress displays a progress message
func (s *CharmLogger) Progress(writer io.Writer, message string, params ...string) {
	s.writer.LogInfo(writer, s.formatter.FormatHeader(constructPrefixFromParams(params...))+s.formatter.FormatProgress(message))
}

// Start displays a start message
func (s *CharmLogger) Start(writer io.Writer, message string, params ...string) {
	s.writer.LogInfo(writer, s.formatter.FormatHeader(constructPrefixFromParams(params...))+s.formatter.FormatStart(message))
}

// SetVerbose enables or disables verbose mode
func (s *CharmLogger) SetVerbose(verbose bool) {
	s.writer.SetVerbose(verbose)
}

// SetOutput sets the output writer
func (s *CharmLogger) SetOutput(w io.Writer) {
	s.writer.SetOutput(w)
}

// IsVerbose returns whether verbose mode is enabled
func (s *CharmLogger) IsVerbose() bool {
	return s.writer.IsVerbose()
}
