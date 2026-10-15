package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/style"
)

func TestPrintTable_BasicOutput(t *testing.T) {
	input := []byte(`{"data":[{"id":"pro_01","name":"Pro Plan"},{"id":"pro_02","name":"Starter Plan"}]}`)
	cols := []ColumnDef{
		{Header: "ID", Field: "id"},
		{Header: "Name", Field: "name"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format: FormatTable,
		Writer: &buf,
	}
	if err := printTable(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ID") {
		t.Errorf("expected output to contain header 'ID', got: %s", out)
	}
	if !strings.Contains(out, "Name") {
		t.Errorf("expected output to contain header 'Name', got: %s", out)
	}
	if !strings.Contains(out, "pro_01") {
		t.Errorf("expected output to contain 'pro_01', got: %s", out)
	}
	if !strings.Contains(out, "Pro Plan") {
		t.Errorf("expected output to contain 'Pro Plan', got: %s", out)
	}
}

func TestPrintTable_EmptyData(t *testing.T) {
	input := []byte(`{"data":[]}`)
	cols := []ColumnDef{
		{Header: "ID", Field: "id"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format: FormatTable,
		Writer: &buf,
	}
	if err := printTable(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "No results.") {
		t.Errorf("expected 'No results.' message, got: %s", out)
	}
}

// TestPrintTable_ColumnsHaveSpacing verifies that adjacent columns are
// separated by whitespace even when the content fills the column width
// exactly. Prior to the fix, cell styles had zero padding so values ran
// directly into each other.
func TestPrintTable_ColumnsHaveSpacing(t *testing.T) {
	// Headers are chosen so that each column's content exactly fills the
	// header width (e.g. "aa"/"bb" match "A"/"B" only after padding is
	// applied). If padding is missing, "aa" and "bb" are rendered as "aabb".
	input := []byte(`{"data":[{"a":"aa","b":"bb"}]}`)
	cols := []ColumnDef{
		{Header: "A", Field: "a"},
		{Header: "B", Field: "b"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format: FormatTable,
		Writer: &buf,
	}
	if err := printTable(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "aabb") {
		t.Errorf("adjacent cells 'aa' and 'bb' have no separator; columns are not padded:\n%s", out)
	}
	// Header row equivalently should not have "AB" adjacency.
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "AB") {
			t.Errorf("headers 'A' and 'B' have no separator:\n%s", out)
		}
	}
}

// TestPrint_JQLimitsTableRows verifies that a jq filter is applied before
// the table formatter runs, so expressions like '.data[0:2]' reduce the
// number of rows rendered.
func TestPrint_JQLimitsTableRows(t *testing.T) {
	input := []byte(`{"data":[
		{"id":"1","name":"one"},
		{"id":"2","name":"two"},
		{"id":"3","name":"three"},
		{"id":"4","name":"four"}
	]}`)
	cols := []ColumnDef{
		{Header: "ID", Field: "id"},
		{Header: "Name", Field: "name"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format: FormatTable,
		JQ:     ".data[0:2]",
		Writer: &buf,
	}
	if err := Print(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("expected 'one' and 'two' in output, got:\n%s", out)
	}
	if strings.Contains(out, "three") || strings.Contains(out, "four") {
		t.Errorf("jq filter did not limit rows: 'three'/'four' should not appear:\n%s", out)
	}
}

// TestPrint_PreservesServerColumnOrder verifies that when columns are
// inferred, they come out in the same order the server sent them, not
// alphabetically and not in the random order map iteration gives you.
func TestPrint_PreservesServerColumnOrder(t *testing.T) {
	// Craft an envelope with keys in a deliberate non-alphabetical order.
	input := []byte(`{"data":[{"zebra":"z1","apple":"a1","mango":"m1"}]}`)

	var buf bytes.Buffer
	opts := Options{
		Format: FormatTable,
		Writer: &buf,
	}
	if err := Print(input, nil, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	// Find the line that actually contains the headers (go-pretty draws
	// border lines above/below so it isn't necessarily line 0).
	var header string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "ZEBRA") {
			header = line
			break
		}
	}
	zIdx := strings.Index(header, "ZEBRA")
	aIdx := strings.Index(header, "APPLE")
	mIdx := strings.Index(header, "MANGO")
	if zIdx < 0 || aIdx < 0 || mIdx < 0 {
		t.Fatalf("expected all three headers present, output was:\n%s", out)
	}
	if zIdx >= aIdx || aIdx >= mIdx {
		t.Errorf("columns not in server order (zebra, apple, mango): header=%q", header)
	}
}

func TestPrintTable_StatusColumnIsColoured(t *testing.T) {
	input := []byte(`{"data":[
		{"id":"sub_01","status":"active"},
		{"id":"sub_02","status":"canceled"},
		{"id":"sub_03","status":"trialing"}
	]}`)
	cols := []ColumnDef{
		{Header: "ID", Field: "id"},
		{Header: "Status", Field: "status"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format:  FormatTable,
		Profile: style.Enabled(),
		Writer:  &buf,
	}
	if err := printTable(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	// Each of the three states gets a different ANSI sequence:
	//   active   → green  (\x1b[32m)
	//   canceled → red    (\x1b[31m)
	//   trialing → yellow (\x1b[33m)
	for _, seq := range []string{"\x1b[32m", "\x1b[31m", "\x1b[33m"} {
		if !strings.Contains(out, seq) {
			t.Errorf("expected ANSI %q in output:\n%s", seq, out)
		}
	}
}

func TestPrintTable_IDColumnIsCyan(t *testing.T) {
	input := []byte(`{"data":[{"id":"pro_01","name":"x"}]}`)
	cols := []ColumnDef{
		{Header: "ID", Field: "id"},
		{Header: "Name", Field: "name"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format:  FormatTable,
		Profile: style.Enabled(),
		Writer:  &buf,
	}
	if err := printTable(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	// Cyan is ANSI 36.
	if !strings.Contains(out, "\x1b[36m") {
		t.Errorf("expected cyan ANSI \\x1b[36m for id column:\n%s", out)
	}
}

func TestPrintTable_EmailColumnIsBrightMagenta(t *testing.T) {
	input := []byte(`{"data":[{"id":"ctm_01","email":"user@example.com"}]}`)
	cols := []ColumnDef{
		{Header: "ID", Field: "id"},
		{Header: "Email", Field: "email"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format:  FormatTable,
		Profile: style.Enabled(),
		Writer:  &buf,
	}
	if err := printTable(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	// Bright magenta is ANSI 95.
	if !strings.Contains(out, "\x1b[95m") {
		t.Errorf("expected bright magenta ANSI \\x1b[95m for email column:\n%s", out)
	}
}

func TestPrintTable_BooleanColumnsAreColoured(t *testing.T) {
	input := []byte(`{"data":[
		{"name":"A","active":true,"marketing_consent":false}
	]}`)
	cols := []ColumnDef{
		{Header: "Name", Field: "name"},
		{Header: "Active", Field: "active"},
		{Header: "Marketing Consent", Field: "marketing_consent"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format:  FormatTable,
		Profile: style.Enabled(),
		Writer:  &buf,
	}
	if err := printTable(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	// true → green (\x1b[32m), false → red (\x1b[31m)
	for _, seq := range []string{"\x1b[32m", "\x1b[31m"} {
		if !strings.Contains(out, seq) {
			t.Errorf("expected ANSI %q in output:\n%s", seq, out)
		}
	}
}

func TestPrintTable_SingleObject(t *testing.T) {
	input := []byte(`{"data":{"id":"pro_01","name":"Pro Plan"}}`)
	cols := []ColumnDef{
		{Header: "ID", Field: "id"},
		{Header: "Name", Field: "name"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format: FormatTable,
		Writer: &buf,
	}
	if err := printTable(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "pro_01") {
		t.Errorf("expected output to contain 'pro_01', got: %s", out)
	}
	if !strings.Contains(out, "Pro Plan") {
		t.Errorf("expected output to contain 'Pro Plan', got: %s", out)
	}
}

// TestPrintTable_HidesAllEmptyColumns verifies that columns where every
// value is empty/null/missing/empty-container are dropped before
// rendering. Covers "" / null / [] / {} cell renderings. Columns with
// even a single non-empty value must remain.
func TestPrintTable_HidesAllEmptyColumns(t *testing.T) {
	input := []byte(`{"data":[
		{"id":"ctm_1","email":"a@x","website":"","custom_data":null,"tags":[],"meta":{}},
		{"id":"ctm_2","email":"b@x","website":"","custom_data":null,"tags":[],"meta":{}},
		{"id":"ctm_3","email":"c@x","website":"","custom_data":null,"tags":[],"meta":{}}
	]}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatTable, Writer: &buf}
	if err := Print(input, nil, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "EMAIL") {
		t.Errorf("expected ID and EMAIL columns to remain:\n%s", out)
	}
	for _, hidden := range []string{"WEBSITE", "CUSTOM_DATA", "TAGS", "META"} {
		if strings.Contains(out, hidden) {
			t.Errorf("%s column has only empty values; should have been hidden:\n%s", hidden, out)
		}
	}
}

// TestPrintTable_KeepsBooleanAndZeroColumns guards the deliberate
// decision to treat false / 0 as data, not noise. They render as
// "false" / "0", which carry information.
func TestPrintTable_KeepsBooleanAndZeroColumns(t *testing.T) {
	input := []byte(`{"data":[
		{"id":"ctm_1","active":false,"count":0},
		{"id":"ctm_2","active":false,"count":0}
	]}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatTable, Writer: &buf}
	if err := Print(input, nil, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ACTIVE") {
		t.Errorf("ACTIVE column has all false values; must remain (false carries info):\n%s", out)
	}
	if !strings.Contains(out, "COUNT") {
		t.Errorf("COUNT column has all 0 values; must remain (0 carries info):\n%s", out)
	}
}

// TestDropEmptyColumns_PreservesOrderWhenMiddleDropped verifies that
// dropping a non-edge column doesn't reorder the surviving columns.
// Guards future refactors of dropEmptyColumns that might change from
// the current append-to-kept pattern. Uses the helper directly to
// avoid rendering-dependent string assertions.
func TestDropEmptyColumns_PreservesOrderWhenMiddleDropped(t *testing.T) {
	rows := []map[string]any{
		{"a": "a1", "b": "", "c": "c1"},
		{"a": "a2", "b": "", "c": "c2"},
	}
	cols := []ColumnDef{
		{Header: "A", Field: "a"},
		{Header: "B", Field: "b"},
		{Header: "C", Field: "c"},
	}
	got := dropEmptyColumns(rows, cols)
	if len(got) != 2 || got[0].Header != "A" || got[1].Header != "C" {
		t.Errorf("expected [A,C] after dropping empty middle column B, got %+v", got)
	}
}

// TestPrintTable_KeepsColumnWithAtLeastOneValue verifies that a single
// non-empty cell in a column is enough to keep the whole column.
func TestPrintTable_KeepsColumnWithAtLeastOneValue(t *testing.T) {
	input := []byte(`{"data":[
		{"id":"ctm_1","note":""},
		{"id":"ctm_2","note":"hello"},
		{"id":"ctm_3","note":""}
	]}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatTable, Writer: &buf}
	if err := Print(input, nil, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "NOTE") {
		t.Errorf("NOTE column has one non-empty value; must remain:\n%s", out)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected the non-empty cell to render:\n%s", out)
	}
}

// TestPrintTable_AllColumnsEmptyFallback verifies the degenerate case:
// if every column would be hidden, we keep them all so the user sees
// the headers and an empty body rather than nothing.
func TestPrintTable_AllColumnsEmptyFallback(t *testing.T) {
	input := []byte(`{"data":[{"a":"","b":null},{"a":"","b":null}]}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatTable, Writer: &buf}
	if err := Print(input, nil, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Errorf("expected A and B headers in the all-empty fallback:\n%s", out)
	}
}

// TestPrintCSV_KeepsAllColumnsIncludingEmpty verifies that CSV is *not*
// affected by the table-only empty-column suppression: downstream CSV
// consumers want a stable schema even when some columns are blank for
// a given query.
func TestPrintCSV_KeepsAllColumnsIncludingEmpty(t *testing.T) {
	input := []byte(`{"data":[{"id":"ctm_1","website":""}]}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatCSV, Writer: &buf}
	if err := Print(input, nil, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "WEBSITE") {
		t.Errorf("CSV must keep all columns even when empty; got:\n%s", out)
	}
}
