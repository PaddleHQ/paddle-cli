package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	return string(data)
}

func oauthConfig() *config.Config {
	return &config.Config{
		CurrentEnvironment: "test_env_apikey_only",
		Environments: map[string]config.EnvironmentConfig{
			"test_env_apikey_only": {AccessToken: "at", RefreshToken: "rt", ExpiresAt: "2030-01-01T00:00:00Z"},
		},
	}
}

func TestSave_KeychainKeepsSecretsOutOfFile(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := oauthConfig()
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw := readFile(t, path)
	for _, leak := range []string{"access_token", "refresh_token", ": at\n", ": rt\n"} {
		if strings.Contains(raw, leak) {
			t.Errorf("config file leaks %q:\n%s", leak, raw)
		}
	}
	if !strings.Contains(raw, "credential_store: keychain") {
		t.Errorf("expected keychain marker in file:\n%s", raw)
	}
	if !strings.Contains(raw, "expires_at:") {
		t.Errorf("expected expires_at to stay in file:\n%s", raw)
	}
	if got := store.Items["test_env_apikey_only"]; got != (config.Secrets{AccessToken: "at", RefreshToken: "rt"}) {
		t.Errorf("keychain item = %+v", got)
	}
	if ok, reason := cfg.StoredInKeychain("test_env_apikey_only"); !ok {
		t.Errorf("StoredInKeychain = false (%v), want true", reason)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	env := loaded.Environments["test_env_apikey_only"]
	if env.AccessToken != "at" || env.RefreshToken != "rt" || env.ExpiresAt != "2030-01-01T00:00:00Z" {
		t.Errorf("loaded env = %+v", env)
	}
}

func TestSave_FallsBackToFileWhenKeychainWriteFails(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	store.SetErr = errors.New("user interaction is not allowed")
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := oauthConfig()
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw := readFile(t, path)
	if !strings.Contains(raw, "access_token: at") {
		t.Errorf("expected plaintext fallback:\n%s", raw)
	}
	if strings.Contains(raw, "credential_store") {
		t.Errorf("unexpected keychain marker on fallback:\n%s", raw)
	}
	ok, reason := cfg.StoredInKeychain("test_env_apikey_only")
	if ok || reason == nil || !strings.Contains(reason.Error(), "user interaction is not allowed") {
		t.Errorf("StoredInKeychain = %v, %v", ok, reason)
	}
}

func TestSave_NoStoreWritesFile(t *testing.T) {
	t.Cleanup(config.SetCredentialStore(nil))
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := oauthConfig()
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if raw := readFile(t, path); !strings.Contains(raw, "access_token: at") {
		t.Errorf("expected plaintext:\n%s", raw)
	}
	if ok, reason := cfg.StoredInKeychain("test_env_apikey_only"); ok || reason == nil {
		t.Errorf("StoredInKeychain = %v, %v", ok, reason)
	}
}

func TestOptOut_LeavesKeychainItemsAlone(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(oauthConfig(), path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	store.Sets = nil

	t.Setenv("PADDLE_NO_KEYCHAIN", "1")
	store.GetErr = errors.New("must not read the keychain")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.CredentialReadError("test_env_apikey_only"); err == nil || !strings.Contains(err.Error(), "PADDLE_NO_KEYCHAIN") {
		t.Errorf("CredentialReadError = %v", err)
	}
	cfg.Color = "never"
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if raw := readFile(t, path); !strings.Contains(raw, "credential_store: keychain") {
		t.Errorf("expected keychain marker preserved:\n%s", raw)
	}
	if _, ok := store.Items["test_env_apikey_only"]; !ok || len(store.Sets) != 0 || len(store.Deletes) != 0 {
		t.Errorf("keychain touched under opt-out: item=%v sets=%v deletes=%v", ok, store.Sets, store.Deletes)
	}
}

func TestOptOut_NewLoginWritesFile(t *testing.T) {
	configtest.NewMemoryStore().Install(t)
	t.Setenv("PADDLE_NO_KEYCHAIN", "1")
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := oauthConfig()
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if raw := readFile(t, path); !strings.Contains(raw, "access_token: at") {
		t.Errorf("expected plaintext:\n%s", raw)
	}
	ok, reason := cfg.StoredInKeychain("test_env_apikey_only")
	if ok || reason == nil || !strings.Contains(reason.Error(), "PADDLE_NO_KEYCHAIN") {
		t.Errorf("StoredInKeychain = %v, %v", ok, reason)
	}
}

func TestSave_KeychainWriteFailureNeverDowngradesToPlaintext(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(oauthConfig(), path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before := readFile(t, path)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Environments["test_env_apikey_only"] = config.EnvironmentConfig{AccessToken: "at2", RefreshToken: "rt2"}
	store.SetErr = errors.New("keychain locked")
	if err := config.Save(cfg, path); err == nil || !strings.Contains(err.Error(), "keychain locked") {
		t.Fatalf("Save = %v, want keychain error", err)
	}

	if after := readFile(t, path); after != before {
		t.Errorf("config file changed on failed save:\n%s", after)
	}
	if store.Items["test_env_apikey_only"].AccessToken != "at" {
		t.Errorf("keychain item = %+v, want original", store.Items["test_env_apikey_only"])
	}
}

func TestSave_KeychainDeleteFailureLeavesFileUnchanged(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(oauthConfig(), path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before := readFile(t, path)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	delete(cfg.Environments, "test_env_apikey_only")
	store.DeleteErr = errors.New("keychain locked")
	if err := config.Save(cfg, path); err == nil || !strings.Contains(err.Error(), "keychain locked") {
		t.Fatalf("Save = %v, want keychain error", err)
	}
	if after := readFile(t, path); after != before {
		t.Errorf("config file changed on failed save, item would be orphaned:\n%s", after)
	}
}

func TestSave_ReloginWhileKeychainLockedFails(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(oauthConfig(), path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before := readFile(t, path)

	locked := errors.New("keychain locked")
	store.GetErr, store.SetErr, store.DeleteErr = locked, locked, locked
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Environments["test_env_apikey_only"] = config.EnvironmentConfig{APIKey: "key_new"}
	if err := config.Save(cfg, path); err == nil {
		t.Fatal("Save = nil, want keychain error")
	}
	if after := readFile(t, path); after != before {
		t.Errorf("expected no plaintext fallback for a keychain-stored environment:\n%s", after)
	}
}

func TestSave_UnchangedSecretsSkipKeychainWrite(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(oauthConfig(), path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	store.Sets = nil

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Color = "never"
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(store.Sets) != 0 || len(store.Deletes) != 0 {
		t.Errorf("expected no keychain writes, got sets=%v deletes=%v", store.Sets, store.Deletes)
	}
}

func TestSave_RemovedEnvironmentDeletesKeychainItem(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(oauthConfig(), path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	delete(cfg.Environments, "test_env_apikey_only")
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := store.Items["test_env_apikey_only"]; ok {
		t.Error("expected keychain item deleted")
	}
}

func TestSave_MigratesPlaintextIntoKeychain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	legacy := "current_environment: test_env_apikey_only\nenvironments:\n  test_env_apikey_only:\n    api_key: key_abc\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store := configtest.NewMemoryStore().Install(t)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if raw := readFile(t, path); strings.Contains(raw, "key_abc") {
		t.Errorf("expected api key moved out of file:\n%s", raw)
	}
	if store.Items["test_env_apikey_only"].APIKey != "key_abc" {
		t.Errorf("keychain item = %+v", store.Items["test_env_apikey_only"])
	}
}

func TestLoad_UnreadableKeychainPreservesItem(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(oauthConfig(), path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	store.GetErr = errors.New("keychain locked")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.CredentialReadError("test_env_apikey_only"); err == nil || !strings.Contains(err.Error(), "keychain locked") {
		t.Errorf("CredentialReadError = %v", err)
	}
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	store.GetErr = nil
	if raw := readFile(t, path); !strings.Contains(raw, "credential_store: keychain") {
		t.Errorf("expected keychain marker preserved:\n%s", raw)
	}
	if _, ok := store.Items["test_env_apikey_only"]; !ok {
		t.Error("expected keychain item preserved")
	}
}

func TestLoad_MissingKeychainItemYieldsNoCredential(t *testing.T) {
	configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := "environments:\n  test_env_apikey_only:\n    credential_store: keychain\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if env := cfg.Environments["test_env_apikey_only"]; env.AccessToken != "" || env.APIKey != "" {
		t.Errorf("expected no credential, got %+v", env)
	}
	if err := cfg.CredentialReadError("test_env_apikey_only"); err == nil || !strings.Contains(err.Error(), "paddle login") {
		t.Errorf("CredentialReadError = %v, want a hint to log in again", err)
	}
}

func TestSave_RemovingUnreadableEnvironmentDeletesKeychainItem(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(oauthConfig(), path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	store.GetErr = errors.New("keychain locked")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	delete(cfg.Environments, "test_env_apikey_only")
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok := store.Items["test_env_apikey_only"]; ok {
		t.Error("expected keychain item deleted on logout even though it was unreadable")
	}
}

func TestOptOut_LogoutAndReloginUseFileOnly(t *testing.T) {
	cases := map[string]struct {
		mutate     func(*config.Config)
		wantInFile string
	}{
		"logout": {func(c *config.Config) { delete(c.Environments, "test_env_apikey_only") }, ""},
		"relogin": {func(c *config.Config) {
			c.Environments["test_env_apikey_only"] = config.EnvironmentConfig{APIKey: "key_new"}
		}, "api_key: key_new"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			store := configtest.NewMemoryStore().Install(t)
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := config.Save(oauthConfig(), path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			store.Sets = nil

			t.Setenv("PADDLE_NO_KEYCHAIN", "1")
			store.GetErr = errors.New("must not read the keychain")
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			c.mutate(cfg)
			if err := config.Save(cfg, path); err != nil {
				t.Fatalf("Save: %v", err)
			}

			raw := readFile(t, path)
			if strings.Contains(raw, "credential_store") {
				t.Errorf("keychain marker kept under opt-out:\n%s", raw)
			}
			if c.wantInFile != "" && !strings.Contains(raw, c.wantInFile) {
				t.Errorf("expected %q in file:\n%s", c.wantInFile, raw)
			}
			if len(store.Sets) != 0 || len(store.Deletes) != 0 {
				t.Errorf("keychain touched under opt-out: sets=%v deletes=%v", store.Sets, store.Deletes)
			}
		})
	}
}

func TestSave_RepeatedSavesOnOneConfig(t *testing.T) {
	store := configtest.NewMemoryStore().Install(t)
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := oauthConfig()
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	cfg.Environments["test_env_apikey_only"] = config.EnvironmentConfig{AccessToken: "at2", RefreshToken: "rt2"}
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	store.Sets = nil
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("third Save: %v", err)
	}
	if len(store.Sets) != 0 {
		t.Errorf("unchanged third save wrote to keychain: %v", store.Sets)
	}
	delete(cfg.Environments, "test_env_apikey_only")
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("fourth Save: %v", err)
	}
	if _, ok := store.Items["test_env_apikey_only"]; ok {
		t.Error("expected item deleted after removing env on a reused Config")
	}
}

func TestSave_FollowsSymlinkedConfig(t *testing.T) {
	configtest.NewMemoryStore().Install(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "real.yaml")
	link := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(oauthConfig(), link); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("config symlink replaced: %v, %v", fi, err)
	}
	if raw := readFile(t, target); !strings.Contains(raw, "credential_store: keychain") {
		t.Errorf("symlink target not written:\n%s", raw)
	}
}
