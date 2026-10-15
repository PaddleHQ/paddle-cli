package commands_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/auth"
	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
)

func TestLoginCmd_SetsEnvironment(t *testing.T) {
	// Stub the login function so it doesn't open a browser.
	original := commands.LoginFunc
	commands.LoginFunc = func(cfg auth.OAuthConfig) (*auth.Token, error) {
		if cfg.ClientID == "" {
			t.Error("expected non-empty ClientID")
		}
		return &auth.Token{
			AccessToken:  "tok_test",
			RefreshToken: "ref_test",
		}, nil
	}
	defer func() { commands.LoginFunc = original }()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		Environments: map[string]config.EnvironmentConfig{},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnv})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.CurrentEnvironment != configtest.TestEnv {
		t.Errorf("expected test_env, got %q", loaded.CurrentEnvironment)
	}
	env, ok := loaded.Environments[configtest.TestEnv]
	if !ok {
		t.Fatal("expected test_env environment in config")
	}
	if env.AccessToken != "tok_test" {
		t.Errorf("expected access_token=tok_test, got %q", env.AccessToken)
	}
	if env.APIKey != "" {
		t.Errorf("expected no api_key stored after OAuth login, got %q", env.APIKey)
	}
}

// TestLoginCmd_RespectsPaddleEnvironmentEnvVar verifies login honors
// PADDLE_ENVIRONMENT (consistent with every other command's resolution)
// instead of falling through to the configtest.TestEnvAPIKeyOnly default.
func TestLoginCmd_RespectsPaddleEnvironmentEnvVar(t *testing.T) {
	original := commands.LoginFunc
	commands.LoginFunc = func(_ auth.OAuthConfig) (*auth.Token, error) {
		return &auth.Token{AccessToken: "tok_test"}, nil
	}
	defer func() { commands.LoginFunc = original }()

	t.Setenv("PADDLE_ENVIRONMENT", configtest.TestEnv)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"login"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.CurrentEnvironment != configtest.TestEnv {
		t.Errorf("expected test_env (from PADDLE_ENVIRONMENT), got %q", loaded.CurrentEnvironment)
	}
}

func TestLoginCmd_InvalidEnvironment(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		Environments: map[string]config.EnvironmentConfig{},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"login", "--environment", "invalid"})

	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid environment")
	}
}

func TestLoginCmd_PassesCorrectOAuthConfig(t *testing.T) {
	var receivedCfg auth.OAuthConfig
	original := commands.LoginFunc
	commands.LoginFunc = func(cfg auth.OAuthConfig) (*auth.Token, error) {
		receivedCfg = cfg
		return &auth.Token{AccessToken: "tok"}, nil
	}
	defer func() { commands.LoginFunc = original }()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		Environments: map[string]config.EnvironmentConfig{},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnv})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := auth.OAuthConfig{
		AuthURL:  "https://auth.example.test/oauth2/authorize",
		TokenURL: "https://auth.example.test/oauth2/token",
		ClientID: "client_test",
	}
	if receivedCfg.ClientID != want.ClientID {
		t.Errorf("ClientID = %q, want %q", receivedCfg.ClientID, want.ClientID)
	}
	if receivedCfg.AuthURL != want.AuthURL {
		t.Errorf("AuthURL = %q, want %q", receivedCfg.AuthURL, want.AuthURL)
	}
	if receivedCfg.TokenURL != want.TokenURL {
		t.Errorf("TokenURL = %q, want %q", receivedCfg.TokenURL, want.TokenURL)
	}
}

func TestLoginCmd_StatusMessagesOnStderr(t *testing.T) {
	original := commands.LoginFunc
	commands.LoginFunc = func(_ auth.OAuthConfig) (*auth.Token, error) {
		return &auth.Token{AccessToken: "tok_test", RefreshToken: "ref_test"}, nil
	}
	defer func() { commands.LoginFunc = original }()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnv})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Logging in to test_env") {
		t.Errorf("stderr missing 'Logging in to test_env', got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Logged in to test_env") {
		t.Errorf("stderr missing 'Logged in to test_env', got %q", stderr.String())
	}
}

func TestLoginCmd_OAuthNotConfiguredForEnvironment(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetErr(&buf)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnvAPIKeyOnly})

	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected error: environment has no OAuth client configured")
	}
	want := `OAuth is not supported for the "test_env_apikey_only" environment; use 'paddle login --api-key <key>' or set PADDLE_API_KEY`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	if code, _ := client.ExitCodeFor(err, commands.ErrUsage); code != client.ExitUsageError {
		t.Errorf("exit code = %d, want %d", code, client.ExitUsageError)
	}
}

func TestLoginCmd_APIKeyValidPersists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/check/whoami" {
			t.Errorf("expected /check/whoami, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key_abc" {
			t.Errorf("expected Bearer key_abc, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":{}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("PADDLE_API_URL", server.URL)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetErr(&buf)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnvAPIKeyOnly, "--api-key", "key_abc"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.CurrentEnvironment != configtest.TestEnvAPIKeyOnly {
		t.Errorf("expected test_env_apikey_only, got %q", loaded.CurrentEnvironment)
	}
	env, ok := loaded.Environments[configtest.TestEnvAPIKeyOnly]
	if !ok {
		t.Fatal("expected test_env_apikey_only environment in config")
	}
	if env.APIKey != "key_abc" {
		t.Errorf("expected api_key=key_abc, got %q", env.APIKey)
	}
	if env.AccessToken != "" {
		t.Errorf("expected no access_token stored for an api-key login, got %q", env.AccessToken)
	}
}

// TestLoginCmd_APIKeyValidationReusesInvocationConfig verifies --header and
// --api-version reach the validation request, not just the credential.
func TestLoginCmd_APIKeyValidationReusesInvocationConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Custom"); got != "value" {
			t.Errorf("expected X-Custom: value, got %q", got)
		}
		if got := r.Header.Get("Paddle-Version"); got != "2" {
			t.Errorf("expected Paddle-Version: 2, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":{}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("PADDLE_API_URL", server.URL)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetErr(&buf)
	root.Cmd.SetArgs([]string{
		"login", "--environment", configtest.TestEnvAPIKeyOnly, "--api-key", "key_abc",
		"--header", "X-Custom: value", "--api-version", "2",
	})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoginCmd_APIKeyInvalidDoesNotPersist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if _, err := w.Write([]byte(`{"error":{"type":"request_error","code":"unauthorized","detail":"invalid key"}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("PADDLE_API_URL", server.URL)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetErr(&buf)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnvAPIKeyOnly, "--api-key", "bad_key"})

	if err := root.Cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid api key")
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if _, ok := loaded.Environments[configtest.TestEnvAPIKeyOnly]; ok {
		t.Error("expected no test_env_apikey_only environment persisted after failed api-key validation")
	}
}

func TestLoginCmd_APIKeyClearsStoredOAuthSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":{}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("PADDLE_API_URL", server.URL)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnvAPIKeyOnly: {AccessToken: "old_tok", RefreshToken: "old_ref", ExpiresAt: "2099-01-01T00:00:00Z"},
		},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetErr(&buf)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnvAPIKeyOnly, "--api-key", "new_key"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	env := loaded.Environments[configtest.TestEnvAPIKeyOnly]
	if env.APIKey != "new_key" {
		t.Errorf("expected api_key=new_key, got %q", env.APIKey)
	}
	if env.AccessToken != "" || env.RefreshToken != "" {
		t.Errorf("expected OAuth fields cleared, got AccessToken=%q RefreshToken=%q", env.AccessToken, env.RefreshToken)
	}
}

func TestLoginCmd_OAuthClearsStoredAPIKey(t *testing.T) {
	original := commands.LoginFunc
	commands.LoginFunc = func(_ auth.OAuthConfig) (*auth.Token, error) {
		return &auth.Token{AccessToken: "new_tok", RefreshToken: "new_ref"}, nil
	}
	defer func() { commands.LoginFunc = original }()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnv: {APIKey: "old_key"},
		},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetErr(&buf)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnv})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	env := loaded.Environments[configtest.TestEnv]
	if env.AccessToken != "new_tok" {
		t.Errorf("expected access_token=new_tok, got %q", env.AccessToken)
	}
	if env.APIKey != "" {
		t.Errorf("expected api_key cleared, got %q", env.APIKey)
	}
}
