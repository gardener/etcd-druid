// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package printer

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/yaml"
)

// Printer defines an interface for formatting data into various output formats.
type Printer interface {
	Print(data any) ([]byte, error)
}

// =======================
// JSON Formatter
// =======================

// JSONFormatter formats data as JSON.
// Indent controls whether the output is pretty-printed.
type JSONFormatter struct {
	Indent bool
}

// Print marshals the provided data into JSON. It pretty-prints if Indent is true.
func (f *JSONFormatter) Print(data any) ([]byte, error) {
	if f.Indent {
		return json.MarshalIndent(data, "", "  ")
	}
	return json.Marshal(data)
}

// =======================
// YAML Formatter
// =======================

// YAMLFormatter formats data as YAML.
type YAMLFormatter struct{}

// Print marshals the provided data into YAML.
func (f *YAMLFormatter) Print(data any) ([]byte, error) {
	return yaml.Marshal(data)
}

// =======================
// Table Formatter
// =======================

// TableFormatter formats data as a table.
// TODO:@anveshreddy18 -> Implementation to be added.
type TableFormatter struct{}

// Format converts the given data into a table representation.
func (f *TableFormatter) Format(_ any) ([]byte, error) {
	// Implement table formatting logic here. Currently returns no data.
	return nil, nil
}

// =======================
// Formatter Factory
// =======================

// NewFormatter creates a new Formatter based on the specified output format.
func NewFormatter(format OutputFormat) (Printer, error) {
	switch format {
	case OutputTypeJSON:
		return &JSONFormatter{Indent: true}, nil
	case OutputTypeJSONRaw:
		return &JSONFormatter{Indent: false}, nil
	case OutputTypeYAML:
		return &YAMLFormatter{}, nil
	case OutputTypeNone:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown format: %s", string(format))
	}
}
