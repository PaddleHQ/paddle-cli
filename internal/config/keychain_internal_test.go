package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Touches the real login keychain, so it only runs when asked to.
func TestMacOSKeychain_RoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("PADDLE_TEST_KEYCHAIN") != "1" {
		t.Skip("set PADDLE_TEST_KEYCHAIN=1 on macOS to run")
	}
	k := newMacOSKeychain("paddle-cli-test")
	account := fmt.Sprintf("test-%d", os.Getpid())
	t.Cleanup(func() { _ = k.Delete(account) })

	want := Secrets{AccessToken: strings.Repeat("a", 1500), RefreshToken: `r"'\ $x`, APIKey: "pdl_key"}
	if err := k.Set(account, want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := k.Get(account)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("Get = %+v, want %+v", got, want)
	}
	if err := k.Delete(account); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := k.Get(account); !errors.Is(err, ErrCredentialNotFound) {
		t.Errorf("Get after Delete = %v, want ErrCredentialNotFound", err)
	}
}

func TestMacOSKeychain_RejectsOversizedSecrets(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "login.keychain-db")
	if err := os.WriteFile(existing, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	bin, ranFile := fakeSecurity(t, existing, 0)
	k := &macOSKeychain{bin: bin, service: "x", timeout: 5 * time.Second}
	err := k.Set("test_env_apikey_only", Secrets{AccessToken: strings.Repeat("a", securityMaxLine)})
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("Set = %v, want size error", err)
	}
	if ran, err := os.ReadFile(ranFile); err == nil {
		t.Errorf("oversized command was sent to security: %s", ran)
	}
}

func TestValidateAccount(t *testing.T) {
	for _, bad := range []string{"", "a b", `a"b`, `a\b`, "a\nb", "a\x01b", "é"} {
		if validateAccount(bad) == nil {
			t.Errorf("validateAccount(%q) = nil, want error", bad)
		}
	}
	if err := validateAccount("test_env_apikey_only"); err != nil {
		t.Errorf("validateAccount(test_env_apikey_only) = %v", err)
	}
}

// fakeSecurity writes a stand-in for /usr/bin/security. default-keychain
// prints defaultKeychain (quoted, as security does; empty prints nothing) and
// exits with defaultExit; every other command succeeds with a valid item and
// records that it ran.
func fakeSecurity(t *testing.T, defaultKeychain string, defaultExit int) (bin, ranFile string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "security")
	ranFile = filepath.Join(dir, "ran")
	item := base64.StdEncoding.EncodeToString([]byte(`{"api_key":"key"}`))
	if defaultKeychain != "" {
		defaultKeychain = `    "` + defaultKeychain + `"`
	}
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = default-keychain ]; then echo '%s'; exit %d; fi\necho \"$1\" >> '%s'\necho %s\n",
		defaultKeychain, defaultExit, ranFile, item)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, ranFile
}

func TestMacOSKeychain_NoDefaultKeychainSkipsKeychainCommands(t *testing.T) {
	cases := map[string]struct {
		defaultKeychain string
		exit            int
	}{
		"command fails": {"", 1},
		"file missing":  {"/nonexistent/login.keychain-db", 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			bin, ranFile := fakeSecurity(t, c.defaultKeychain, c.exit)
			k := &macOSKeychain{bin: bin, service: "x", timeout: 5 * time.Second}
			if err := k.Set("test_env_apikey_only", Secrets{APIKey: "key"}); err == nil {
				t.Error("Set = nil, want readiness error")
			}
			if _, err := k.Get("test_env_apikey_only"); err == nil {
				t.Error("Get = nil, want readiness error")
			}
			if err := k.Delete("test_env_apikey_only"); err == nil {
				t.Error("Delete = nil, want readiness error")
			}
			if ran, err := os.ReadFile(ranFile); err == nil {
				t.Errorf("keychain commands ran without a default keychain: %s", ran)
			}
		})
	}
}

func TestMacOSKeychain_ReadyKeychainRunsCommands(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "login.keychain-db")
	if err := os.WriteFile(existing, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	bin, _ := fakeSecurity(t, existing, 0)
	k := &macOSKeychain{bin: bin, service: "x", timeout: 5 * time.Second}
	got, err := k.Get("test_env_apikey_only")
	if err != nil || got.APIKey != "key" {
		t.Errorf("Get = %+v, %v", got, err)
	}
}

func TestMacOSKeychain_TimesOut(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "security")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	k := &macOSKeychain{bin: bin, service: "x", timeout: 100 * time.Millisecond}
	start := time.Now()
	_, err := k.Get("test_env_apikey_only")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("Get = %v, want timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Get took %s, timeout not enforced", elapsed)
	}
}
