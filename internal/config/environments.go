package config

import (
	"errors"
	"fmt"
	"sort"

	"github.com/muesli/termenv"
)

// Environment holds the configuration for a Paddle environment: its REST API
// base URL and the OAuth issuer and client used to log in to it.
type Environment struct {
	BaseURL    string   // API host for this Paddle environment
	AuthDomain string   // OAuth issuer host for this environment
	ClientID   string   // OAuth client ID (public client, PKCE; no secret). Empty means OAuth isn't configured for this environment yet.
	Scopes     []string // OAuth scopes requested on login

	// Color of the environment name in output. The zero value (black)
	// renders the name dimmed instead.
	Color termenv.ANSIColor
	Bold  bool // render the name in bold too, e.g. to flag production
}

// AuthURL is the OAuth 2.0 authorization endpoint.
func (e Environment) AuthURL() string { return "https://" + e.AuthDomain + "/oauth2/authorize" }

// TokenURL is the OAuth 2.0 token endpoint (used for both the
// authorization_code exchange and refresh_token grants).
func (e Environment) TokenURL() string { return "https://" + e.AuthDomain + "/oauth2/token" }

// openid enables OIDC and offline_access a refresh token; the OAuth app must
// be allowed to request each of them. Login appends one of the access scopes
// below.
var defaultScopes = []string{"openid", "offline_access"}

// Access scopes select how much of the user's role-derived access the issued
// token carries: the gateway maps ScopeAPIRead to the read-only subset of the
// user's role scopes and ScopeAPIWrite to all of them. Each must be registered
// on the environment's Connect app before it can be requested.
const (
	ScopeAPIRead  = "api_read"
	ScopeAPIWrite = "api_write"
)

var environments = map[string]Environment{
	"sandbox": {
		BaseURL:    "https://sandbox-api.paddle.com",
		AuthDomain: "sandbox-id.paddle.com",
		Scopes:     defaultScopes,
		Color:      termenv.ANSIYellow,
	},
	"production": {
		BaseURL:    "https://api.paddle.com",
		AuthDomain: "id.paddle.com",
		ClientID:   "client_01M4WWX8WHPSPYQPT5W3EJYH3N",
		Scopes:     defaultScopes,
		Color:      termenv.ANSIBrightRed,
		Bold:       true,
	},
}

// DefaultEnvironment is used by login when no environment is selected.
var DefaultEnvironment = "production"

// SetEnvironments replaces the known environments and the default, and
// returns a func that restores both. Tests use it to avoid depending on real
// environments.
func SetEnvironments(envs map[string]Environment, defaultEnv string) (restore func()) {
	prevEnvs, prevDefault := environments, DefaultEnvironment
	environments, DefaultEnvironment = envs, defaultEnv
	return func() { environments, DefaultEnvironment = prevEnvs, prevDefault }
}

// LookupEnvironment returns the named environment's configuration.
func LookupEnvironment(name string) (Environment, bool) {
	e, ok := environments[name]
	return e, ok
}

// EnvironmentURL returns the API base URL for the given environment.
func EnvironmentURL(env string) (string, error) {
	e, ok := environments[env]
	if !ok {
		return "", fmt.Errorf("%w %q, valid environments: %v", errUnknownEnvironment, env, ValidEnvironments())
	}
	return e.BaseURL, nil
}

var errUnknownEnvironment = errors.New("unknown environment")

// ErrOAuthNotSupported is returned by OAuthConfig for an environment with no
// OAuth client, which can only be used with an API key.
var ErrOAuthNotSupported = errors.New("OAuth is not supported")

// OAuthConfig returns the OAuth configuration for the given environment.
// Returns an error if the environment has no OAuth client configured.
func OAuthConfig(env string) (Environment, error) {
	e, ok := environments[env]
	if !ok {
		return Environment{}, fmt.Errorf("%w %q", errUnknownEnvironment, env)
	}
	if e.ClientID == "" || e.AuthDomain == "" {
		return Environment{}, fmt.Errorf("%w for the %q environment", ErrOAuthNotSupported, env)
	}
	return e, nil
}

// ValidEnvironments returns the names of all configured environments, sorted.
func ValidEnvironments() []string {
	envs := make([]string, 0, len(environments))
	for name := range environments {
		envs = append(envs, name)
	}
	sort.Strings(envs)
	return envs
}
