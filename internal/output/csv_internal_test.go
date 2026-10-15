package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintCSV_BasicOutput(t *testing.T) {
	input := []byte(`{"data":[{"id":"pro_01","name":"Pro Plan"},{"id":"pro_02","name":"Starter Plan"}]}`)
	cols := []ColumnDef{
		{Header: "ID", Field: "id"},
		{Header: "Name", Field: "name"},
	}
	var buf bytes.Buffer
	opts := Options{
		Format: FormatCSV,
		Writer: &buf,
	}
	if err := printCSV(input, cols, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines (header + 2 data rows), got %d: %s", len(lines), out)
	}
	if lines[0] != "ID,Name" {
		t.Errorf("expected header row 'ID,Name', got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "pro_01") {
		t.Errorf("expected first data row to contain 'pro_01', got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "pro_02") {
		t.Errorf("expected second data row to contain 'pro_02', got: %s", lines[2])
	}
}
