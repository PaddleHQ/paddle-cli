package commands_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/auth"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
)

// Each of the hand-crafted leaf commands takes no positional arguments. This
// suite verifies that passing an extra arg surfaces ErrUsage so main() exits
// 2. Without it the command would silently ignore the extra arg and run
// (e.g. `paddle logout extra` clearing creds was the original footgun).
func TestLeafCommandsRejectExtraArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		add  func(root *commands.Root)
	}{
		{
			name: "whoami",
			args: []string{"whoami", "extra"},
			add:  func(r *commands.Root) { r.Cmd.AddCommand(commands.NewWhoamiCmd(r)) },
		},
		{
			name: "version",
			args: []string{"version", "extra"},
			add:  func(r *commands.Root) { r.Cmd.AddCommand(commands.NewVersionCmd(r)) },
		},
		{
			name: "environment",
			args: []string{"environment", "extra"},
			add:  func(r *commands.Root) { r.Cmd.AddCommand(commands.NewEnvironmentCmd(r)) },
		},
		{
			name: "login",
			args: []string{"login", "extra"},
			add: func(r *commands.Root) {
				r.Cmd.AddCommand(commands.NewLoginCmd(r))
				commands.LoginFunc = func(_ auth.OAuthConfig) (*auth.Token, error) {
					t.Fatal("login RunE should not be reached when arg validation rejects extra args")
					return nil, nil
				}
			},
		},
		{
			name: "logout",
			args: []string{"logout", "extra"},
			add:  func(r *commands.Root) { r.Cmd.AddCommand(commands.NewLogoutCmd(r)) },
		},
		{
			name: "env-shortcut",
			args: []string{configtest.TestEnvAPIKeyOnly, "extra"},
			add: func(r *commands.Root) {
				r.Cmd.AddCommand(commands.NewEnvShortcutCmd(r, configtest.TestEnvAPIKeyOnly, "Switch to test_env_apikey_only"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			if err := config.Save(&config.Config{
				CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
				Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok"}},
			}, cfgPath); err != nil {
				t.Fatal(err)
			}

			root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
			tt.add(root)
			commands.EnforceArgsExitCode(root.Cmd)

			var stdout, stderr bytes.Buffer
			root.Cmd.SetOut(&stdout)
			root.Cmd.SetErr(&stderr)
			root.Cmd.SetArgs(tt.args)

			err := root.Cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for `paddle %v`, got nil (state may have been mutated)", tt.args)
			}
			if !errors.Is(err, commands.ErrUsage) {
				t.Errorf("expected ErrUsage, got %v", err)
			}
		})
	}
}
