package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/PaddleHQ/paddle-cli/internal/style"

	"github.com/itchyny/gojq"
)

// Format represents the output format for API responses.
type Format string

// Supported output formats.
const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatCSV   Format = "csv"
)

var (
	errUnknownFormat     = errors.New("unknown format")
	errUnsupportedFormat = errors.New("unsupported format")
	errNotObject         = errors.New("expected object")
	errNonStringKey      = errors.New("non-string key")
)

// ParseFormat parses a format string into a Format value.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatTable, FormatJSON, FormatCSV:
		return Format(s), nil
	default:
		return "", fmt.Errorf("%w %q, expected one of: table, json, csv", errUnknownFormat, s)
	}
}

// Options controls how output is rendered.
type Options struct {
	Format  Format
	JQ      string
	Profile style.Profile
	NoTrunc bool
	// Compact disables JSON indentation. Agent mode sets this so structured
	// output consumed by another process stays lean.
	Compact bool
	// Environment is the resolved active environment name (e.g. "sandbox",
	// "production"). Set by Root.setup. JSON output uses it to inject
	// meta.environment; the banner uses it to render the footer.
	Environment string
	// NextCommand, if non-empty, is injected as meta.pagination.next_command in
	// JSON output and written to stderr as a pagination hint after the banner.
	// Populated by generated list commands when the response has has_more=true.
	NextCommand string
	Writer      io.Writer
}

// DefaultOptions returns options suitable for interactive terminal use.
func DefaultOptions() Options {
	return Options{
		Format: FormatTable,
		Writer: os.Stdout,
	}
}

// AgentOptions returns options suitable for machine/agent consumption.
func AgentOptions() Options {
	return Options{
		Format:  FormatJSON,
		NoTrunc: true,
		Compact: true,
		Writer:  os.Stdout,
	}
}

// ColumnDef defines a column for table and CSV output.
type ColumnDef struct {
	Header string
	Field  string
}

// Print renders data in the specified format to opts.Writer.
// data must be the raw JSON response body from the Paddle API.
// columns defines which fields to display for table and CSV formats.
//
// When opts.JQ is set, the expression is applied as a preprocessing step for
// table and CSV formats so filters like '.data[0:3]' limit what gets rendered.
// If the filtered result is not tabular (e.g. a primitive or an array of
// primitives) we fall back to line-based output so jq works uniformly across
// formats, matching the JSON format's own jq handler.
func Print(data []byte, columns []ColumnDef, opts Options) error {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	// 204 No Content (e.g. deletes) has nothing to render in any format.
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	switch opts.Format {
	case FormatJSON:
		return printJSON(data, opts)
	case FormatCSV, FormatTable:
		return printTabular(data, columns, opts)
	default:
		return fmt.Errorf("%w: %s", errUnsupportedFormat, opts.Format)
	}
}

// printTabular renders data as table or CSV, applying opts.JQ first.
func printTabular(data []byte, columns []ColumnDef, opts Options) error {
	// Extract column order from the original server response before JQ
	// runs: encoding/json loses field order as soon as it unmarshals
	// into a map, and JQ's output passes through Marshal which sorts
	// keys alphabetically. Peeking at the raw bytes first preserves the
	// order the server sent.
	if len(columns) == 0 {
		if keys := extractOrderedKeys(data); len(keys) > 0 {
			columns = columnsFromKeys(keys)
		}
	}
	if opts.JQ != "" {
		filtered, err := applyJQ(data, opts.JQ)
		if err != nil {
			return err
		}
		// A jq expression like '.data | length' or '.data[].id' produces
		// non-tabular output: table/CSV can't represent it, so fall
		// back to a line-per-value renderer instead of erroring.
		if !isTabular(filtered) {
			return printPlain(filtered, opts)
		}
		data = filtered
		if keys := jqColumnKeys(opts.JQ, data); len(keys) > 0 {
			columns = columnsFromKeys(keys)
		}
	}
	if opts.Format == FormatCSV {
		return printCSV(data, columns, opts)
	}
	return printTable(data, columns, opts)
}

// jqColumnKeys returns the column order for jq-filtered data. The user wrote
// the fields in a specific order (e.g. map({id, status})) and we honor that;
// falling back to extractOrderedKeys covers expressions that don't project an
// explicit field list.
func jqColumnKeys(expr string, filtered []byte) []string {
	if projected := jqProjectedKeys(expr); len(projected) > 0 {
		return projected
	}
	return extractOrderedKeys(filtered)
}

func columnsFromKeys(keys []string) []ColumnDef {
	columns := make([]ColumnDef, len(keys))
	for i, k := range keys {
		columns[i] = ColumnDef{Header: strings.ToUpper(k), Field: k}
	}
	return columns
}

// isTabular reports whether data can be rendered as a table, i.e. an object,
// an empty array, or an array whose first element is an object. Primitives
// and arrays of primitives are not tabular.
func isTabular(data []byte) bool {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return false
	}
	switch val := v.(type) {
	case map[string]any:
		return true
	case []any:
		if len(val) == 0 {
			return true
		}
		_, ok := val[0].(map[string]any)
		return ok
	default:
		return false
	}
}

// printPlain renders a non-tabular jq result as one value per line. Strings
// are written unquoted (jq -r style); other scalars and nested objects are
// JSON-encoded. Null values are skipped. Arrays are flattened one level so
// multi-result jq expressions like '.data[].id' print cleanly.
func printPlain(data []byte, opts Options) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return printPlainValue(v, opts)
}

func printPlainValue(v any, opts Options) error {
	switch val := v.(type) {
	case nil:
		return nil
	case string:
		if _, err := fmt.Fprintln(opts.Writer, val); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		return nil
	case []any:
		for _, item := range val {
			if err := printPlainValue(item, opts); err != nil {
				return err
			}
		}
		return nil
	default:
		enc := json.NewEncoder(opts.Writer)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(val); err != nil {
			return fmt.Errorf("encoding JSON: %w", err)
		}
		return nil
	}
}

// extractOrderedKeys returns the field names of the first row in the JSON
// document, preserving the server's original field order. Handles the
// standard Paddle envelope {"data":[...]} / {"data":{...}} as well as bare
// arrays and bare objects. Returns nil if no object can be found.
func extractOrderedKeys(data []byte) []string {
	var root orderedObject
	if err := json.Unmarshal(data, &root); err == nil {
		if inner, ok := root.raw["data"]; ok {
			if keys := firstRowKeys(inner); keys != nil {
				return keys
			}
		}
		return root.keys
	}
	return firstRowKeys(data)
}

func firstRowKeys(data []byte) []string {
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		if len(arr) == 0 {
			return nil
		}
		var obj orderedObject
		if err := json.Unmarshal(arr[0], &obj); err == nil {
			return obj.keys
		}
		return nil
	}
	var obj orderedObject
	if err := json.Unmarshal(data, &obj); err == nil {
		return obj.keys
	}
	return nil
}

// orderedObject decodes a JSON object while recording the order of its
// top-level keys. Values are kept as raw bytes so downstream code can decide
// how to interpret them.
type orderedObject struct {
	keys []string
	raw  map[string]json.RawMessage
}

func (o *orderedObject) UnmarshalJSON(data []byte) error {
	o.raw = make(map[string]json.RawMessage)
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("reading token: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("%w, got %v", errNotObject, tok)
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("reading key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("%w %v", errNonStringKey, keyTok)
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return fmt.Errorf("reading value for %q: %w", key, err)
		}
		o.keys = append(o.keys, key)
		o.raw[key] = val
	}
	return nil
}

// jqProjectedKeys parses expr and returns the keys of the first explicit object
// literal found in the expression, in the order the user wrote them. This
// preserves column order for expressions like `map({id, status, origin})` that
// would otherwise be alphabetised by encoding/json. Returns nil when the
// expression contains no object literal (e.g. `.data[0:3]`).
func jqProjectedKeys(expr string) []string {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil
	}
	return walkQueryForKeys(q)
}

// walkQueryForKeys does a depth-first walk of the gojq AST, preferring the
// right side of pipes so it finds the output-shaping object rather than an
// intermediate one.
func walkQueryForKeys(q *gojq.Query) []string {
	if q == nil {
		return nil
	}
	// Pipe: the right-hand side determines the output shape.
	if q.Op == gojq.OpPipe {
		if keys := walkQueryForKeys(q.Right); keys != nil {
			return keys
		}
		return walkQueryForKeys(q.Left)
	}
	return walkTermForKeys(q.Term)
}

func walkTermForKeys(t *gojq.Term) []string {
	if t == nil {
		return nil
	}
	switch t.Type {
	case gojq.TermTypeObject:
		return objectTermKeys(t.Object)
	case gojq.TermTypeFunc:
		for _, arg := range t.Func.Args {
			if keys := walkQueryForKeys(arg); keys != nil {
				return keys
			}
		}
	case gojq.TermTypeArray:
		if t.Array != nil {
			return walkQueryForKeys(t.Array.Query)
		}
	case gojq.TermTypeQuery:
		return walkQueryForKeys(t.Query)
	default:
		// Other term types cannot project an object literal.
	}
	return nil
}

func objectTermKeys(obj *gojq.Object) []string {
	if obj == nil {
		return nil
	}
	keys := make([]string, 0, len(obj.KeyVals))
	for _, kv := range obj.KeyVals {
		switch {
		case kv.Key != "":
			keys = append(keys, kv.Key)
		case kv.KeyString != nil:
			keys = append(keys, kv.KeyString.Str)
		}
		// KeyQuery (computed key like (.foo)) is skipped: we can't know the
		// value statically.
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

// applyJQ runs the expression against data and returns a single JSON document:
//   - 0 results  → null
//   - 1 result   → that result
//   - N results  → a JSON array of results
//
// This lets non-JSON formatters treat jq as a uniform "filter, then render" step.
func applyJQ(data []byte, expr string) ([]byte, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression %q: %w", expr, err)
	}
	var input any
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq expression: %w", err)
	}

	results, err := collectJQ(code.Run(input))
	if err != nil {
		return nil, err
	}

	var out any
	switch len(results) {
	case 0:
		out = nil
	case 1:
		out = results[0]
	default:
		out = results
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encoding jq result: %w", err)
	}
	return b, nil
}

func collectJQ(iter gojq.Iter) ([]any, error) {
	var results []any
	for {
		v, ok := iter.Next()
		if !ok {
			return results, nil
		}
		if err, ok := v.(error); ok {
			return nil, fmt.Errorf("jq error: %w", err)
		}
		results = append(results, v)
	}
}
