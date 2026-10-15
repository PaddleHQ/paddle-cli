package commands_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/output"
	"github.com/PaddleHQ/paddle-cli/internal/style"
)

func TestWriteAPIError_JSONFormat(t *testing.T) {
	apiErr := &client.APIError{
		Status: 404,
		Code:   "entity_not_found",
		Detail: "Resource not found",
		Type:   "request_error",
	}

	var buf bytes.Buffer
	commands.WriteAPIError(&buf, apiErr, output.Options{Format: output.FormatJSON})

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v (got: %s)", err, buf.String())
	}
	if parsed["code"] != "entity_not_found" {
		t.Errorf("code = %v, want entity_not_found", parsed["code"])
	}
	if parsed["status"].(float64) != 404 {
		t.Errorf("status = %v, want 404", parsed["status"])
	}
	// Output should be indented (two-space) to match success JSON output.
	if !bytes.Contains(buf.Bytes(), []byte("\n  \"")) {
		t.Errorf("expected indented JSON, got: %s", buf.String())
	}
}

func TestWriteAPIError_PlainFormat(t *testing.T) {
	apiErr := &client.APIError{
		Status: 404,
		Code:   "entity_not_found",
		Detail: "Resource not found",
	}

	for _, format := range []output.Format{output.FormatTable, output.FormatCSV, ""} {
		var buf bytes.Buffer
		commands.WriteAPIError(&buf, apiErr, output.Options{Format: format})
		if !bytes.Contains(buf.Bytes(), []byte("entity_not_found: Resource not found")) {
			t.Errorf("format %q: expected plain text, got: %s", format, buf.String())
		}
		if bytes.Contains(buf.Bytes(), []byte(`"status"`)) {
			t.Errorf("format %q: unexpected JSON in plain output: %s", format, buf.String())
		}
	}
}

func TestWriteAPIError_PlainFormat_ShowsFieldErrors(t *testing.T) {
	apiErr := &client.APIError{
		Status: 400,
		Code:   "bad_request",
		Detail: "Invalid request.",
		Errors: []client.FieldError{
			{Field: "domain", Message: "invalid format"},
			{Field: "type", Message: "must be one of: proxy, cdn"},
		},
	}

	var buf bytes.Buffer
	commands.WriteAPIError(&buf, apiErr, output.Options{Format: output.FormatTable})

	out := buf.String()
	for _, want := range []string{
		"bad_request: Invalid request. (status 400)",
		"  - domain: invalid format",
		"  - type: must be one of: proxy, cdn",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestWriteAPIError_PlainFormat_ShowsDocumentationURL(t *testing.T) {
	apiErr := &client.APIError{
		Status:           400,
		Code:             "bad_request",
		Detail:           "Invalid request.",
		DocumentationURL: "https://developer.paddle.com/errors/bad_request",
	}

	var buf bytes.Buffer
	commands.WriteAPIError(&buf, apiErr, output.Options{Format: output.FormatTable})

	if !bytes.Contains(buf.Bytes(), []byte("https://developer.paddle.com/errors/bad_request")) {
		t.Errorf("expected documentation URL in output:\n%s", buf.String())
	}
}

func TestWriteAPIError_PlainFormat_AppliesColourWhenEnabled(t *testing.T) {
	apiErr := &client.APIError{
		Status:           400,
		Code:             "bad_request",
		Detail:           "Invalid request.",
		DocumentationURL: "https://developer.paddle.com/errors/bad_request",
		Errors: []client.FieldError{
			{Field: "domain", Message: "invalid format"},
		},
	}
	var buf bytes.Buffer
	commands.WriteAPIError(&buf, apiErr, output.Options{
		Format:  output.FormatTable,
		Profile: style.Enabled(),
	})
	out := buf.String()
	// bright-red+bold for top line (lipgloss emits the SGR pair as
	// "\x1b[1;91m"), yellow for the field bullet, cyan for the doc URL.
	for _, seq := range []string{"\x1b[1;91m", "\x1b[33m", "\x1b[36m"} {
		if !bytes.Contains(buf.Bytes(), []byte(seq)) {
			t.Errorf("expected ANSI %q in output:\n%s", seq, out)
		}
	}
}

func TestWriteAPIError_AgentModeIsCompact(t *testing.T) {
	apiErr := &client.APIError{
		Status: 404,
		Code:   "entity_not_found",
		Detail: "Resource not found",
	}

	var buf bytes.Buffer
	commands.WriteAPIError(&buf, apiErr, output.AgentOptions())

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v (got: %s)", err, buf.String())
	}
	// Compact JSON has exactly one newline (the trailing one from Encode).
	if n := bytes.Count(buf.Bytes(), []byte("\n")); n != 1 {
		t.Errorf("expected 1 newline (compact), got %d: %s", n, buf.String())
	}
	if bytes.Contains(buf.Bytes(), []byte("  ")) {
		t.Errorf("expected no indentation, got: %s", buf.String())
	}
}

func TestWriteAPIError_PlainFormat_ShowsRequestID(t *testing.T) {
	apiErr := &client.APIError{
		Status:    404,
		Code:      "not_found",
		Detail:    "Resource not found",
		RequestID: "req_01habc",
	}

	var buf bytes.Buffer
	commands.WriteAPIError(&buf, apiErr, output.Options{Format: output.FormatTable})

	if !bytes.Contains(buf.Bytes(), []byte("Request ID: req_01habc")) {
		t.Errorf("expected 'Request ID: req_01habc' in output:\n%s", buf.String())
	}
}

func TestWriteAPIError_PlainFormat_OmitsEmptyRequestID(t *testing.T) {
	apiErr := &client.APIError{
		Status: 404,
		Code:   "not_found",
		Detail: "Resource not found",
	}

	var buf bytes.Buffer
	commands.WriteAPIError(&buf, apiErr, output.Options{Format: output.FormatTable})

	if bytes.Contains(buf.Bytes(), []byte("Request ID:")) {
		t.Errorf("expected no 'Request ID:' line when empty, got:\n%s", buf.String())
	}
}

func TestWriteAPIError_PlainFormat_ShowsRetryable(t *testing.T) {
	cases := []struct {
		name      string
		retryable bool
		want      string
	}{
		{"retryable true", true, "Retryable: true"},
		{"retryable false", false, "Retryable: false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := &client.APIError{
				Status:    500,
				Code:      "server_error",
				Detail:    "boom",
				Retryable: tc.retryable,
			}
			var buf bytes.Buffer
			commands.WriteAPIError(&buf, apiErr, output.Options{Format: output.FormatTable})
			if !bytes.Contains(buf.Bytes(), []byte(tc.want)) {
				t.Errorf("expected %q in output:\n%s", tc.want, buf.String())
			}
		})
	}
}

func TestWriteAPIError_JSONFormat_IncludesNewFields(t *testing.T) {
	apiErr := &client.APIError{
		Status:    503,
		Code:      "service_unavailable",
		Detail:    "down",
		RequestID: "req_01hxyz",
		Retryable: true,
	}

	var buf bytes.Buffer
	commands.WriteAPIError(&buf, apiErr, output.Options{Format: output.FormatJSON})

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v (got: %s)", err, buf.String())
	}
	if parsed["request_id"] != "req_01hxyz" {
		t.Errorf("request_id = %v, want req_01hxyz", parsed["request_id"])
	}
	if parsed["retryable"] != true {
		t.Errorf("retryable = %v, want true", parsed["retryable"])
	}
}
