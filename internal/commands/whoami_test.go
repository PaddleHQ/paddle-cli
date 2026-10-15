package commands_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
	"github.com/PaddleHQ/paddle-cli/internal/output"
)

// whoamiBody is the real /check/whoami response shape (condensed for tests).
const whoamiBody = `{
	"data": {
		"auth_seller_id": "506",
		"auth_user_id": "493",
		"auth_publisher_id": null,
		"auth_customer_ids": null,
		"auth_classic_customer_ids": null,
		"header_seller_id": "506",
		"header_user_id": "493",
		"header_publisher_id": null,
		"header_customer_ids": null,
		"header_classic_customer_ids": null,
		"scopes": ["all.adjustment.read", "all.adjustment.write"]
	},
	"meta": { "request_id": "req_01" }
}`

func newWhoamiServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/check/whoami" {
			t.Errorf("expected /check/whoami, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok_abc" {
			t.Errorf("expected bearer token header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(whoamiBody)); err != nil {
			t.Error(err)
		}
	}))
}

func newWhoamiRoot(t *testing.T, serverURL string) *commands.Root {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnvAPIKeyOnly: {AccessToken: "tok_abc"},
		},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PADDLE_API_URL", serverURL)
	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewWhoamiCmd(root))
	return root
}

// TestWhoamiCmd_TableDefault verifies the curated table view:
// environment, collapsed seller/user IDs, scope count.
func TestWhoamiCmd_TableDefault(t *testing.T) {
	server := newWhoamiServer(t)
	defer server.Close()

	root := newWhoamiRoot(t, server.URL)
	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"whoami", "--output", "table"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()

	// curated fields present
	for _, want := range []string{configtest.TestEnvAPIKeyOnly, "506", "493", "2 scopes"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Errorf("expected %q in table output, got:\n%s", want, got)
		}
	}
	// raw scopes must NOT appear; they're replaced by the count
	if bytes.Contains([]byte(got), []byte("all.adjustment.read")) {
		t.Errorf("raw scope name should not appear in default table output, got:\n%s", got)
	}
}

// TestWhoamiCmd_TableFull verifies that --full expands scopes to individual names.
func TestWhoamiCmd_TableFull(t *testing.T) {
	server := newWhoamiServer(t)
	defer server.Close()

	root := newWhoamiRoot(t, server.URL)
	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"whoami", "--output", "table", "--full"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()

	// scope names must appear
	for _, want := range []string{"all.adjustment.read", "all.adjustment.write"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Errorf("expected scope %q in --full output, got:\n%s", want, got)
		}
	}
	// scope count must NOT appear when --full is set
	if bytes.Contains([]byte(got), []byte("2 scopes")) {
		t.Errorf("scope count should not appear in --full output, got:\n%s", got)
	}
}

// TestWhoamiCmd_JSONPassthrough verifies that JSON format emits the full API
// response (all data fields including scopes) with meta.environment injected.
func TestWhoamiCmd_JSONPassthrough(t *testing.T) {
	server := newWhoamiServer(t)
	defer server.Close()

	root := newWhoamiRoot(t, server.URL)
	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"whoami", "--output", "json"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()

	// full API data fields present
	for _, want := range []string{`"auth_seller_id"`, `"auth_user_id"`, `"all.adjustment.read"`} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Errorf("expected %q in JSON output, got:\n%s", want, got)
		}
	}
	// meta.environment injected by injectMetaEnvironment
	if !bytes.Contains([]byte(got), []byte(`"environment"`)) {
		t.Errorf("expected meta.environment in JSON output, got:\n%s", got)
	}
}

// TestWhoamiCmd_JSONFullIsNoOp verifies that --full has no effect on JSON output.
func TestWhoamiCmd_JSONFullIsNoOp(t *testing.T) {
	server := newWhoamiServer(t)
	defer server.Close()

	rootWithout := newWhoamiRoot(t, server.URL)
	var bufWithout bytes.Buffer
	rootWithout.Cmd.SetOut(&bufWithout)
	rootWithout.Cmd.SetArgs([]string{"whoami", "--output", "json"})
	if err := rootWithout.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	server2 := newWhoamiServer(t)
	defer server2.Close()
	rootWith := newWhoamiRoot(t, server2.URL)
	var bufWith bytes.Buffer
	rootWith.Cmd.SetOut(&bufWith)
	rootWith.Cmd.SetArgs([]string{"whoami", "--output", "json", "--full"})
	if err := rootWith.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Strip trailing newline differences before comparing
	without := bytes.TrimRight(bufWithout.Bytes(), "\n")
	with := bytes.TrimRight(bufWith.Bytes(), "\n")
	if !bytes.Equal(without, with) {
		t.Errorf("--full should have no effect on JSON output\nwithout --full: %s\nwith --full:    %s", without, with)
	}
}

// TestWhoamiCmd_Unauthenticated verifies the error path when no token exists.
func TestWhoamiCmd_Unauthenticated(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {}},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewWhoamiCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetErr(&buf)
	root.Cmd.SetArgs([]string{"whoami"})

	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected error when unauthenticated")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("not authenticated")) {
		t.Errorf("expected 'not authenticated' in error, got: %v", err)
	}
}

// TestWhoamiCmd_APIKeyFlagAuthenticates verifies whoami works when the only
// credential is a --api-key flag, with no stored config at all.
func TestWhoamiCmd_APIKeyFlagAuthenticates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer key_flag" {
			t.Errorf("expected Bearer key_flag, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(whoamiBody)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("PADDLE_API_URL", server.URL)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewWhoamiCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"whoami", "--api-key", "key_flag"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Ensure output package is imported; it's used for format constant reference in tests.
var _ = output.FormatTable
