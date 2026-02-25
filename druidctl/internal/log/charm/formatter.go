// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package charm provides log output formatting and writing implementations using charmbracelet libraries
package charm

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// CharmFormatter implements log.Formatter using Lipgloss
type CharmFormatter struct {
	styles struct {
		Success lipgloss.Style
		Error   lipgloss.Style
		Info    lipgloss.Style
		Warning lipgloss.Style
		Header  lipgloss.Style
		Key     lipgloss.Style
		Value   lipgloss.Style
	}
}

// NewCharmFormatter creates a new CharmFormatter with default styles
func NewCharmFormatter() *CharmFormatter {
	f := &CharmFormatter{}

	// Initialize styles
	f.styles.Success = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	f.styles.Error = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	f.styles.Info = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	f.styles.Warning = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	f.styles.Header = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	f.styles.Key = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	f.styles.Value = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))

	return f
}

// FormatSuccess formats a success message with green checkmark
func (f *CharmFormatter) FormatSuccess(message string) string {
	return f.styles.Success.Render("✓ " + message)
}

// FormatError formats an error message with red X
func (f *CharmFormatter) FormatError(message string, err error) string {
	errMsg := message
	if err != nil {
		errMsg = fmt.Sprintf("%s: %s", message, err.Error())
	}
	return f.styles.Error.Render(errMsg)
}

// FormatInfo formats an info message with blue info icon
func (f *CharmFormatter) FormatInfo(message string) string {
	return f.styles.Info.Render(message)
}

// FormatWarning formats a warning message with yellow warning icon
func (f *CharmFormatter) FormatWarning(message string) string {
	return f.styles.Warning.Render(message)
}

// FormatHeader formats a header message in magenta
func (f *CharmFormatter) FormatHeader(message string) string {
	return f.styles.Header.Render(message)
}

// FormatProgress formats a progress message
func (f *CharmFormatter) FormatProgress(message string) string {
	return f.styles.Info.Render(message)
}

// FormatStart formats a start message
func (f *CharmFormatter) FormatStart(message string) string {
	return f.styles.Info.Render(message)
}
