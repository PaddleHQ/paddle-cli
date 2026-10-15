package commands_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/PaddleHQ/paddle-cli/internal/auth"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
)

func startWhoamiStub(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":{"auth_user_id":"1"}}`)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestRootSetup_RefreshesExpiredToken(t *testing.T) {
	var called bool
	var seenRefreshToken string
	originalRefresh := commands.RefreshFunc
	commands.RefreshFunc = func(_ context.Context, _ auth.OAuthConfig, refreshToken string) (*auth.Token, error) {
		called = true
		seenRefreshToken = refreshToken
		return &auth.Token{
			AccessToken:  "at_new",
			RefreshToken: "rt_new",
			ExpiresAt:    time.Now().Add(time.Hour),
		}, nil
	}
	defer func() { commands.RefreshFunc = originalRefresh }()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnv,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnv: {
				AccessToken:  "at_old",
				RefreshToken: "rt_old",
				ExpiresAt:    time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
			},
		},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewWhoamiCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"whoami"})
	t.Setenv("PADDLE_API_URL", startWhoamiStub(t))

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !called {
		t.Fatal("expected refresh function to be called")
	}
	if seenRefreshToken != "rt_old" {
		t.Errorf("refresh called with %q, want rt_old", seenRefreshToken)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	envCfg := loaded.Environments[configtest.TestEnv]
	if envCfg.AccessToken != "at_new" {
		t.Errorf("AccessToken = %q, want at_new", envCfg.AccessToken)
	}
	if envCfg.RefreshToken != "rt_new" {
		t.Errorf("RefreshToken = %q, want rt_new", envCfg.RefreshToken)
	}
}

func TestRootSetup_SkipsRefreshForLogin(t *testing.T) {
	originalRefresh := commands.RefreshFunc
	commands.RefreshFunc = func(_ context.Context, _ auth.OAuthConfig, _ string) (*auth.Token, error) {
		t.Fatal("refresh should not be called for login command")
		return nil, nil
	}
	defer func() { commands.RefreshFunc = originalRefresh }()

	originalLogin := commands.LoginFunc
	commands.LoginFunc = func(_ auth.OAuthConfig) (*auth.Token, error) {
		return &auth.Token{AccessToken: "at_login"}, nil
	}
	defer func() { commands.LoginFunc = originalLogin }()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnv,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnv: {
				AccessToken:  "at_old",
				RefreshToken: "rt_old",
				ExpiresAt:    time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
			},
		},
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
}

func TestRootSetup_NoRefreshWhenTokenValid(t *testing.T) {
	originalRefresh := commands.RefreshFunc
	commands.RefreshFunc = func(_ context.Context, _ auth.OAuthConfig, _ string) (*auth.Token, error) {
		t.Fatal("refresh should not be called for a still-valid token")
		return nil, nil
	}
	defer func() { commands.RefreshFunc = originalRefresh }()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnvAPIKeyOnly: {
				AccessToken:  "at_valid",
				RefreshToken: "rt_valid",
				ExpiresAt:    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			},
		},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewWhoamiCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"whoami"})
	t.Setenv("PADDLE_API_URL", startWhoamiStub(t))

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
