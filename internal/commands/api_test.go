package commands_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
	"github.com/PaddleHQ/paddle-cli/internal/output"
)

func TestAPICmd_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/customers" {
			t.Errorf("expected /customers, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"data": []any{},
		}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnvAPIKeyOnly: {AccessToken: "tok_test"},
		},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewAPICmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"api", "/customers", "--output", "json"})

	t.Setenv("PADDLE_API_URL", server.URL)

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("data")) {
		t.Errorf("expected JSON output containing 'data', got: %s", buf.String())
	}
}

func TestAPICmd_Post(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "ctm_01"}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnvAPIKeyOnly: {AccessToken: "tok_test"},
		},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewAPICmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"api", "/customers", "--method", "POST", "--json", `{"email":"a@b.com"}`})
	t.Setenv("PADDLE_API_URL", server.URL)

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["email"] != "a@b.com" {
		t.Errorf("expected email in body, got: %v", receivedBody)
	}
}

func TestAPICmd_ErrorPropagatesAsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"type":   "request_error",
				"code":   "entity_not_found",
				"detail": "Customer not found",
			},
		}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnvAPIKeyOnly: {AccessToken: "tok_test"},
		},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewAPICmd(root))

	root.Cmd.SetArgs([]string{"api", "/customers/ctm_missing"})
	t.Setenv("PADDLE_API_URL", server.URL)

	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "entity_not_found" {
		t.Errorf("code = %q, want entity_not_found", apiErr.Code)
	}
	if apiErr.Status != 404 {
		t.Errorf("status = %d, want 404", apiErr.Status)
	}

	var buf bytes.Buffer
	commands.WriteAPIError(&buf, apiErr, output.Options{Format: output.FormatJSON})
	var parsed map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &parsed); jsonErr != nil {
		t.Errorf("JSON-formatted error is not valid JSON: %v (got %s)", jsonErr, buf.String())
	}
}

func TestAPICmd_EmbeddedQueryString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/customers" {
			t.Errorf("expected path=/customers, got %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("status"); got != "active" {
			t.Errorf("expected status=active, got %q (full query=%v)", got, r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": []any{}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok_test"}},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewAPICmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"api", "/customers?status=active", "--output", "json"})
	t.Setenv("PADDLE_API_URL", server.URL)

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPICmd_EmbeddedQueryMergesWithFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("status") != "active" {
			t.Errorf("expected status=active (from path), got %q", q.Get("status"))
		}
		if q.Get("per_page") != "5" {
			t.Errorf("expected per_page=5 (from --query), got %q", q.Get("per_page"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": []any{}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok_test"}},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewAPICmd(root))

	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetArgs([]string{"api", "/customers?status=active", "--query", "per_page=5", "--output", "json"})
	t.Setenv("PADDLE_API_URL", server.URL)

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPICmd_FlagOverridesEmbeddedQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("status"); got != "archived" {
			t.Errorf("expected --query to win: status=archived, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": []any{}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok_test"}},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewAPICmd(root))

	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetArgs([]string{"api", "/customers?status=active", "--query", "status=archived", "--output", "json"})
	t.Setenv("PADDLE_API_URL", server.URL)

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPICmd_RejectsFullURL(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok_test"}},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewAPICmd(root))

	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})
	root.Cmd.SetArgs([]string{"api", "https://evil.example.com/customers"})

	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected error for full URL, got nil")
	}
	if !strings.Contains(err.Error(), "must be a path") {
		t.Errorf("expected 'must be a path' in error, got %q", err.Error())
	}
}

func TestAPICmd_RejectsEmptyPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok_test"}},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewAPICmd(root))

	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})
	root.Cmd.SetArgs([]string{"api", "?status=active"})

	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected error for path with no path component, got nil")
	}
	if !strings.Contains(err.Error(), "missing path component") {
		t.Errorf("expected 'missing path component' in error, got %q", err.Error())
	}
}
