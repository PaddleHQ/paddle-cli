package commands_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/output"
)

func newVersionTestRoot(t *testing.T) *commands.Root {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{}, cfgPath); err != nil {
		t.Fatalf("saving config: %v", err)
	}
	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewVersionCmd(root))
	return root
}

func TestVersionCmd_JSON(t *testing.T) {
	root := newVersionTestRoot(t)
	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"--agent", "version"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{`"version"`, `"commit"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("expected %q in JSON output, got: %s", want, buf.String())
		}
	}
}

func TestVersionCmd_Table(t *testing.T) {
	ttyOriginal := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStdoutTTY = ttyOriginal })

	root := newVersionTestRoot(t)
	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"version"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"VERSION", "COMMIT"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("expected %q in table output, got: %s", want, buf.String())
		}
	}
}
