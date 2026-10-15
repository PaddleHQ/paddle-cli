package commands_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
)

func TestOutputCmd_Sets(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{}, cfgPath); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewOutputCmd(root))

	out, err := runConfigCmd(t, root, "output", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Default output set to json") {
		t.Errorf("unexpected output: %s", out)
	}
	cfg, _ := config.Load(cfgPath)
	if cfg.DefaultOutput != "json" {
		t.Errorf("DefaultOutput = %q, want json", cfg.DefaultOutput)
	}
}

func TestOutputCmd_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{}, cfgPath); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewOutputCmd(root))

	_, err := runConfigCmd(t, root, "output", "xml")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	cfg, _ := config.Load(cfgPath)
	if cfg.DefaultOutput != "" {
		t.Errorf("config should not have been written; got %q", cfg.DefaultOutput)
	}
}
