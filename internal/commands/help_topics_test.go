package commands_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"

	"github.com/spf13/cobra"
)

func TestHelpTopics_KnownTopicPrintsContent(t *testing.T) {
	topics := []string{"environment", "exit-codes", "formatting", "api-versions"}
	for _, topic := range topics {
		t.Run(topic, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
				t.Fatal(err)
			}

			root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
			commands.InstallHelpTopics(root)

			var buf bytes.Buffer
			root.Cmd.SetOut(&buf)
			root.Cmd.SetArgs([]string{"help", topic})
			if err := root.Cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			out := buf.String()
			if len(out) < 50 {
				t.Errorf("topic %q: expected substantial output, got %q", topic, out)
			}
		})
	}
}

func TestHelpTopics_UnknownTopicFallsThroughToCommand(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	commands.InstallHelpTopics(root)
	root.Cmd.AddCommand(commands.NewVersionCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"help", "version"})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "version") {
		t.Errorf("expected version command help, got %q", out)
	}
}

func TestHelpTopics_UnknownTopicIsUsageError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	commands.InstallHelpTopics(root)

	var stdout bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetArgs([]string{"help", "bogus"})
	err := root.Cmd.Execute()
	if !errors.Is(err, commands.ErrUsage) {
		t.Fatalf("Execute error = %v, want one wrapping ErrUsage", err)
	}
	if !strings.Contains(err.Error(), `"bogus"`) {
		t.Errorf("error %q doesn't name the unknown topic", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no help on stdout, got %q", stdout.String())
	}
}

func TestHelpTopics_UnknownSubcommandIsUsageError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	parent := &cobra.Command{Use: "customers"}
	parent.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	root.Cmd.AddCommand(parent)
	commands.InstallHelpTopics(root)

	var stdout bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetArgs([]string{"help", "customers", "bogus"})
	err := root.Cmd.Execute()
	if !errors.Is(err, commands.ErrUsage) {
		t.Fatalf("Execute error = %v, want one wrapping ErrUsage", err)
	}
	if !strings.Contains(err.Error(), `"customers bogus"`) {
		t.Errorf("error %q doesn't name the unknown command", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no help on stdout, got %q", stdout.String())
	}
}

func TestHelpTopics_IgnoresGlobalFlags(t *testing.T) {
	cases := map[string]struct {
		args []string
		want string
	}{
		"topic":                {[]string{"--no-banner", "help", "exit-codes"}, "Exit Codes"},
		"no topic":             {[]string{"--agent", "help"}, "interact with the Paddle API"},
		"separate flag value":  {[]string{"--environment", "sandbox", "help", "exit-codes"}, "Exit Codes"},
		"shorthand flag value": {[]string{"-o", "json", "help", "exit-codes"}, "Exit Codes"},
		"attached flag value":  {[]string{"--output=json", "help", "exit-codes"}, "Exit Codes"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
				t.Fatal(err)
			}

			root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
			commands.InstallHelpTopics(root)

			var stdout bytes.Buffer
			root.Cmd.SetOut(&stdout)
			root.Cmd.SetArgs(tc.args)
			if err := root.Cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Errorf("expected %q in output, got %q", tc.want, stdout.String())
			}
		})
	}
}

func TestHelpTopics_NoArgs_ShowsRootHelp(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	commands.InstallHelpTopics(root)

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"help"})
	_ = root.Cmd.Execute()
	out := buf.String()
	if !strings.Contains(strings.ToLower(out), "paddle") {
		t.Errorf("expected root help to contain 'paddle', got %q", out)
	}
}
