package commands_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
)

// Drives login → authenticated request → logout through the real login
// keychain, under a throwaway service name so no real paddle-cli item is touched.
func TestKeychainEndToEnd(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("PADDLE_TEST_KEYCHAIN") != "1" {
		t.Skip("set PADDLE_TEST_KEYCHAIN=1 on macOS to run")
	}
	store := config.NewMacOSKeychain("paddle-cli-e2e-test")
	t.Cleanup(config.SetCredentialStore(store))
	t.Cleanup(func() { _ = store.Delete(configtest.TestEnvAPIKeyOnly) })

	var gotAuth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":{}}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	t.Setenv("PADDLE_API_URL", server.URL)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	run := func(args ...string) string {
		t.Helper()
		stderr, err := runCmd(t, cfgPath, args...)
		if err != nil {
			t.Fatalf("%v: %v (stderr: %s)", args, err, stderr)
		}
		return stderr
	}

	if stderr := run("login", "--environment", configtest.TestEnvAPIKeyOnly, "--api-key", "pdl_e2e_fake"); strings.Contains(stderr, "plaintext") {
		t.Fatalf("login fell back to plaintext: %s", stderr)
	}
	raw, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(raw), "pdl_e2e_fake") || !strings.Contains(string(raw), "credential_store: keychain") {
		t.Fatalf("unexpected config file:\n%s", raw)
	}
	if sec, err := store.Get(configtest.TestEnvAPIKeyOnly); err != nil || sec.APIKey != "pdl_e2e_fake" {
		t.Fatalf("keychain item = %+v, %v", sec, err)
	}

	run("api", "/event-types")
	if last := gotAuth[len(gotAuth)-1]; last != "Bearer pdl_e2e_fake" {
		t.Errorf("request used %q, want key from keychain", last)
	}

	run("logout")
	if _, err := store.Get(configtest.TestEnvAPIKeyOnly); !errors.Is(err, config.ErrCredentialNotFound) {
		t.Errorf("keychain item after logout: %v, want not found", err)
	}
}
