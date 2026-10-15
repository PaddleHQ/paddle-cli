package commands_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
)

func newConfigTestRoot(t *testing.T) (root *commands.Root, cfgPath string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{}, cfgPath); err != nil {
		t.Fatalf("saving config: %v", err)
	}
	root = commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewConfigCmd(root))
	return root, cfgPath
}

func runConfigCmd(t *testing.T, root *commands.Root, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetErr(&buf)
	root.Cmd.SetArgs(args)
	err := root.Cmd.Execute()
	return buf.String(), err
}

func TestConfigSet_DefaultOutput(t *testing.T) {
	root, cfgPath := newConfigTestRoot(t)
	out, err := runConfigCmd(t, root, "config", "set", "default_output", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Set default_output = json") {
		t.Errorf("unexpected output: %s", out)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.DefaultOutput != "json" {
		t.Errorf("config.DefaultOutput = %q, want json", cfg.DefaultOutput)
	}
}

func TestConfigSet_DefaultOutput_InvalidValue(t *testing.T) {
	root, cfgPath := newConfigTestRoot(t)
	_, err := runConfigCmd(t, root, "config", "set", "default_output", "xml")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("expected validation error, got: %v", err)
	}
	cfg, _ := config.Load(cfgPath)
	if cfg.DefaultOutput != "" {
		t.Errorf("config should not have been persisted; got DefaultOutput = %q", cfg.DefaultOutput)
	}
}

func TestConfigSet_UnknownKey(t *testing.T) {
	root, _ := newConfigTestRoot(t)
	_, err := runConfigCmd(t, root, "config", "set", "nonsense", "x")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("expected unknown key error, got: %v", err)
	}
}

func TestConfigGet_EmptyAndSet(t *testing.T) {
	root, _ := newConfigTestRoot(t)

	out, err := runConfigCmd(t, root, "config", "get", "default_output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty value for unset key, got: %q", out)
	}

	root2, _ := newConfigTestRoot(t)
	if _, err := runConfigCmd(t, root2, "config", "set", "default_output", "csv"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	out, err = runConfigCmd(t, root2, "config", "get", "default_output")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if strings.TrimSpace(out) != "csv" {
		t.Errorf("get default_output = %q, want csv", strings.TrimSpace(out))
	}
}

func TestConfigUnset(t *testing.T) {
	root, cfgPath := newConfigTestRoot(t)
	if _, err := runConfigCmd(t, root, "config", "set", "default_output", "csv"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	root2 := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root2.Cmd.AddCommand(commands.NewConfigCmd(root2))
	if _, err := runConfigCmd(t, root2, "config", "unset", "default_output"); err != nil {
		t.Fatalf("unset failed: %v", err)
	}
	cfg, _ := config.Load(cfgPath)
	if cfg.DefaultOutput != "" {
		t.Errorf("expected default_output cleared, got %q", cfg.DefaultOutput)
	}
}

func TestConfigCmd_APIVersionKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewConfigCmd(root))
	var setOut bytes.Buffer
	root.Cmd.SetOut(&setOut)
	root.Cmd.SetArgs([]string{"config", "set", "api_version", "2025-01-01"})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("config set: %v", err)
	}

	root2 := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root2.Cmd.AddCommand(commands.NewConfigCmd(root2))
	var getOut bytes.Buffer
	root2.Cmd.SetOut(&getOut)
	root2.Cmd.SetArgs([]string{"config", "get", "api_version"})
	if err := root2.Cmd.Execute(); err != nil {
		t.Fatalf("config get: %v", err)
	}
	if got := strings.TrimSpace(getOut.String()); got != "2025-01-01" {
		t.Errorf("expected config get to return %q, got %q", "2025-01-01", got)
	}
}

func TestConfigCmd_ColorKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewConfigCmd(root))
	root.Cmd.SetArgs([]string{"config", "set", "color", "never"})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("config set: %v", err)
	}

	root2 := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root2.Cmd.AddCommand(commands.NewConfigCmd(root2))
	var getOut bytes.Buffer
	root2.Cmd.SetOut(&getOut)
	root2.Cmd.SetArgs([]string{"config", "get", "color"})
	if err := root2.Cmd.Execute(); err != nil {
		t.Fatalf("config get: %v", err)
	}
	if got := strings.TrimSpace(getOut.String()); got != "never" {
		t.Errorf("expected config get to return %q, got %q", "never", got)
	}
}

func TestConfigCmd_ColorKey_InvalidValue(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewConfigCmd(root))
	root.Cmd.SetArgs([]string{"config", "set", "color", "yes"})
	if err := root.Cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid color value, got nil")
	}
}

func TestConfigList(t *testing.T) {
	root, _ := newConfigTestRoot(t)
	if _, err := runConfigCmd(t, root, "config", "set", "default_output", "json"); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	root2 := commands.NewRoot(commands.RootConfig{ConfigPath: root.Config.ConfigPath})
	root2.Cmd.AddCommand(commands.NewConfigCmd(root2))
	out, err := runConfigCmd(t, root2, "config", "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(out, "default_output") || !strings.Contains(out, "json") {
		t.Errorf("expected default_output=json in list output, got: %s", out)
	}
}
