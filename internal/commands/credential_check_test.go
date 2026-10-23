package commands_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
	"github.com/PaddleHQ/paddle-cli/internal/generated"

	"github.com/spf13/cobra"
)

// newRootWithResource builds a root with "products list" and "products create",
// and "countries list" marked as needing no credential, as the public endpoint
// is. Each leaf records whether it ran. storedKey, if set, is stored as the
// login for the current environment.
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
	countries := &cobra.Command{Use: "countries"}
	countries.AddCommand(&cobra.Command{
		Use:         "list",
		Annotations: map[string]string{commands.AnnotationNoAuth: "true"},
		RunE:        run,
	})

	root = commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(products, countries, commands.NewAPICmd(root), commands.NewEnvironmentCmd(root))
	commands.AssignDefaultGroup(root.Cmd, commands.GroupIDResources)
	commands.EnforceUnknownSubcommandError(root.Cmd) // as main does: group commands get a RunE
	root.Cmd.SetOut(io.Discard)
	root.Cmd.SetErr(io.Discard)
	return root, ran
}

func TestCredentialCheck_NotLoggedIn(t *testing.T) {
	for name, args := range map[string][]string{
		"resource command": {"products", "list"},
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
		"api key flag":    {"products", "list", "--api-key", "pdl_sdbx_apikey_test"},
		"schema only":     {"products", "create", "--schema"},
		"no-API command":  {"environment"},
		"no-auth command": {"countries", "list"},
	} {
		t.Run(name, func(t *testing.T) {
			root, ran := newRootWithResource(t, "")
			root.Cmd.SetArgs(args)

			if err := root.Cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if args[0] != "environment" && !*ran {
				t.Error("command didn't run")
			}
		})
	}
}

// `paddle api` takes any path, so without a login it sends the request bare
// and leaves the API to answer, as it does for a public endpoint.
func TestCredentialCheck_APIWithoutLoginLeavesItToTheAPI(t *testing.T) {
	var sent bool
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, auth = true, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"type":"request_error","code":"forbidden","detail":"denied"}}`))
	}))
	defer server.Close()
	t.Setenv("PADDLE_API_URL", server.URL)

	root, _ := newRootWithResource(t, "")
	root.Cmd.SetArgs([]string{"api", "/products"})
	err := root.Cmd.Execute()

	if !sent {
		t.Fatalf("request wasn't sent; Execute error = %v", err)
	}
	if auth != "" {
		t.Errorf("Authorization = %q, want none", auth)
	}
	if code, _ := client.ExitCodeFor(err, commands.ErrUsage); code != client.ExitForbidden {
		t.Errorf("exit code = %d, want the API's 403 (%d); error: %v", code, client.ExitForbidden, err)
	}
}

// An Authorization header overrides the stored credential, so it counts as one.
func TestCredentialCheck_AuthorizationHeaderCountsAsCredential(t *testing.T) {
	for name, tc := range map[string]struct {
		args      []string
		envHeader string
	}{
		"--header":      {args: []string{"products", "list", "--header", "Authorization: Bearer some_token"}},
		"PADDLE_HEADER": {args: []string{"products", "list"}, envHeader: "authorization: Bearer some_token"},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.envHeader != "" {
				t.Setenv("PADDLE_HEADER", tc.envHeader)
			}
			root, ran := newRootWithResource(t, "")
			root.Cmd.SetArgs(tc.args)

			if err := root.Cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !*ran {
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
		"api command":    {args: []string{"api", "/products", "--api-key", mismatched}},
		// The API rejects a bad key even on an endpoint that needs none.
		"no-auth command": {args: []string{"countries", "list", "--api-key", mismatched}},
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

// The generator marks commands for endpoints that need no credential with a
// literal key; it must match the one the credential check reads.
func TestCredentialCheck_GeneratedNoAuthCommandsAreMarked(t *testing.T) {
	for _, cmd := range []*cobra.Command{
		generated.NewCountriesCmd(nil, nil),
		generated.NewCurrenciesCmd(nil, nil),
		generated.NewIpAddressesCmd(nil, nil),
	} {
		for _, leaf := range cmd.Commands() {
			if _, ok := leaf.Annotations[commands.AnnotationNoAuth]; !ok {
				t.Errorf("%s %s isn't marked %s", cmd.Name(), leaf.Name(), commands.AnnotationNoAuth)
			}
		}
	}
	products := generated.NewProductsCmd(nil, nil)
	for _, leaf := range products.Commands() {
		if _, ok := leaf.Annotations[commands.AnnotationNoAuth]; ok {
			t.Errorf("products %s is marked %s, but needs a credential", leaf.Name(), commands.AnnotationNoAuth)
		}
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
