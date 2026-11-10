package commands

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PaddleHQ/paddle-cli/internal/auth"
	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/config"

	"github.com/spf13/cobra"
)

// LoginFunc is the function used to perform the interactive login flow.
// Override in tests to avoid opening a browser.
var LoginFunc = auth.LoginInteractive

// NewLoginCmd builds `paddle login`, which stores an OAuth session or a validated API key.
func NewLoginCmd(root *Root) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "login",
		Short:   "Authenticate with Paddle via OAuth or an API key",
		GroupID: GroupIDSession,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, _ := cmd.Flags().GetString(flagEnvironment)
			agent, _ := cmd.Flags().GetBool("agent")
			apiKey, _ := cmd.Flags().GetString("api-key")

			cfgPath := root.Config.ConfigPath
			if cfgPath == "" {
				cfgPath = config.DefaultPath()
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}

			env = resolveLoginEnv(env, cfg)

			baseURL, err := config.EnvironmentURL(env)
			if err != nil {
				return err
			}
			override := os.Getenv("PADDLE_API_URL")
			if override != "" {
				baseURL = override
			}

			if apiKey != "" {
				if override == "" {
					if err := checkKeyEnvironment(apiKey, env); err != nil {
						return err
					}
				}
				return loginWithAPIKey(cmd, root, cfg, cfgPath, env, baseURL, apiKey)
			}
			return loginWithOAuth(cmd, root, cfg, cfgPath, env, agent)
		},
	}

	cmd.Flags().String(flagEnvironment, "", "Environment to authenticate with")
	cmd.Flags().Bool("agent", false, "Print the auth URL to stdout instead of opening a browser (for headless use)")

	return cmd
}

// resolveLoginEnv picks the target environment: --environment, then
// PADDLE_ENVIRONMENT, then the stored current environment, then the default.
func resolveLoginEnv(flagEnv string, cfg *config.Config) string {
	if flagEnv != "" {
		return flagEnv
	}
	if env := os.Getenv("PADDLE_ENVIRONMENT"); env != "" {
		return env
	}
	if cfg.CurrentEnvironment != "" {
		return cfg.CurrentEnvironment
	}
	return config.DefaultEnvironment
}

// loginWithOAuth runs the interactive OAuth flow and persists the resulting session.
func loginWithOAuth(cmd *cobra.Command, root *Root, cfg *config.Config, cfgPath, env string, agent bool) error {
	oauthEnv, err := config.OAuthConfig(env)
	if errors.Is(err, config.ErrOAuthNotSupported) {
		return usageError{fmt.Errorf("%w; use 'paddle login --api-key <key>' or set PADDLE_API_KEY", err)}
	}
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Logging in to %s environment...\n", env)

	scopes := append(append([]string{}, oauthEnv.Scopes...), config.ScopeAPIWrite)
	token, err := LoginFunc(auth.OAuthConfig{
		AuthURL:  oauthEnv.AuthURL(),
		TokenURL: oauthEnv.TokenURL(),
		ClientID: oauthEnv.ClientID,
		Scopes:   scopes,
		Agent:    agent,
		Output:   cmd.OutOrStdout(),
	})
	if err != nil {
		return err
	}

	// A token response without scope means the server granted exactly
	// what was requested (RFC 6749 §5.1).
	grantedScope := token.Scope
	if grantedScope == "" {
		grantedScope = strings.Join(scopes, " ")
	}

	cfg.CurrentEnvironment = env
	envCfg := config.EnvironmentConfig{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Scope:        grantedScope,
	}
	if !token.ExpiresAt.IsZero() {
		envCfg.ExpiresAt = token.ExpiresAt.UTC().Format(time.RFC3339)
	}
	cfg.Environments[env] = envCfg
	if err := config.Save(cfg, cfgPath); err != nil {
		return err
	}

	warnIfPlaintext(cmd, root, cfg, cfgPath, env)
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), root.OutputOpts().Profile.Success("Logged in to "+env))
	return nil
}

// loginWithAPIKey validates the given key against the target environment's
// API and, on success, persists it as that environment's stored credential,
// replacing any previously stored OAuth session. Nothing is written to
// disk if validation fails.
func loginWithAPIKey(cmd *cobra.Command, root *Root, cfg *config.Config, cfgPath, env, baseURL, apiKey string) error {
	// root.Client() is configured with whatever credential existed before this
	// login attempt, not the key being validated, so start from it so headers/
	// --api-version still apply, but override the two that must change.
	validationCfg := root.Client().Config()
	validationCfg.BaseURL = baseURL
	validationCfg.AccessToken = apiKey
	validationClient := client.New(validationCfg)
	resp, err := validationClient.Do("GET", "/check/whoami", nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return client.ParseAPIError(resp)
	}

	cfg.CurrentEnvironment = env
	cfg.Environments[env] = config.EnvironmentConfig{APIKey: apiKey}
	if err := config.Save(cfg, cfgPath); err != nil {
		return err
	}

	warnIfPlaintext(cmd, root, cfg, cfgPath, env)
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), root.OutputOpts().Profile.Success("Logged in to "+env))
	return nil
}

func warnIfPlaintext(cmd *cobra.Command, root *Root, cfg *config.Config, cfgPath, env string) {
	inKeychain, reason := cfg.StoredInKeychain(env)
	if inKeychain {
		return
	}
	msg := fmt.Sprintf("Warning: credentials stored in plaintext at %s", cfgPath)
	if reason != nil {
		msg += fmt.Sprintf(" (%v)", reason)
	}
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), root.OutputOpts().Profile.Warn(msg))
}
