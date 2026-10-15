package commands_test

import (
	"bytes"
	"errors"
	"io"
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
// "list" and "create" leaves record whether they ran. No credentials are stored.
func newRootWithResource(t *testing.T) (root *commands.Root, ran *bool) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(&config.Config{CurrentEnvironment: configtest.TestEnvAPIKeyOnly}, cfgPath); err != nil {
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
			root, ran := newRootWithResource(t)
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
			root, ran := newRootWithResource(t)
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
		root, _ := newRootWithResource(t)
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
		root, _ := newRootWithResource(t)
		root.Cmd.SetArgs([]string{"products", "bogus"})

		if err := root.Cmd.Execute(); !errors.Is(err, commands.ErrUsage) {
			t.Fatalf("Execute error = %v, want one wrapping ErrUsage", err)
		}
	})
}
