package commands_test

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PaddleHQ/paddle-cli/internal/auth"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
)

func TestLoginCmd_RequestsWriteScope(t *testing.T) {
	var requested []string
	original := commands.LoginFunc
	commands.LoginFunc = func(cfg auth.OAuthConfig) (*auth.Token, error) {
		requested = cfg.Scopes
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

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnv})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !slices.Contains(requested, config.ScopeAPIWrite) {
		t.Errorf("requested scopes %v, want %s included", requested, config.ScopeAPIWrite)
	}
	if slices.Contains(requested, config.ScopeAPIRead) {
		t.Errorf("requested scopes %v, want %s excluded", requested, config.ScopeAPIRead)
	}

	// The stubbed server omits scope, so the requested set is stored.
	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got, want := loaded.Environments[configtest.TestEnv].Scope, strings.Join(requested, " "); got != want {
		t.Errorf("stored scope = %q, want %q", got, want)
	}
}

func TestLoginCmd_PersistsServerGrantedScope(t *testing.T) {
	granted := "openid offline_access api_read"
	original := commands.LoginFunc
	commands.LoginFunc = func(_ auth.OAuthConfig) (*auth.Token, error) {
		return &auth.Token{AccessToken: "tok_test", RefreshToken: "ref_test", Scope: granted}, nil
	}
	defer func() { commands.LoginFunc = original }()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
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
	if got := loaded.Environments[configtest.TestEnv].Scope; got != granted {
		t.Errorf("stored scope = %q, want %q", got, granted)
	}
}

// A refresh response typically omits scope (RFC 6749: unchanged), so the
// originally granted scope must survive the token rotation.
func TestRootSetup_RefreshPreservesScope(t *testing.T) {
	originalRefresh := commands.RefreshFunc
	commands.RefreshFunc = func(_ context.Context, _ auth.OAuthConfig, _ string) (*auth.Token, error) {
		return &auth.Token{
			AccessToken:  "at_new",
			RefreshToken: "rt_new",
			ExpiresAt:    time.Now().Add(time.Hour),
		}, nil
	}
	defer func() { commands.RefreshFunc = originalRefresh }()

	granted := "openid offline_access api_write"
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnv,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnv: {
				AccessToken:  "at_old",
				RefreshToken: "rt_old",
				ExpiresAt:    time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
				Scope:        granted,
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

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	envCfg := loaded.Environments[configtest.TestEnv]
	if envCfg.AccessToken != "at_new" {
		t.Errorf("AccessToken = %q, want at_new", envCfg.AccessToken)
	}
	if envCfg.Scope != granted {
		t.Errorf("Scope = %q, want %q preserved across refresh", envCfg.Scope, granted)
	}
}
