package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
	"github.com/PaddleHQ/paddle-cli/internal/style"
)

func TestPrintJSON_PrettyPrints(t *testing.T) {
	input := []byte(`{"data":[{"id":"1","name":"test"}]}`)
	var buf bytes.Buffer
	opts := Options{
		Format: FormatJSON,
		Writer: &buf,
	}
	if err := printJSON(input, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\n") {
		t.Errorf("expected pretty-printed JSON with newlines, got: %s", out)
	}
	if !strings.Contains(out, `"id"`) {
		t.Errorf("expected output to contain 'id', got: %s", out)
	}
}

func TestPrintJSON_JQFilter(t *testing.T) {
	input := []byte(`{"data":[{"id":"aaa"},{"id":"bbb"}]}`)
	var buf bytes.Buffer
	opts := Options{
		Format: FormatJSON,
		JQ:     ".data[].id",
		Writer: &buf,
	}
	if err := printJSON(input, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "aaa") {
		t.Errorf("expected output to contain 'aaa', got: %s", out)
	}
	if !strings.Contains(out, "bbb") {
		t.Errorf("expected output to contain 'bbb', got: %s", out)
	}
}

func TestPrintJSON_CompactForAgent(t *testing.T) {
	input := []byte(`{"data":[{"id":"1","name":"test"}]}`)
	var buf bytes.Buffer
	opts := AgentOptions()
	opts.Writer = &buf
	if err := printJSON(input, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	// Compact output has exactly one newline (encoder's trailing).
	if n := strings.Count(out, "\n"); n != 1 {
		t.Errorf("expected 1 newline in compact output, got %d: %s", n, out)
	}
	if strings.Contains(out, "  ") {
		t.Errorf("expected no indentation in compact output, got: %s", out)
	}
}

func TestPrintJSON_AppliesColourWhenEnabled(t *testing.T) {
	input := []byte(`{"data":[{"id":"pro_01","amount":100}]}`)
	var buf bytes.Buffer
	opts := Options{
		Format:  FormatJSON,
		Profile: style.Enabled(),
		Writer:  &buf,
	}
	if err := printJSON(input, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.ContainsRune(out, '\x1b') {
		t.Errorf("expected ANSI escapes in JSON output:\n%s", out)
	}
}

func TestPrintJSON_AgentModeStaysPlain(t *testing.T) {
	input := []byte(`{"data":[{"id":"pro_01"}]}`)
	var buf bytes.Buffer
	opts := AgentOptions()
	// Agent mode means Compact=true. Even if a profile somehow leaks
	// through, the compact gate must keep output bytes clean.
	opts.Profile = style.Enabled()
	opts.Writer = &buf
	if err := printJSON(input, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.ContainsRune(buf.String(), '\x1b') {
		t.Errorf("agent/compact mode must not emit ANSI:\n%s", buf.String())
	}
}

func TestPrintJSON_InvalidJQ(t *testing.T) {
	input := []byte(`{"data":[]}`)
	var buf bytes.Buffer
	opts := Options{
		Format: FormatJSON,
		JQ:     "this is not valid jq !!!",
		Writer: &buf,
	}
	err := printJSON(input, opts)
	if err == nil {
		t.Error("expected error for invalid jq expression, got nil")
	}
}

func TestPrintJSON_InjectsMetaEnvironment_WhenObject(t *testing.T) {
	body := []byte(`{"data":[{"id":"ctm_01"}],"meta":{"request_id":"req_01"}}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatJSON, Writer: &buf, Environment: configtest.TestEnvAPIKeyOnly}

	if err := Print(body, nil, opts); err != nil {
		t.Fatalf("Print: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, buf.String())
	}
	meta, ok := got["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta missing or not an object: %v", got["meta"])
	}
	if meta["environment"] != configtest.TestEnvAPIKeyOnly {
		t.Errorf("meta.environment = %v, want test_env_apikey_only", meta["environment"])
	}
	if meta["request_id"] != "req_01" {
		t.Errorf("meta.request_id should be preserved, got %v", meta["request_id"])
	}
}

func TestPrintJSON_CreatesMeta_WhenAbsent(t *testing.T) {
	body := []byte(`{"data":{"id":"ctm_01"}}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatJSON, Writer: &buf, Environment: configtest.TestEnv}

	if err := Print(body, nil, opts); err != nil {
		t.Fatalf("Print: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, buf.String())
	}
	meta, ok := got["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta missing or not an object: %v", got["meta"])
	}
	if meta["environment"] != configtest.TestEnv {
		t.Errorf("meta.environment = %v, want test_env", meta["environment"])
	}
}

func TestPrintJSON_OverridesExistingEnvironment(t *testing.T) {
	body := []byte(`{"data":{},"meta":{"environment":"stale"}}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatJSON, Writer: &buf, Environment: configtest.TestEnvAPIKeyOnly}

	if err := Print(body, nil, opts); err != nil {
		t.Fatalf("Print: %v", err)
	}

	if !strings.Contains(buf.String(), `"environment": "test_env_apikey_only"`) &&
		!strings.Contains(buf.String(), `"environment":"test_env_apikey_only"`) {
		t.Errorf("expected test_env_apikey_only to win, got: %s", buf.String())
	}
}

func TestPrintJSON_LeavesArrayBodiesAlone(t *testing.T) {
	body := []byte(`[1,2,3]`)
	var buf bytes.Buffer
	opts := Options{Format: FormatJSON, Writer: &buf, Environment: configtest.TestEnvAPIKeyOnly}

	if err := Print(body, nil, opts); err != nil {
		t.Fatalf("Print: %v", err)
	}

	if strings.Contains(buf.String(), "environment") {
		t.Errorf("did not expect environment injection on bare array, got: %s", buf.String())
	}
}

func TestPrintJSON_SkipsInjectionWhenEnvironmentEmpty(t *testing.T) {
	// Defensive: if Options.Environment is empty (e.g. PreRun didn't run),
	// don't synthesize an empty meta block on bodies that lack one.
	body := []byte(`{"data":{"id":"ctm_01"}}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatJSON, Writer: &buf, Environment: ""}

	if err := Print(body, nil, opts); err != nil {
		t.Fatalf("Print: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, buf.String())
	}
	if _, ok := got["meta"]; ok {
		t.Errorf("did not expect meta to be created when Environment is empty: %s", buf.String())
	}
}

func TestPrintJSON_InjectionVisibleToJQ(t *testing.T) {
	body := []byte(`{"data":{}}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatJSON, Writer: &buf, Environment: configtest.TestEnvAPIKeyOnly, JQ: ".meta.environment"}

	if err := Print(body, nil, opts); err != nil {
		t.Fatalf("Print: %v", err)
	}

	if strings.TrimSpace(buf.String()) != configtest.TestEnvAPIKeyOnly {
		t.Errorf("expected --jq '.meta.environment' to print 'test_env_apikey_only', got: %q", buf.String())
	}
}

func TestPrintJSON_InjectsNextCommand(t *testing.T) {
	data := []byte(`{"data":[],"meta":{"pagination":{"has_more":true,"next":"ctm_456"}}}`)
	var buf bytes.Buffer
	opts := Options{
		Format:      FormatJSON,
		NextCommand: "paddle customers list --after ctm_456",
		Writer:      &buf,
		Compact:     true,
	}
	if err := printJSON(data, opts); err != nil {
		t.Fatalf("printJSON: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"next_command"`) {
		t.Errorf("expected next_command in JSON output, got %q", out)
	}
	if !strings.Contains(out, "paddle customers list --after ctm_456") {
		t.Errorf("expected next command string in JSON output, got %q", out)
	}
}

func TestPrintJSON_NoNextCommandWhenEmpty(t *testing.T) {
	data := []byte(`{"data":[],"meta":{"pagination":{"has_more":false}}}`)
	var buf bytes.Buffer
	opts := Options{Format: FormatJSON, NextCommand: "", Writer: &buf, Compact: true}
	if err := printJSON(data, opts); err != nil {
		t.Fatalf("printJSON: %v", err)
	}
	if strings.Contains(buf.String(), "next_command") {
		t.Errorf("expected no next_command when empty, got %q", buf.String())
	}
}

// A null meta or pagination used to panic: unmarshalling null into a map
// leaves it nil.
func TestInjectMeta_NullObjects(t *testing.T) {
	cases := []struct {
		name string
		got  []byte
		want string
	}{
		{
			"environment, null meta", injectMetaEnvironment([]byte(`{"data":[],"meta":null}`), "sandbox"),
			`{"data":[],"meta":{"environment":"sandbox"}}`,
		},
		{
			"next command, null meta", injectNextCommand([]byte(`{"data":[],"meta":null}`), "paddle x --after a"),
			`{"data":[],"meta":{"pagination":{"next_command":"paddle x --after a"}}}`,
		},
		{
			"next command, null pagination", injectNextCommand([]byte(`{"data":[],"meta":{"pagination":null}}`), "paddle x --after a"),
			`{"data":[],"meta":{"pagination":{"next_command":"paddle x --after a"}}}`,
		},
	}
	for _, tc := range cases {
		if string(tc.got) != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, tc.got, tc.want)
		}
	}
}
