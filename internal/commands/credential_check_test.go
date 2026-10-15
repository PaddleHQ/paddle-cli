package commands_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"

	"github.com/spf13/cobra"
)

// newRootWithResource builds a root with a "products" resource command whose
// "list" and "create" leaves record whether they ran. storedKey, if set, is
// stored as the login for the current environment.
func newRootWithResource(t *testing.T, storedKey string) (root *commands.Root, ran *bool) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{CurrentEnvironment: configtest.TestEnvAPIKeyOnly}
	if storedKey != "" {
		cfg.Environments = map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {APIKey: storedKey}}
	}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}

	ran = new(bool)
	run := func(*cobra.Command, []string) error { *ran = true; return nil }
	products := &cobra.Command{Use: "products"}
	products.AddCommand(&cobra.Command{Use: "list", RunE: run})
	create := &cobra.Command{Use: "create", RunE: run}
	create.Flags().Bool("schema", false, "")
	products.AddCommand(create)

	root = commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(products, commands.NewAPICmd(root), commands.NewEnvironmentCmd(root))
	commands.AssignDefaultGroup(root.Cmd, commands.GroupIDResources)
	commands.EnforceUnknownSubcommandError(root.Cmd) // as main does: group commands get a RunE
	root.Cmd.SetOut(io.Discard)
	root.Cmd.SetErr(io.Discard)
	return root, ran
}

func TestCredentialCheck_NotLoggedIn(t *testing.T) {
	for name, args := range map[string][]string{
		"resource command": {"products", "list"},
		"api command":      {"api", "/products"},
	} {
		t.Run(name, func(t *testing.T) {
			root, ran := newRootWithResource(t, "")
			root.Cmd.SetArgs(args)

			err := root.Cmd.Execute()
			if !errors.Is(err, client.ErrNotAuthenticated) {
				t.Fatalf("Execute error = %v, want one wrapping ErrNotAuthenticated", err)
			}
			if code, _ := client.ExitCodeFor(err, commands.ErrUsage); code != client.ExitAuthFailure {
				t.Errorf("exit code = %d, want %d", code, client.ExitAuthFailure)
			}
			for _, want := range []string{configtest.TestEnvAPIKeyOnly, "paddle login"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q doesn't mention %q", err, want)
				}
			}
			if *ran {
				t.Error("command ran without a credential")
			}
		})
	}
}

func TestCredentialCheck_RunsWithoutCheck(t *testing.T) {
	for name, args := range map[string][]string{
		"api key flag":   {"products", "list", "--api-key", "pdl_sdbx_apikey_test"},
		"schema only":    {"products", "create", "--schema"},
		"no-API command": {"environment"},
	} {
		t.Run(name, func(t *testing.T) {
			root, ran := newRootWithResource(t, "")
			root.Cmd.SetArgs(args)

			if err := root.Cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if args[0] == "products" && !*ran {
				t.Error("command didn't run")
			}
		})
	}
}

func TestCredentialCheck_GroupCommandWithoutLogin(t *testing.T) {
	t.Run("lists subcommands", func(t *testing.T) {
		root, _ := newRootWithResource(t, "")
		var out bytes.Buffer
		root.Cmd.SetOut(&out)
		root.Cmd.SetArgs([]string{"products"})

		if err := root.Cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(out.String(), "list") {
			t.Errorf("expected the subcommands in help, got %q", out.String())
		}
	})
	t.Run("unknown subcommand is a usage error", func(t *testing.T) {
		root, _ := newRootWithResource(t, "")
		root.Cmd.SetArgs([]string{"products", "bogus"})

		if err := root.Cmd.Execute(); !errors.Is(err, commands.ErrUsage) {
			t.Fatalf("Execute error = %v, want one wrapping ErrUsage", err)
		}
	})
}

func TestCredentialCheck_KeyForAnotherEnvironment(t *testing.T) {
	mismatched := configtest.TestEnvKeyPrefix + "abc"
	for name, tc := range map[string]struct {
		storedKey, envVar string
		args              []string
	}{
		"--api-key":      {args: []string{"products", "list", "--api-key", mismatched}},
		"PADDLE_API_KEY": {envVar: mismatched, args: []string{"products", "list"}},
		"stored login":   {storedKey: mismatched, args: []string{"products", "list"}},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.envVar != "" {
				t.Setenv("PADDLE_API_KEY", tc.envVar)
			}
			root, ran := newRootWithResource(t, tc.storedKey)
			root.Cmd.SetArgs(tc.args)

			err := root.Cmd.Execute()
			if !errors.Is(err, client.ErrCredentialMismatch) {
				t.Fatalf("Execute error = %v, want one wrapping ErrCredentialMismatch", err)
			}
			if code, _ := client.ExitCodeFor(err, commands.ErrUsage); code != client.ExitAuthFailure {
				t.Errorf("exit code = %d, want %d", code, client.ExitAuthFailure)
			}
			if want := "--environment " + configtest.TestEnv; !strings.Contains(err.Error(), want) {
				t.Errorf("error %q doesn't suggest %q", err, want)
			}
			if *ran {
				t.Error("command ran with a key for another environment")
			}
		})
	}
}

func TestCredentialCheck_KeyForAnotherEnvironmentAllowed(t *testing.T) {
	mismatched := configtest.TestEnvKeyPrefix + "abc"
	for name, tc := range map[string]struct {
		args   []string
		apiURL string
	}{
		"matching key":            {args: []string{"products", "list", "--api-key", configtest.TestEnvAPIKeyOnlyKeyPrefix + "abc"}},
		"unknown key format":      {args: []string{"products", "list", "--api-key", "some_oauth_token"}},
		"API URL override":        {args: []string{"products", "list", "--api-key", mismatched}, apiURL: "http://127.0.0.1:1"},
		"command without the API": {args: []string{"environment", "--api-key", mismatched}},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.apiURL != "" {
				t.Setenv("PADDLE_API_URL", tc.apiURL)
			}
			root, ran := newRootWithResource(t, "")
			root.Cmd.SetArgs(tc.args)

			if err := root.Cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if tc.args[0] == "products" && !*ran {
				t.Error("command didn't run")
			}
		})
	}
}

func TestLoginCmd_RefusesKeyForAnotherEnvironment(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root))
	root.Cmd.SetOut(io.Discard)
	root.Cmd.SetErr(io.Discard)
	root.Cmd.SetArgs([]string{"login", "--environment", configtest.TestEnv, "--api-key", configtest.TestEnvAPIKeyOnlyKeyPrefix + "abc"})

	err := root.Cmd.Execute()
	if !errors.Is(err, client.ErrCredentialMismatch) {
		t.Fatalf("Execute error = %v, want one wrapping ErrCredentialMismatch", err)
	}
	if want := "--environment " + configtest.TestEnvAPIKeyOnly; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q doesn't suggest %q", err, want)
	}
	if _, statErr := os.Stat(cfgPath); !os.IsNotExist(statErr) {
		t.Errorf("config file written for a refused key (stat err: %v)", statErr)
	}
}
