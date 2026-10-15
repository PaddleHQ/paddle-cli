package commands_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
	"github.com/PaddleHQ/paddle-cli/internal/output"
)

func TestEnvironmentCmd_JSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{CurrentEnvironment: configtest.TestEnvAPIKeyOnly}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewEnvironmentCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"--agent", "environment"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{`"environment"`, `"test_env_apikey_only"`, `"api_url"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("expected %q in JSON output, got: %s", want, buf.String())
		}
	}
}

func TestEnvironmentCmd_DefaultsBeforeLogin(t *testing.T) {
	// No config file: the command must report the default environment and
	// its API URL, not empty strings.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewEnvironmentCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"--agent", "environment"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defaultURL, err := config.EnvironmentURL(config.DefaultEnvironment)
	if err != nil {
		t.Fatalf("resolving default environment URL: %v", err)
	}
	for _, want := range []string{
		`"environment":"` + config.DefaultEnvironment + `"`,
		`"api_url":"` + defaultURL + `"`,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("expected %q in JSON output, got: %s", want, buf.String())
		}
	}
}

func TestEnvironmentCmd_Table(t *testing.T) {
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStdoutTTY = original })

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{CurrentEnvironment: configtest.TestEnv}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewEnvironmentCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"environment"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"ENVIRONMENT", "API_URL", configtest.TestEnv} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("expected %q in table output, got: %s", want, buf.String())
		}
	}
}
