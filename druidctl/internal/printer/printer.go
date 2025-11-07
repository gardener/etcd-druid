// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
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
	Print(data interface{}) ([]byte, error)
}

// =======================
// JSON Formatter
// =======================

// JSONFormatter formats data as JSON.
// Indent controls whether the output is pretty-printed.
type JSONFormatter struct {
	Indent bool
}

func (f *JSONFormatter) Print(data interface{}) ([]byte, error) {
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

func (f *YAMLFormatter) Print(data interface{}) ([]byte, error) {
	return yaml.Marshal(data)
}

// =======================
// Table Formatter
// =======================

// TableFormatter formats data as a table.
// (Implementation to be added)
type TableFormatter struct{}

func (f *TableFormatter) Format(data interface{}) ([]byte, error) {
	// Implement table formatting logic here
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
