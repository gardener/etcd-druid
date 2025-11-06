package printer

// OutputFormat defines the type for output format, e.g., json, yaml, table
type OutputFormat string

const (
	// OutputTypeJSON represents JSON output format
	OutputTypeJSON OutputFormat = "json"
	// OutputTypeJSONRaw represents raw JSON output format
	OutputTypeJSONRaw OutputFormat = "json-raw"
	// OutputTypeYAML represents YAML output format
	OutputTypeYAML OutputFormat = "yaml"
	// OutputTypeTable represents table output format
	OutputTypeTable OutputFormat = "table"
	// OutputTypeNone represents no output format
	OutputTypeNone OutputFormat = ""
)
