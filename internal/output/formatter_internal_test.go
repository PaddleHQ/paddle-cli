package output

import (
	"bytes"
	"strings"
	"testing"
)

// TestPrint_JQFallsBackForPrimitives covers the case where a jq expression
// produces a value that cannot be rendered as a row (a bare string, number,
// bool, or null). Before the fallback landed, table/CSV would error out with
// "unexpected JSON shape: ..."; now the value is printed on its own line so
// jq works uniformly across formats.
func TestPrint_JQFallsBackForPrimitives(t *testing.T) {
	input := []byte(`{"data":[{"id":"pro_01"},{"id":"pro_02"}]}`)

	cases := []struct {
		name   string
		format Format
		jq     string
		want   string
	}{
		{"table: scalar from length", FormatTable, ".data | length", "2\n"},
		{"table: single string select", FormatTable, ".data[0].id", "pro_01\n"},
		{"table: boolean select", FormatTable, ".data | length > 0", "true\n"},
		{"csv: scalar from length", FormatCSV, ".data | length", "2\n"},
		{"csv: single string select", FormatCSV, ".data[0].id", "pro_01\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			opts := Options{
				Format: tc.format,
				JQ:     tc.jq,
				Writer: &buf,
			}
			if err := Print(input, nil, opts); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPrint_JQMultiPrimitiveResults verifies that a jq expression yielding
// multiple primitive results (e.g. '.data[].id') prints one value per line
// for both table and CSV formats, rather than rendering an empty table.
func TestPrint_JQMultiPrimitiveResults(t *testing.T) {
	input := []byte(`{"data":[{"id":"pro_01"},{"id":"pro_02"},{"id":"pro_03"}]}`)

	for _, format := range []Format{FormatTable, FormatCSV} {
		t.Run(string(format), func(t *testing.T) {
			var buf bytes.Buffer
			opts := Options{
				Format: format,
				JQ:     ".data[].id",
				Writer: &buf,
			}
			if err := Print(input, nil, opts); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := "pro_01\npro_02\npro_03\n"
			if got := buf.String(); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// TestPrint_JQReshapesTableColumns verifies that when jq transforms the rows
// into a new shape (e.g. 'map({id})'), the table's inferred column set
// reflects the new shape rather than the original server response.
func TestPrint_JQReshapesTableColumns(t *testing.T) {
	input := []byte(`{"data":[{"id":"pro_01","name":"Pro","price":100}]}`)

	var buf bytes.Buffer
	opts := Options{
		Format: FormatTable,
		JQ:     ".data | map({id})",
		Writer: &buf,
	}
	if err := Print(input, nil, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ID") {
		t.Errorf("expected 'ID' header in output, got:\n%s", out)
	}
	if strings.Contains(out, "NAME") || strings.Contains(out, "PRICE") {
		t.Errorf("expected dropped columns not to appear, got:\n%s", out)
	}
}

// TestPrint_JQPreservesColumnOrder verifies that a jq expression that projects
// a specific field order (e.g. map({id, status, origin})) produces table
// columns in that order rather than alphabetically.
func TestPrint_JQPreservesColumnOrder(t *testing.T) {
	input := []byte(`{"data":[{"id":"txn_1","status":"draft","origin":"web","collection_mode":"automatic","currency_code":"USD"}]}`)

	cases := []struct {
		name      string
		jq        string
		wantOrder []string
	}{
		{
			"shorthand fields",
			`.data | map({id, status, origin, collection_mode, currency_code})`,
			[]string{"ID", "STATUS", "ORIGIN", "COLLECTION_MODE", "CURRENCY_CODE"},
		},
		{
			"explicit key-value fields",
			`.data | map({"id": .id, "status": .status, "origin": .origin})`,
			[]string{"ID", "STATUS", "ORIGIN"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			opts := Options{Format: FormatTable, JQ: tc.jq, Writer: &buf}
			if err := Print(input, nil, opts); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			out := buf.String()
			// Find each header and verify they appear left-to-right in order.
			pos := -1
			for _, hdr := range tc.wantOrder {
				idx := strings.Index(out, hdr)
				if idx < 0 {
					t.Errorf("header %q not found in output:\n%s", hdr, out)
					continue
				}
				if idx <= pos {
					t.Errorf("header %q appears before previous header in output:\n%s", hdr, out)
				}
				pos = idx
			}
		})
	}
}

// TestJQProjectedKeys verifies that the AST-based key extractor returns fields
// in the order they are written in the jq expression.
func TestJQProjectedKeys(t *testing.T) {
	cases := []struct {
		expr string
		want []string
	}{
		{`.data | map({id, status, origin})`, []string{"id", "status", "origin"}},
		{`map({"id": .id, "status": .status})`, []string{"id", "status"}},
		{`[.[] | {name, id}]`, []string{"name", "id"}},
		{`.data[0:3]`, nil},
		{`invalid jq !!!`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			got := jqProjectedKeys(tc.expr)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestPrint_JQStillRendersObjectsAsTable guards the happy path: when the
// filter returns a tabular shape, table/CSV rendering still kicks in rather
// than falling back to plain output.
func TestPrint_JQStillRendersObjectsAsTable(t *testing.T) {
	input := []byte(`{"data":[{"id":"pro_01","name":"Pro"},{"id":"pro_02","name":"Starter"}]}`)

	var buf bytes.Buffer
	opts := Options{
		Format: FormatTable,
		JQ:     ".data[0]",
		Writer: &buf,
	}
	if err := Print(input, nil, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "pro_01") || !strings.Contains(out, "Pro") {
		t.Errorf("expected tabular output for object result, got:\n%s", out)
	}
	if strings.Contains(out, "pro_02") {
		t.Errorf("jq filter did not limit to first item, got:\n%s", out)
	}
}

// TestIsTabular covers the shape detection directly so future reshapings
// (e.g. handling null or numeric arrays) have a clear contract.
func TestIsTabular(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"object", `{"id":"1"}`, true},
		{"empty array", `[]`, true},
		{"array of objects", `[{"id":"1"}]`, true},
		{"bare string", `"abc"`, false},
		{"bare number", `42`, false},
		{"bare bool", `true`, false},
		{"bare null", `null`, false},
		{"array of primitives", `["a","b"]`, false},
		{"invalid JSON", `not json`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTabular([]byte(tc.in)); got != tc.want {
				t.Errorf("isTabular(%s) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestPrint_EmptyBodyEmitsNothing covers 204 No Content responses (e.g. the
// delete commands): the body must not be parsed as JSON, nothing is written,
// and no error is returned in any format.
func TestPrint_EmptyBodyEmitsNothing(t *testing.T) {
	cases := []struct {
		name   string
		format Format
		data   []byte
	}{
		{"json: nil body", FormatJSON, nil},
		{"json: empty body", FormatJSON, []byte{}},
		{"json: whitespace body", FormatJSON, []byte(" \n\t")},
		{"table: empty body", FormatTable, []byte{}},
		{"csv: empty body", FormatCSV, []byte{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			opts := Options{Format: tc.format, Writer: &buf}
			if err := Print(tc.data, nil, opts); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := buf.String(); got != "" {
				t.Errorf("expected no output, got %q", got)
			}
		})
	}
}
