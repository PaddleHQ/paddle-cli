package commands_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PaddleHQ/paddle-cli/internal/auth"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
)

// runCmd runs args against a fresh Root with the session commands registered
// and returns what was written to stderr.
func runCmd(t *testing.T, cfgPath string, args ...string) (string, error) {
	t.Helper()
	root := commands.NewRoot(commands.RootConfig{ConfigPath: cfgPath})
	root.Cmd.AddCommand(commands.NewLoginCmd(root), commands.NewLogoutCmd(root), commands.NewWhoamiCmd(root), commands.NewAPICmd(root))
	var stderr bytes.Buffer
	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs(args)
	err := root.Cmd.Execute()
	return stderr.String(), err
}

// seedAPIKey saves a test_env_apikey_only API-key credential through the installed store.
func seedAPIKey(t *testing.T, cfgPath string) {
	t.Helper()
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnvAPIKeyOnly,
		Environments:       map[string]config.EnvironmentConfig{configtest.TestEnvAPIKeyOnly: {APIKey: "key_abc"}},
	}, cfgPath); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
}

func runOAuthLogin(t *testing.T, cfgPath string) string {
	t.Helper()
	original := commands.LoginFunc
	commands.LoginFunc = func(auth.OAuthConfig) (*auth.Token, error) {
		return &auth.Token{AccessToken: "tok_test", RefreshToken: "ref_test"}, nil
	}
	t.Cleanup(func() { commands.LoginFunc = original })

	stderr, err := runCmd(t, cfgPath, "login", "--environment", configtest.TestEnv)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return stderr
}

func TestLoginCmd_KeychainStorageHasNoWarning(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	stderr := runOAuthLogin(t, cfgPath)

	if strings.Contains(stderr, "plaintext") {
		t.Errorf("unexpected plaintext warning: %q", stderr)
	}
	if store.Items[configtest.TestEnv].AccessToken != "tok_test" {
		t.Errorf("keychain item = %+v", store.Items[configtest.TestEnv])
	}
	raw, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(raw), "tok_test") {
		t.Errorf("token leaked to config file:\n%s", raw)
	}
}

func TestLoginCmd_WarnsWhenKeychainUnavailable(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	store.SetErr = errors.New("user interaction is not allowed")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	stderr := runOAuthLogin(t, cfgPath)

	if !strings.Contains(stderr, "credentials stored in plaintext at "+cfgPath) ||
		!strings.Contains(stderr, "user interaction is not allowed") {
		t.Errorf("expected plaintext warning with reason, got %q", stderr)
	}
}

func TestLoginCmd_WarnsWhenNoKeychainSupport(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	stderr := runOAuthLogin(t, cfgPath)

	if !strings.Contains(stderr, "credentials stored in plaintext") || !strings.Contains(stderr, "not supported") {
		t.Errorf("expected plaintext warning, got %q", stderr)
	}
}

func TestLoginCmd_APIKeyWarnsWhenKeychainUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":{}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("PADDLE_API_URL", server.URL)
	t.Setenv("PADDLE_NO_KEYCHAIN", "1")
	configtest.NewMemoryStore().Install(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	stderr, err := runCmd(t, cfgPath, "login", "--environment", configtest.TestEnvAPIKeyOnly, "--api-key", "key_abc")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(stderr, "PADDLE_NO_KEYCHAIN") {
		t.Errorf("expected opt-out reason in warning, got %q", stderr)
	}
}

func TestLogoutCmd_RemovesKeychainItem(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	seedAPIKey(t, cfgPath)

	if _, err := runCmd(t, cfgPath, "logout"); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, ok := store.Items[configtest.TestEnvAPIKeyOnly]; ok {
		t.Error("expected keychain item removed on logout")
	}
}

func TestRoot_WarnsWhenKeychainUnreadable(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	seedAPIKey(t, cfgPath)
	store.GetErr = errors.New("keychain locked")

	stderr, _ := runCmd(t, cfgPath, "whoami")
	if !strings.Contains(stderr, "keychain locked") {
		t.Errorf("expected keychain read warning, got %q", stderr)
	}
}

// runRefresh seeds an expired OAuth session, calls beforeRun, then runs a
// command that triggers a token refresh.
func runRefresh(t *testing.T, beforeRun func()) (cfgPath, stderr string, err error) {
	t.Helper()
	original := commands.RefreshFunc
	commands.RefreshFunc = func(context.Context, auth.OAuthConfig, string) (*auth.Token, error) {
		return &auth.Token{AccessToken: "at_new", RefreshToken: "rt_new", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	t.Cleanup(func() { commands.RefreshFunc = original })
	t.Setenv("PADDLE_API_URL", startWhoamiStub(t))

	cfgPath = filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(&config.Config{
		CurrentEnvironment: configtest.TestEnv,
		Environments: map[string]config.EnvironmentConfig{configtest.TestEnv: {
			AccessToken: "at_old", RefreshToken: "rt_old",
			ExpiresAt: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		}},
	}, cfgPath); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	beforeRun()

	stderr, err = runCmd(t, cfgPath, "whoami")
	return cfgPath, stderr, err
}

func TestRefresh_StoresRotatedTokensInKeychain(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	cfgPath, stderr, err := runRefresh(t, func() {})
	if err != nil {
		t.Fatalf("whoami: %v (%s)", err, stderr)
	}
	if got := store.Items[configtest.TestEnv]; got.AccessToken != "at_new" || got.RefreshToken != "rt_new" {
		t.Errorf("keychain item = %+v", got)
	}
	if raw, _ := os.ReadFile(cfgPath); strings.Contains(string(raw), "_new") {
		t.Errorf("refreshed tokens leaked to file:\n%s", raw)
	}
}

func TestRefresh_KeychainSaveFailureErrors(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	cfgPath, _, err := runRefresh(t, func() {
		store.SetErr = errors.New("keychain locked")
	})
	if err == nil || !strings.Contains(err.Error(), "keychain locked") || !strings.Contains(err.Error(), "paddle login") {
		t.Fatalf("whoami = %v, want keychain save error with login hint", err)
	}
	if got := store.Items[configtest.TestEnv]; got.AccessToken != "at_old" {
		t.Errorf("keychain item = %+v, want original session", got)
	}
	raw, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(raw), "at_new") || strings.Contains(string(raw), "at_old") {
		t.Errorf("tokens reached the config file:\n%s", raw)
	}
}
