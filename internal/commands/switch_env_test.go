package commands_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
)

func TestSwitchCmd(t *testing.T) {
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
	root.Cmd.AddCommand(commands.NewSwitchCmd(root))

	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"switch", configtest.TestEnv})
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
	if root.Environment != configtest.TestEnv {
		t.Errorf("expected root.Environment to be updated to test_env, got %q", root.Environment)
	}
	if !root.BannerSuppressed {
		t.Error("expected banner to be suppressed after switch")
	}

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Switched to test_env") {
		t.Errorf("expected stderr to contain 'Switched to test_env', got %q", stderr.String())
	}
}

func TestEnvShortcutCmd_StatusOnStderr(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewEnvShortcutCmd(root, configtest.TestEnvAPIKeyOnly, "Switch to test_env_apikey_only"))

	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{configtest.TestEnvAPIKeyOnly})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if root.Environment != configtest.TestEnvAPIKeyOnly {
		t.Errorf("expected root.Environment to be updated to test_env_apikey_only, got %q", root.Environment)
	}
	if !root.BannerSuppressed {
		t.Error("expected banner to be suppressed after switch")
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Switched to test_env_apikey_only") {
		t.Errorf("expected stderr to contain 'Switched to test_env_apikey_only', got %q", stderr.String())
	}
}

func TestSwitchCmd_SuccessMessageIsColouredWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("current_environment: test_env_apikey_only\nenvironments: {}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("NO_COLOR", "")

	r := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	r.Cmd.AddCommand(commands.NewSwitchCmd(r))
	r.Cmd.SetArgs([]string{"switch", configtest.TestEnv})
	var stderr bytes.Buffer
	r.Cmd.SetOut(io.Discard)
	r.Cmd.SetErr(&stderr)
	if err := r.Cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := stderr.String()
	// "Switched to" in green (32m), environment name in bright-red (91m for test_env).
	if !strings.Contains(out, "\x1b[32m") {
		t.Errorf("expected green ANSI for 'Switched to', got: %q", out)
	}
	if !strings.Contains(out, "91m") {
		t.Errorf("expected bright-red color code for env name, got: %q", out)
	}
	if !strings.Contains(out, "Switched to") || !strings.Contains(out, configtest.TestEnv) {
		t.Errorf("expected 'Switched to' and 'test_env' in output, got: %q", out)
	}
}

func TestRegisterEnvShortcuts_OnePerEnvironment(t *testing.T) {
	root := commands.NewRoot(commands.RootConfig{})
	commands.RegisterEnvShortcuts(root)

	for _, env := range []string{configtest.TestEnv, configtest.TestEnvAPIKeyOnly} {
		cmd, _, err := root.Cmd.Find([]string{env})
		if err != nil || cmd.Name() != env {
			t.Errorf("expected a %q shortcut command, got %v (err %v)", env, cmd, err)
		}
	}
}
