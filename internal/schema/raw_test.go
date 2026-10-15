package schema_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/schema"
)

func TestPrintRaw_Compact(t *testing.T) {
	input := `{"type":"object","properties":{"email":{"type":"string"}}}`
	var buf bytes.Buffer
	if err := schema.PrintRaw(&buf, input, true); err != nil {
		t.Fatalf("PrintRaw error: %v", err)
	}
	got := buf.String()
	if !strings.HasSuffix(got, "\n") {
		t.Error("expected trailing newline")
	}
	want := `{"type":"object","properties":{"email":{"type":"string"}}}` + "\n"
	if got != want {
		t.Errorf("compact output mismatch\nwant: %q\ngot:  %q", want, got)
	}
}

func TestPrintRaw_InvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	err := schema.PrintRaw(&buf, "{not json", false)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing schema JSON") {
		t.Errorf("expected error to mention parsing schema JSON, got: %v", err)
	}
}

func TestPrintRaw_Pretty(t *testing.T) {
	input := `{"type":"object","properties":{"email":{"type":"string"}}}`
	var buf bytes.Buffer
	if err := schema.PrintRaw(&buf, input, false); err != nil {
		t.Fatalf("PrintRaw error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "\n  \"type\": \"object\"") {
		t.Errorf("expected 2-space indented output, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("expected trailing newline")
	}
}
