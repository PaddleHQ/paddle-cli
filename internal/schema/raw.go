// Package schema renders request-body JSON Schema for generated CLI
// commands, as an indented, human-readable tree for --help, or as raw
// JSON (pretty or compact) for the --schema flag.
package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// PrintRaw writes schemaJSON to w as JSON. When compact is false the output
// is pretty-printed with a 2-space indent; when true it is the smallest
// valid JSON representation. A trailing newline is always written.
//
// If w is nil, os.Stdout is used.
func PrintRaw(w io.Writer, schemaJSON string, compact bool) error {
	if w == nil {
		w = os.Stdout
	}
	// Validate the JSON without re-marshaling through a map (which would
	// re-order keys). Use json.RawMessage so the original key order is
	// preserved in both compact and pretty output.
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(schemaJSON), &raw); err != nil {
		return fmt.Errorf("parsing schema JSON: %w", err)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if !compact {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(raw); err != nil {
		return fmt.Errorf("writing schema JSON: %w", err)
	}
	return nil
}
