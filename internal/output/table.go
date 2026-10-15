package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/PaddleHQ/paddle-cli/internal/style"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

var (
	errUnexpectedShape     = errors.New("unexpected JSON shape")
	errUnexpectedDataField = errors.New("unexpected data field type")
)

// styleField applies the profile's semantic helper appropriate for the
// column field name. Status-like columns use Status(); id-like columns
// use ID(); everything else passes through.
func styleField(profile style.Profile, field, value string) string {
	if !profile.IsEnabled() || value == "" {
		return value
	}
	switch {
	case isFieldKind(field, "status") || isFieldKind(field, "state"):
		return profile.Status(value)
	case isFieldKind(field, "id"):
		return profile.ID(value)
	case isFieldKind(field, "email"):
		return profile.Email(value)
	case value == "true" || value == "false":
		return profile.Bool(value)
	}
	return value
}

// isFieldKind reports whether field is kind itself or a "_kind"-suffixed name.
func isFieldKind(field, kind string) bool {
	return field == kind || strings.HasSuffix(field, "_"+kind)
}

// printTable renders data as a styled table using go-pretty.
func printTable(data []byte, columns []ColumnDef, opts Options) error {
	rows, err := extractRows(data)
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(opts.Writer, "No results.")
		return nil
	}

	if len(columns) == 0 {
		columns = inferColumns(rows[0])
	}
	columns = dropEmptyColumns(rows, columns)

	t := table.NewWriter()
	t.SetOutputMirror(opts.Writer)
	t.SetStyle(table.StyleRounded)
	// Preserve header text exactly as supplied: the caller has already
	// chosen the casing (inferred columns are uppercased, explicit columns
	// keep whatever the caller set).
	t.Style().Format.Header = text.FormatDefault
	// Header styling now goes via the profile so the no-color path is the
	// same one all other surfaces use. Keep this conditional: go-pretty's
	// renderer uses the Color block to keep column padding stable.
	if opts.Profile.IsEnabled() {
		t.Style().Color.Header = text.Colors{text.Bold}
	}

	header := make(table.Row, len(columns))
	for i, col := range columns {
		header[i] = col.Header
	}
	t.AppendHeader(header)

	for _, row := range rows {
		cells := make(table.Row, len(columns))
		for i, col := range columns {
			val := extractField(row, col.Field)
			if !opts.NoTrunc && len(val) > 64 {
				val = val[:61] + "..."
			}
			cells[i] = styleField(opts.Profile, col.Field, val)
		}
		t.AppendRow(cells)
	}

	t.Render()
	return nil
}

// extractRows pulls the row slice from a Paddle API response.
// Handles both {"data":[...]} and {"data":{...}} response shapes,
// as well as bare arrays and bare objects.
func extractRows(data []byte) ([]map[string]any, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	switch v := raw.(type) {
	case map[string]any:
		if dataField, ok := v["data"]; ok {
			return toRows(dataField)
		}
		// Bare object: treat as single row.
		return []map[string]any{toStringMap(v)}, nil
	case []any:
		return toRowsFromSlice(v)
	default:
		return nil, fmt.Errorf("%w: %T", errUnexpectedShape, raw)
	}
}

func toRows(v any) ([]map[string]any, error) {
	switch val := v.(type) {
	case []any:
		return toRowsFromSlice(val)
	case map[string]any:
		return []map[string]any{toStringMap(val)}, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: %T", errUnexpectedDataField, v)
	}
}

func toRowsFromSlice(items []any) ([]map[string]any, error) {
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			rows = append(rows, toStringMap(m))
		}
	}
	return rows, nil
}

// toStringMap returns a copy of m with all values converted to a flat
// representation suitable for display.
func toStringMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	return out
}

// extractField retrieves a dot-separated path from a row map.
// Nested values are marshaled to JSON strings.
func extractField(row map[string]any, field string) string {
	parts := strings.SplitN(field, ".", 2)
	v, ok := row[parts[0]]
	if !ok {
		return ""
	}
	if len(parts) == 2 {
		if nested, ok := v.(map[string]any); ok {
			return extractField(nested, parts[1])
		}
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		var buf strings.Builder
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(val); err != nil {
			return fmt.Sprintf("%v", val)
		}
		return strings.TrimRight(buf.String(), "\n")
	}
}

// dropEmptyColumns returns columns whose corresponding values in rows are
// not all empty. "Empty" matches what the renderer would print as an
// empty cell: `nil`, missing keys, the empty string, or any value whose
// rendered form via extractField looks empty (`""`, `"[]"`, `"{}"`,
// `"null"`). Returns the original slice if every column would be dropped,
// so we never emit a zero-column table.
func dropEmptyColumns(rows []map[string]any, columns []ColumnDef) []ColumnDef {
	if len(rows) == 0 || len(columns) == 0 {
		return columns
	}
	kept := make([]ColumnDef, 0, len(columns))
	for _, col := range columns {
		for _, row := range rows {
			if !isEmptyCell(extractField(row, col.Field)) {
				kept = append(kept, col)
				break
			}
		}
	}
	if len(kept) == 0 {
		return columns
	}
	return kept
}

// isEmptyCell reports whether a rendered cell value should be treated as
// empty for the purpose of dropping the column. Strings empty after
// extractField (missing key, nil, "") plus the conventional empty
// container renderings ("[]", "{}", "null") all count as empty;
// otherwise a column of {} placeholders survives the filter and shows
// up as user-visible noise, which defeats the feature's purpose.
// Booleans ("false") and numeric zeros ("0") are intentionally NOT
// treated as empty: those carry information.
func isEmptyCell(s string) bool {
	switch s {
	case "", "[]", "{}", "null":
		return true
	}
	return false
}

// inferColumns creates column definitions from the keys of the first row.
func inferColumns(row map[string]any) []ColumnDef {
	cols := make([]ColumnDef, 0, len(row))
	for k := range row {
		cols = append(cols, ColumnDef{Header: strings.ToUpper(k), Field: k})
	}
	return cols
}
