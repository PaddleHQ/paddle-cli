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

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestRootCommand_Version(t *testing.T) {
	root := commands.NewRoot(commands.RootConfig{})
	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"--version"})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRootCommand_HasGlobalFlags(t *testing.T) {
	root := commands.NewRoot(commands.RootConfig{})
	flags := []string{"output", "agent", "jq", "header", "environment"}
	for _, name := range flags {
		f := root.Cmd.PersistentFlags().Lookup(name)
		if f == nil {
			t.Errorf("expected persistent flag %q", name)
		}
	}
	// -o is the documented shorthand for --output.
	if root.Cmd.PersistentFlags().ShorthandLookup("o") == nil {
		t.Error("expected -o shorthand for --output")
	}
}

func TestRootCommand_CredentialFlagsAreSensitive(t *testing.T) {
	root := commands.NewRoot(commands.RootConfig{})
	for _, name := range []string{"api-key", "header"} {
		f := root.Cmd.PersistentFlags().Lookup(name)
		if f == nil {
			t.Fatalf("expected persistent flag %q", name)
		}
		if _, ok := f.Annotations[output.FlagAnnotationSensitive]; !ok {
			t.Errorf("--%s isn't annotated as sensitive, so it would be replayed in next-page commands", name)
		}
	}
}

func TestRootCommand_HasNonInteractiveFlag(t *testing.T) {
	root := commands.NewRoot(commands.RootConfig{})
	f := root.Cmd.PersistentFlags().Lookup("non-interactive")
	if f == nil {
		t.Fatal("expected persistent flag --non-interactive")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("expected bool flag, got %s", f.Value.Type())
	}
}

func TestRootCommand_HasAPIVersionFlag(t *testing.T) {
	root := commands.NewRoot(commands.RootConfig{})
	f := root.Cmd.PersistentFlags().Lookup("api-version")
	if f == nil {
		t.Fatal("expected persistent flag --api-version")
	}
	if f.Value.Type() != "string" {
		t.Errorf("expected string flag, got %s", f.Value.Type())
	}
}

func TestRootCommand_DefaultOutputFromConfig(t *testing.T) {
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStdoutTTY = original })

	tests := []struct {
		name       string
		configFmt  string
		envFmt     string
		flagFmt    string
		agent      bool
		wantFormat output.Format
		wantErr    bool
	}{
		{name: "no config → table", wantFormat: output.FormatTable},
		{name: "config json", configFmt: "json", wantFormat: output.FormatJSON},
		{name: "config csv", configFmt: "csv", wantFormat: output.FormatCSV},
		{name: "env overrides config", configFmt: "csv", envFmt: "json", wantFormat: output.FormatJSON},
		{name: "flag overrides env+config", configFmt: "csv", envFmt: "json", flagFmt: "table", wantFormat: output.FormatTable},
		{name: "agent mode ignores config", configFmt: "csv", agent: true, wantFormat: output.FormatJSON},
		{name: "agent mode + explicit flag", configFmt: "csv", agent: true, flagFmt: "csv", wantFormat: output.FormatCSV},
		{name: "invalid config errors", configFmt: "xml", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			cfg := &config.Config{DefaultOutput: tt.configFmt}
			if err := config.Save(cfg, cfgPath); err != nil {
				t.Fatalf("saving config: %v", err)
			}
			if tt.envFmt != "" {
				t.Setenv("PADDLE_OUTPUT", tt.envFmt)
			}

			root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
			root.Cmd.AddCommand(&cobra.Command{
				Use:  "noop",
				RunE: func(_ *cobra.Command, _ []string) error { return nil },
			})
			args := []string{"noop"}
			if tt.flagFmt != "" {
				args = append(args, "--output", tt.flagFmt)
			}
			if tt.agent {
				args = append(args, "--agent")
			}
			root.Cmd.SetArgs(args)
			var buf bytes.Buffer
			root.Cmd.SetOut(&buf)

			err := root.Cmd.Execute()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := root.OutputOpts().Format; got != tt.wantFormat {
				t.Errorf("format = %q, want %q", got, tt.wantFormat)
			}
		})
	}
}

func TestRootCommand_NonTTYDefaultsToAgentLikeOutput(t *testing.T) {
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return false }
	t.Cleanup(func() { output.IsStdoutTTY = original })
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("NO_COLOR", "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{}, cfgPath); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(&cobra.Command{
		Use:  "noop",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	root.Cmd.SetArgs([]string{"noop"})
	root.Cmd.SetOut(&bytes.Buffer{})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := root.OutputOpts()
	if got.Format != output.FormatJSON {
		t.Errorf("Format = %q, want json", got.Format)
	}
	if got.Profile.IsEnabled() {
		t.Error("Profile.IsEnabled = true, want false under non-TTY")
	}
	if !got.Compact {
		t.Error("Compact = false, want true under non-TTY")
	}
	if !got.NoTrunc {
		t.Error("NoTrunc = false, want true under non-TTY")
	}
}

func TestRootCommand_TTYDefaultsToTable(t *testing.T) {
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStdoutTTY = original })
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("NO_COLOR", "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{}, cfgPath); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(&cobra.Command{
		Use:  "noop",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	root.Cmd.SetArgs([]string{"noop"})
	root.Cmd.SetOut(&bytes.Buffer{})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := root.OutputOpts()
	if got.Format != output.FormatTable {
		t.Errorf("Format = %q, want table", got.Format)
	}
	if !got.Profile.IsEnabled() {
		t.Error("Profile.IsEnabled = false, want true under TTY")
	}
}

func TestRootCommand_NonTTY_ExplicitOutputHonoured(t *testing.T) {
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return false }
	t.Cleanup(func() { output.IsStdoutTTY = original })
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("NO_COLOR", "")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{DefaultOutput: "csv"}, cfgPath); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(&cobra.Command{
		Use:  "noop",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	root.Cmd.SetArgs([]string{"noop"})
	root.Cmd.SetOut(&bytes.Buffer{})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := root.OutputOpts()
	if got.Format != output.FormatCSV {
		t.Errorf("Format = %q, want csv (config default_output must beat non-TTY auto-default)", got.Format)
	}
	if got.Profile.IsEnabled() {
		t.Error("Profile.IsEnabled = true, want false: non-TTY must still strip ANSI even with explicit format")
	}
}

func TestRootCommand_EnvironmentFlagOverride(t *testing.T) {
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
	root.Cmd.SetArgs([]string{"--environment", configtest.TestEnvAPIKeyOnly, "--agent", "environment"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"test_env_apikey_only"`) {
		t.Errorf("expected test_env_apikey_only in output, got: %s", out)
	}
	if !strings.Contains(out, "apikey-only.example.test") {
		t.Errorf("expected test_env_apikey_only API URL in output, got: %s", out)
	}
}

func TestRootCommand_EnvironmentFlag_OverridesEnvVar(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{CurrentEnvironment: configtest.TestEnv}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatalf("saving config: %v", err)
	}
	t.Setenv("PADDLE_ENVIRONMENT", configtest.TestEnv)

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewEnvironmentCmd(root))

	var buf bytes.Buffer
	root.Cmd.SetOut(&buf)
	root.Cmd.SetArgs([]string{"--environment", configtest.TestEnvAPIKeyOnly, "--agent", "environment"})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `"test_env_apikey_only"`) {
		t.Errorf("expected flag to beat PADDLE_ENVIRONMENT, got: %s", buf.String())
	}
}

func TestRoot_ColourDetection_Precedence(t *testing.T) {
	makeConfig := func(t *testing.T, color string) string {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		body := "current_environment: test_env_apikey_only\ncolor: " + color + "\nenvironments: {}\n"
		if color == "" {
			body = "current_environment: test_env_apikey_only\nenvironments: {}\n"
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return path
	}

	cases := []struct {
		name        string
		args        []string
		envForce    string
		envNoColor  string
		configColor string
		want        bool
	}{
		{name: "agent_disables", args: []string{"--agent", "noop"}, want: false},
		{name: "no_color_flag_disables", args: []string{"--no-color", "noop"}, want: false},
		{name: "force_color_enables", args: []string{"noop"}, envForce: "1", want: true},
		{name: "no_color_env_disables", args: []string{"noop"}, envNoColor: "1", want: false},
		{name: "config_always_enables", args: []string{"noop"}, configColor: "always", want: true},
		{name: "config_never_disables", args: []string{"noop"}, configColor: "never", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfgPath := makeConfig(t, c.configColor)
			t.Setenv("FORCE_COLOR", c.envForce)
			t.Setenv("NO_COLOR", c.envNoColor)

			r := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
			r.Cmd.AddCommand(&cobra.Command{
				Use:  "noop",
				RunE: func(_ *cobra.Command, _ []string) error { return nil },
			})
			r.Cmd.SetArgs(c.args)
			r.Cmd.SetOut(io.Discard)
			r.Cmd.SetErr(io.Discard)
			_ = r.Cmd.Execute()

			if got := r.OutputOpts().Profile.IsEnabled(); got != c.want {
				t.Errorf("Profile.IsEnabled = %v, want %v", got, c.want)
			}
		})
	}
}

func TestRootCommand_EnvironmentFlag_InvalidEnv(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{CurrentEnvironment: configtest.TestEnvAPIKeyOnly}
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewEnvironmentCmd(root))
	root.Cmd.SetErr(&bytes.Buffer{})

	root.Cmd.SetArgs([]string{"--environment", "does-not-exist", "environment"})
	if err := root.Cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid environment name")
	}
}

func TestRootCommand_PopulatesEnvironmentInOptions(t *testing.T) {
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStdoutTTY = original })

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok"}},
	}, cfgPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(&cobra.Command{
		Use:  "noop",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	root.Cmd.SetArgs([]string{"noop"})
	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if got := root.OutputOpts().Environment; got != configtest.TestEnvAPIKeyOnly {
		t.Errorf("Options.Environment = %q, want test_env_apikey_only", got)
	}
}

func TestRootCommand_BannerSuppressedByFlag(t *testing.T) {
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStdoutTTY = original })

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok"}},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(&cobra.Command{Use: "noop", RunE: func(_ *cobra.Command, _ []string) error { return nil }})
	root.Cmd.SetArgs([]string{"--no-banner", "noop"})
	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !root.BannerSuppressed {
		t.Error("BannerSuppressed = false, want true with --no-banner")
	}
}

func TestRootCommand_BannerSuppressedByEnv(t *testing.T) {
	t.Setenv("PADDLE_NO_BANNER", "1")
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStdoutTTY = original })

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok"}},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(&cobra.Command{Use: "noop", RunE: func(_ *cobra.Command, _ []string) error { return nil }})
	root.Cmd.SetArgs([]string{"noop"})
	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !root.BannerSuppressed {
		t.Error("BannerSuppressed = false, want true with PADDLE_NO_BANNER=1")
	}
}

func TestRootCommand_BannerSuppressedByConfig(t *testing.T) {
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStdoutTTY = original })

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		NoBanner:           true,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok"}},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(&cobra.Command{Use: "noop", RunE: func(_ *cobra.Command, _ []string) error { return nil }})
	root.Cmd.SetArgs([]string{"noop"})
	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !root.BannerSuppressed {
		t.Error("BannerSuppressed = false, want true with no_banner: true in config")
	}
}

func TestRootCommand_BannerNotSuppressedByDefault(t *testing.T) {
	original := output.IsStdoutTTY
	output.IsStdoutTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStdoutTTY = original })

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {AccessToken: "tok"}},
	}, cfgPath); err != nil {
		t.Fatal(err)
	}

	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(&cobra.Command{Use: "noop", RunE: func(_ *cobra.Command, _ []string) error { return nil }})
	root.Cmd.SetArgs([]string{"noop"})
	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})

	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if root.BannerSuppressed {
		t.Error("BannerSuppressed = true, want false by default")
	}
	if root.Environment != configtest.TestEnvAPIKeyOnly {
		t.Errorf("Root.Environment = %q, want test_env_apikey_only", root.Environment)
	}
}

func TestRootSetup_SendsDetectedAgentAsClientApp(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"no agent", nil, ""},
		{"claude code", map[string]string{"CLAUDECODE": "1"}, "claude-code"},
		{"codex", map[string]string{"CODEX_CI": "1"}, "codex"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			var got http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"data":[]}`)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			t.Setenv("PADDLE_API_URL", server.URL)
			t.Setenv("PADDLE_API_KEY", "key_test")

			cfgPath := filepath.Join(t.TempDir(), "config.yaml")
			root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
			root.Cmd.AddCommand(commands.NewAPICmd(root))
			root.Cmd.SetOut(&bytes.Buffer{})
			root.Cmd.SetErr(&bytes.Buffer{})
			root.Cmd.SetArgs([]string{"api", "/event-types"})
			if err := root.Cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}

			if v := got.Get("X-Paddle-Client-App"); v != tc.want {
				t.Errorf("X-Paddle-Client-App = %q, want %q", v, tc.want)
			}
		})
	}
}

func TestRootSetup_CredentialPrecedence(t *testing.T) {
	cases := []struct {
		name        string
		flag        string
		envVar      string
		storedKey   string
		storedTok   string
		wantAuth    string
		wantHasCred bool
	}{
		{name: "flag wins over everything", flag: "flag_key", envVar: "env_key", storedKey: "stored_key", storedTok: "stored_tok", wantAuth: "Bearer flag_key", wantHasCred: true},
		{name: "env var wins over stored", envVar: "env_key", storedKey: "stored_key", storedTok: "stored_tok", wantAuth: "Bearer env_key", wantHasCred: true},
		{name: "stored api key wins over stored oauth token", storedKey: "stored_key", storedTok: "stored_tok", wantAuth: "Bearer stored_key", wantHasCred: true},
		{name: "falls back to stored oauth token", storedTok: "stored_tok", wantAuth: "Bearer stored_tok", wantHasCred: true},
		{name: "nothing set", wantAuth: "", wantHasCred: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			cfg := &config.Config{
				CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
				Environments: map[string]config.EnvironmentConfig{
					configtest.TestEnvAPIKeyOnly: {APIKey: tc.storedKey, AccessToken: tc.storedTok},
				},
			}
			if err := config.Save(cfg, cfgPath); err != nil {
				t.Fatalf("saving config: %v", err)
			}
			t.Setenv("PADDLE_API_URL", server.URL)
			if tc.envVar != "" {
				t.Setenv("PADDLE_API_KEY", tc.envVar)
			}

			var gotHasCred bool
			root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
			root.Cmd.AddCommand(&cobra.Command{
				Use: "noop",
				RunE: func(_ *cobra.Command, _ []string) error {
					gotHasCred = root.HasCredential
					_, err := root.Client().Do("GET", "/ping", nil, nil)
					return err
				},
			})
			args := []string{"noop"}
			if tc.flag != "" {
				args = append(args, "--api-key", tc.flag)
			}
			root.Cmd.SetArgs(args)
			root.Cmd.SetOut(&bytes.Buffer{})
			root.Cmd.SetErr(&bytes.Buffer{})

			if err := root.Cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if gotAuth != tc.wantAuth {
				t.Errorf("Authorization header = %q, want %q", gotAuth, tc.wantAuth)
			}
			if gotHasCred != tc.wantHasCred {
				t.Errorf("HasCredential = %v, want %v", gotHasCred, tc.wantHasCred)
			}
		})
	}
}

func TestRootCommand_PersistentFlagUsageHasNoEmbeddedBackticks(t *testing.T) {
	root := commands.NewRoot(commands.RootConfig{})
	root.Cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if strings.Contains(f.Usage, "`") {
			t.Errorf("flag %q usage contains a backtick, which pflag misinterprets as a value-placeholder delimiter: %q", f.Name, f.Usage)
		}
	})
}

// A header can carry a credential, so a malformed one fails without echoing it.
func TestRootSetup_RejectsMalformedHeaders(t *testing.T) {
	cases := []struct {
		name    string
		headers []string
		env     string
		want    string
	}{
		{"no colon", []string{"Authorization Bearer secret"}, "", "--header #1"},
		{"empty name", []string{": secret"}, "", "--header #1"},
		{"space in name", []string{"Authorization Bearer secret: x"}, "", "--header #1"},
		{"newline in value", []string{"X-Ok: 1", "X-Foo: a\r\nX-Secret: b"}, "", "--header #2"},
		{"empty flag", []string{""}, "", "--header #1"},
		{"bad env entry", nil, "X-Ok: 1,secret", "PADDLE_HEADER entry #2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hit := false
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
			defer server.Close()
			t.Setenv("PADDLE_API_URL", server.URL)
			t.Setenv("PADDLE_API_KEY", "key_test")
			t.Setenv("PADDLE_HEADER", tc.env)

			root := commands.NewRoot(commands.RootConfig{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")})
			root.Cmd.AddCommand(commands.NewAPICmd(root))
			root.Cmd.SetOut(&bytes.Buffer{})
			root.Cmd.SetErr(&bytes.Buffer{})
			args := []string{"api", "/event-types"}
			for _, h := range tc.headers {
				args = append(args, "--header", h)
			}
			root.Cmd.SetArgs(args)

			err := root.Cmd.Execute()
			if !errors.Is(err, commands.ErrUsage) {
				t.Fatalf("expected a usage error, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should name %s", err, tc.want)
			}
			if strings.Contains(strings.ToLower(err.Error()), "secret") {
				t.Errorf("error %q echoes the header's contents", err)
			}
			if hit {
				t.Error("request was sent despite the malformed header")
			}
		})
	}
}

func TestRootSetup_SendsWellFormedHeaders(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":[]}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("PADDLE_API_URL", server.URL)
	t.Setenv("PADDLE_API_KEY", "key_test")
	t.Setenv("PADDLE_HEADER", "X-Env: 2,") // a trailing comma isn't an entry

	root := commands.NewRoot(commands.RootConfig{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")})
	root.Cmd.AddCommand(commands.NewAPICmd(root))
	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})
	root.Cmd.SetArgs([]string{"api", "/event-types", "--header", "X-Flag: a:b"})
	if err := root.Cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if v := got.Get("X-Flag"); v != "a:b" {
		t.Errorf("X-Flag = %q, want %q", v, "a:b")
	}
	if v := got.Get("X-Env"); v != "2" {
		t.Errorf("X-Env = %q, want %q", v, "2")
	}
}
