package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// useTestEnvironments installs dummy environments. configtest can't be used
// here: it imports this package.
func useTestEnvironments(t *testing.T) {
	t.Cleanup(SetEnvironments(map[string]Environment{
		"test_env": {
			BaseURL:      "https://api.example.test",
			AuthDomain:   "auth.example.test",
			ClientID:     "client_test",
			Scopes:       []string{"openid", "offline_access"},
			APIKeyPrefix: "test_live_apikey_",
		},
		"test_env_apikey_only": {BaseURL: "https://apikey-only.example.test", APIKeyPrefix: "test_sdbx_apikey_"},
	}, "test_env"))
}

func TestAPIKeyEnvironment(t *testing.T) {
	useTestEnvironments(t)

	for key, want := range map[string]string{
		"test_live_apikey_abc": "test_env",
		"test_sdbx_apikey_abc": "test_env_apikey_only",
		"some_oauth_token":     "",
		"":                     "",
	} {
		if got := APIKeyEnvironment(key); got != want {
			t.Errorf("APIKeyEnvironment(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestEnvironmentURL(t *testing.T) {
	useTestEnvironments(t)

	got, err := EnvironmentURL("test_env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://api.example.test" {
		t.Errorf("EnvironmentURL = %q, want https://api.example.test", got)
	}
}

func TestEnvironmentURL_UnknownListsValid(t *testing.T) {
	useTestEnvironments(t)

	_, err := EnvironmentURL("invalid")
	if err == nil {
		t.Fatal("expected error for unknown environment")
	}
	if !strings.Contains(err.Error(), "test_env test_env_apikey_only") {
		t.Errorf("error %q does not list the valid environments", err)
	}
}

func TestOAuthConfig(t *testing.T) {
	useTestEnvironments(t)

	e, err := OAuthConfig("test_env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := e.AuthURL(), "https://auth.example.test/oauth2/authorize"; got != want {
		t.Errorf("AuthURL = %q, want %q", got, want)
	}
	if got, want := e.TokenURL(), "https://auth.example.test/oauth2/token"; got != want {
		t.Errorf("TokenURL = %q, want %q", got, want)
	}
	if e.ClientID != "client_test" {
		t.Errorf("ClientID = %q, want client_test", e.ClientID)
	}

	if _, err := OAuthConfig("test_env_apikey_only"); !errors.Is(err, ErrOAuthNotSupported) {
		t.Errorf("err = %v, want ErrOAuthNotSupported for an environment without an OAuth client", err)
	}
	if _, err := OAuthConfig("invalid"); err == nil {
		t.Error("expected error for an unknown environment")
	}
}

func TestValidEnvironments(t *testing.T) {
	useTestEnvironments(t)

	if got, want := ValidEnvironments(), []string{"test_env", "test_env_apikey_only"}; !slices.Equal(got, want) {
		t.Errorf("ValidEnvironments() = %v, want %v", got, want)
	}
}

// The checks below run against the real environments, so they must hold for
// any environments.go a repository ships.

func TestEnvironments_Invariants(t *testing.T) {
	if len(environments) == 0 {
		t.Fatal("no environments configured")
	}
	for name, e := range environments {
		if e.BaseURL == "" {
			t.Errorf("%s: empty BaseURL", name)
		}
		if e.ClientID != "" && e.AuthDomain == "" {
			t.Errorf("%s: ClientID set without an OAuth domain", name)
		}
	}
}
