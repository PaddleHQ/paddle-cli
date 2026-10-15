package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
)

func TestLogoutCmd_StatusOnStderr(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnvAPIKeyOnly: {AccessToken: "tok"},
		},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLogoutCmd(root))

	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"logout"})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, _ := config.Load(cfgPath)
	if _, exists := loaded.Environments[configtest.TestEnvAPIKeyOnly]; exists {
		t.Error("expected test_env_apikey_only env to be cleared")
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Logged out of test_env_apikey_only") {
		t.Errorf("stderr missing 'Logged out of test_env_apikey_only', got %q", stderr.String())
	}
}

func TestLogoutCmd_ErrorsWhenNotLoggedIn(t *testing.T) {
	// No config file: logout must not claim success or create a config file.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLogoutCmd(root))

	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})
	root.Cmd.SetArgs([]string{"logout"})

	err := root.Cmd.Execute()
	if err == nil {
		t.Fatal("expected error logging out with no stored credentials, got nil")
	}
	if !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("error = %q, want it to mention 'not logged in'", err)
	}
	if _, statErr := os.Stat(cfgPath); !os.IsNotExist(statErr) {
		t.Errorf("expected no config file to be created, stat error = %v", statErr)
	}
}

func TestLogoutCmd_ClearsAPIKeyOnlyEntry(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments: map[string]config.EnvironmentConfig{
			configtest.TestEnvAPIKeyOnly: {APIKey: "key_abc"},
		},
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLogoutCmd(root))

	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})
	root.Cmd.SetArgs([]string{"logout"})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, _ := config.Load(cfgPath)
	if _, exists := loaded.Environments[configtest.TestEnvAPIKeyOnly]; exists {
		t.Error("expected test_env_apikey_only env (api-key only) to be cleared")
	}
}
