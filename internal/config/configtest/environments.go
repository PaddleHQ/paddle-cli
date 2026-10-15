// Package configtest provides test doubles for the config package.
package configtest

import (
	"github.com/PaddleHQ/paddle-cli/internal/config"

	"github.com/muesli/termenv"
)

const (
	// TestEnv has OAuth configured.
	TestEnv = "test_env"
	// TestEnvAPIKeyOnly has no OAuth client, so only API keys work.
	TestEnvAPIKeyOnly = "test_env_apikey_only" //nolint:gosec // environment name, not a credential
)

// Environments returns a fresh set of dummy environments.
func Environments() map[string]config.Environment {
	return map[string]config.Environment{
		TestEnv: {
			BaseURL:    "https://api.example.test",
			AuthDomain: "auth.example.test",
			ClientID:   "client_test",
			Scopes:     []string{"openid", "offline_access"},
			Color:      termenv.ANSIBrightRed,
			Bold:       true,
		},
		TestEnvAPIKeyOnly: {
			BaseURL: "https://apikey-only.example.test",
			Color:   termenv.ANSIYellow,
		},
	}
}

// Install makes Environments() the known environments, with TestEnv as the
// default, and returns a func that restores the real ones. Call it from
// TestMain so no test in the package sees a real environment.
func Install() (restore func()) {
	return config.SetEnvironments(Environments(), TestEnv)
}
