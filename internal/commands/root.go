package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PaddleHQ/paddle-cli/internal/agent"
	"github.com/PaddleHQ/paddle-cli/internal/auth"
	"github.com/PaddleHQ/paddle-cli/internal/build"
	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/output"
	"github.com/PaddleHQ/paddle-cli/internal/style"

	"github.com/spf13/cobra"
)

// RefreshFunc is the function used to refresh access tokens.
// Override in tests to avoid real HTTP calls.
var RefreshFunc = auth.RefreshAccessToken

// flagEnvironment names the persistent --environment flag (also defined locally on login).
const flagEnvironment = "environment"

// refreshSkew is how far before expiry we proactively refresh.
const refreshSkew = time.Minute

// RootConfig holds construction-time overrides for the root command.
type RootConfig struct {
	ConfigPath string // override for testing
}

// Root is the `paddle` root command plus the state its PersistentPreRunE resolves for subcommands.
type Root struct {
	Cmd              *cobra.Command
	ClientPtr        *client.Client
	OutputPtr        *output.Options
	Config           RootConfig
	Environment      string // resolved current environment, populated in setup
	BannerSuppressed bool   // resolved suppression from flag/env/config
	NonInteractive   bool   // --non-interactive: disables prompts without changing output format
	HasCredential    bool   // true once setup() resolves a non-empty bearer credential (API key or OAuth token)
}

// NewRoot builds the root command with its persistent flags and command groups.
func NewRoot(cfg RootConfig) *Root {
	r := &Root{
		Config:    cfg,
		ClientPtr: &client.Client{},
		OutputPtr: &output.Options{},
	}

	cmd := &cobra.Command{
		Use:               "paddle",
		Short:             "Paddle CLI: interact with the Paddle API",
		Version:           fmt.Sprintf("%s (%s)", build.Version, build.Commit),
		PersistentPreRunE: r.setup,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	pf := cmd.PersistentFlags()
	pf.StringP("output", "o", "",
		"Output format: table, json, csv (default: value of default_output in config / PADDLE_OUTPUT env, else table, or json with --agent)")
	pf.Bool("agent", false, "Agent mode: structured JSON output, no colors/prompts")
	pf.Bool("no-color", false, "Disable ANSI color output")
	pf.String("jq", "", "JQ expression to filter the response (works with all formats)")
	pf.StringArray("header", nil, "Custom HTTP header (repeatable, format: 'Key: Value')")
	pf.String(flagEnvironment, "",
		"Environment to use for this invocation (overrides PADDLE_ENVIRONMENT and the stored current_environment)")
	pf.Bool("no-banner", false, "Suppress the environment banner footer")
	pf.Bool("non-interactive", false,
		"Disable interactive prompts (TTY confirmation). Does not change output format. Use --agent to also force JSON output.")
	pf.String("api-version", "",
		"Paddle API version to use (sets Paddle-Version header). Overrides PADDLE_API_VERSION and api_version config key.")
	pf.String("api-key", "",
		"API key to use for this invocation (overrides PADDLE_API_KEY and any stored credential). "+
			"Use 'paddle login --api-key <key>' to persist one instead.")
	// Headers can carry credentials too (Authorization overrides the stored one).
	for _, name := range []string{"api-key", "header"} {
		_ = pf.SetAnnotation(name, output.FlagAnnotationSensitive, []string{"true"})
	}

	AddCommandGroups(cmd)

	r.Cmd = cmd
	return r
}

func (r *Root) setup(cmd *cobra.Command, _ []string) error {
	cfgPath := r.Config.ConfigPath
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	resolved, err := resolveWithFlags(cmd, cfg)
	if err != nil {
		return err
	}

	opts, err := resolveOutputOptions(cmd, resolved)
	if err != nil {
		return err
	}
	*r.OutputPtr = opts

	r.Environment = resolved.CurrentEnvironment

	noBannerFlag, _ := cmd.Flags().GetBool("no-banner")
	r.BannerSuppressed = noBannerFlag || resolved.NoBanner

	nonInteractiveFlag, _ := cmd.Flags().GetBool("non-interactive")
	r.NonInteractive = nonInteractiveFlag

	apiVersion := resolved.APIVersion
	if v, _ := cmd.Flags().GetString("api-version"); v != "" {
		apiVersion = v
	}

	apiURL := os.Getenv("PADDLE_API_URL")
	if apiURL == "" && resolved.CurrentEnvironment != "" {
		apiURL, _ = config.EnvironmentURL(resolved.CurrentEnvironment)
	}

	accessToken, err := r.resolveCredential(cmd, cfg, resolved, cfgPath)
	if err != nil {
		return err
	}
	r.HasCredential = accessToken != ""

	r.ClientPtr.Configure(client.Config{
		BaseURL:     apiURL,
		AccessToken: accessToken,
		APIVersion:  apiVersion,
		Headers:     resolveHeaders(cmd),
		ClientApp:   agent.Detect(),
	})

	return nil
}

// resolveOutputOptions picks format, jq filter, color profile and environment
// from flags, env vars and config.
func resolveOutputOptions(cmd *cobra.Command, resolved *config.Config) (output.Options, error) {
	agentFlag, _ := cmd.Flags().GetBool("agent")
	agentEnv := os.Getenv("PADDLE_AGENT") == "1"
	isAgent := agentFlag || agentEnv || isAgentEnvEnabled()
	isMachineOutput := isAgent || !output.IsStdoutTTY()

	var opts output.Options
	if isMachineOutput {
		opts = output.AgentOptions()
	} else {
		opts = output.DefaultOptions()
	}

	// Config file / PADDLE_OUTPUT applies whenever the user has explicitly
	// chosen a format, regardless of TTY state. Agent mode still ignores it
	// to keep the existing "agent forces JSON" contract; only --output
	// overrides agent.
	if !isAgent && resolved.DefaultOutput != "" {
		f, err := output.ParseFormat(resolved.DefaultOutput)
		if err != nil {
			return output.Options{}, fmt.Errorf("invalid default_output %q in config: %w", resolved.DefaultOutput, err)
		}
		opts.Format = f
	}

	formatStr, _ := cmd.Flags().GetString("output")
	if formatStr != "" {
		f, err := output.ParseFormat(formatStr)
		if err != nil {
			return output.Options{}, err
		}
		opts.Format = f
	}

	jqExpr, _ := cmd.Flags().GetString("jq")
	opts.JQ = jqExpr

	noColorFlag, _ := cmd.Flags().GetBool("no-color")
	opts.Profile = style.Resolve(style.Inputs{
		NoColorFlag:  noColorFlag,
		AgentFlag:    isAgent,
		NoColorEnv:   os.Getenv("NO_COLOR") != "",
		ForceColor:   os.Getenv("FORCE_COLOR") != "",
		ConfigPref:   resolved.Color,
		AutoDetected: detectColorAuto(),
	})

	opts.Environment = resolved.CurrentEnvironment
	return opts, nil
}

// resolveCredential returns the bearer credential, refusing up front a command
// that calls the API without one rather than letting the API reject it.
func (r *Root) resolveCredential(cmd *cobra.Command, cfg, resolved *config.Config, cfgPath string) (string, error) {
	accessToken, err := r.resolveAccessToken(cmd, cfg, resolved, cfgPath)
	if err != nil {
		return "", err
	}
	if !callsAPI(cmd) {
		return accessToken, nil
	}
	env := resolved.CurrentEnvironment
	if accessToken == "" {
		return "", fmt.Errorf("%w to %q; run 'paddle login', or pass --api-key or set PADDLE_API_KEY",
			client.ErrNotAuthenticated, env)
	}
	// PADDLE_API_URL points away from env's host, so env's key format doesn't apply.
	if os.Getenv("PADDLE_API_URL") == "" {
		if err := checkKeyEnvironment(accessToken, env); err != nil {
			return "", err
		}
	}
	return accessToken, nil
}

// checkKeyEnvironment returns an ErrCredentialMismatch error when key is an
// API key issued for an environment other than env.
func checkKeyEnvironment(key, env string) error {
	keyEnv := config.APIKeyEnvironment(key)
	if keyEnv == "" || keyEnv == env {
		return nil
	}
	return fmt.Errorf("%w: it's a %s key, but the environment is %q; pass --environment %s",
		client.ErrCredentialMismatch, keyEnv, env, keyEnv)
}

// resolveAccessToken returns the bearer credential: --api-key, then
// PADDLE_API_KEY, then the stored credential (refreshing an expiring OAuth token).
func (r *Root) resolveAccessToken(cmd *cobra.Command, cfg, resolved *config.Config, cfgPath string) (string, error) {
	if apiKeyFlag, _ := cmd.Flags().GetString("api-key"); apiKeyFlag != "" {
		return apiKeyFlag, nil
	}
	if apiKeyEnv := os.Getenv("PADDLE_API_KEY"); apiKeyEnv != "" {
		return apiKeyEnv, nil
	}
	envCfg, ok := resolved.Environments[resolved.CurrentEnvironment]
	if !ok {
		return "", nil
	}
	if err := cfg.CredentialReadError(resolved.CurrentEnvironment); err != nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), r.OutputOpts().Profile.Warn("Warning: "+err.Error()))
	}
	if envCfg.APIKey != "" {
		return envCfg.APIKey, nil
	}
	if !shouldRefresh(envCfg) || skipRefresh(cmd) {
		return envCfg.AccessToken, nil
	}
	refreshed, err := r.refreshEnvToken(resolved.CurrentEnvironment, envCfg, cfgPath)
	if err != nil {
		return "", fmt.Errorf("refreshing access token: %w "+
			"(run 'paddle logout' then 'paddle login --api-key <key>', or set PADDLE_API_KEY)", err)
	}
	return refreshed, nil
}

// resolveHeaders merges --header flags with PADDLE_HEADER, the env var winning on conflict.
func resolveHeaders(cmd *cobra.Command) map[string]string {
	headerFlags, _ := cmd.Flags().GetStringArray("header")
	headers := parseHeaders(headerFlags)

	if envHeaders := os.Getenv("PADDLE_HEADER"); envHeaders != "" {
		for h := range strings.SplitSeq(envHeaders, ",") {
			k, v, ok := strings.Cut(h, ":")
			if ok {
				headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	return headers
}

// Client returns the API client configured during setup.
func (r *Root) Client() *client.Client {
	return r.ClientPtr
}

// OutputOpts returns a copy of the output options resolved during setup.
func (r *Root) OutputOpts() output.Options {
	return *r.OutputPtr
}

// annotationCallsAPI marks a hand-crafted command that calls the Paddle API,
// so it needs a credential. Resource commands are covered by their group.
const annotationCallsAPI = "paddle_calls_api"

func callsAPIAnnotations() map[string]string {
	return map[string]string{annotationCallsAPI: "true"}
}

// callsAPI reports whether cmd sends a request to the Paddle API: a resource
// command (unless it only prints its --schema) or one marked annotationCallsAPI.
// Group commands only print help or a usage error, so they never do.
func callsAPI(cmd *cobra.Command) bool {
	if cmd.HasSubCommands() {
		return false
	}
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return false
	}
	top := cmd
	for top.HasParent() && top.Parent().HasParent() {
		top = top.Parent()
	}
	_, marked := cmd.Annotations[annotationCallsAPI]
	return marked || (top != cmd.Root() && top.GroupID == GroupIDResources)
}

// isAgentEnvEnabled reports whether the process is running inside a known
// agent environment. Checked in addition to --agent / PADDLE_AGENT=1.
func isAgentEnvEnabled() bool {
	return agent.Detect() != ""
}

// shouldRefresh returns true if the stored token is expired (or within refreshSkew of expiry)
// and a refresh token is available.
func shouldRefresh(envCfg config.EnvironmentConfig) bool {
	if envCfg.RefreshToken == "" || envCfg.ExpiresAt == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, envCfg.ExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().Add(refreshSkew).After(expiresAt)
}

// skipRefresh returns true for commands that should not trigger token refresh
// (login manages its own tokens; logout is about to discard them).
func skipRefresh(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "login", "logout":
		return true
	}
	return false
}

// refreshEnvToken refreshes the stored token for the given environment and persists the result.
// Returns the new access token.
func (r *Root) refreshEnvToken(env string, envCfg config.EnvironmentConfig, cfgPath string) (string, error) {
	oauthEnv, err := config.OAuthConfig(env)
	if err != nil {
		return "", err
	}
	tok, err := RefreshFunc(context.Background(), auth.OAuthConfig{
		TokenURL: oauthEnv.TokenURL(),
		ClientID: oauthEnv.ClientID,
	}, envCfg.RefreshToken)
	if err != nil {
		return "", err
	}

	// Keychain sessions never fall back to plaintext; the old refresh token may
	// already be rotated, hence the login hint.
	if err := persistRefreshedToken(env, tok, cfgPath); err != nil {
		return "", fmt.Errorf("saving refreshed session (if retrying fails, run 'paddle login'): %w", err)
	}
	return tok.AccessToken, nil
}

func persistRefreshedToken(env string, tok *auth.Token, cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	newEnvCfg := config.EnvironmentConfig{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Scope:        tok.Scope,
	}
	// A refresh response without scope keeps the originally granted scope.
	if newEnvCfg.Scope == "" {
		newEnvCfg.Scope = cfg.Environments[env].Scope
	}
	if !tok.ExpiresAt.IsZero() {
		newEnvCfg.ExpiresAt = tok.ExpiresAt.UTC().Format(time.RFC3339)
	}
	cfg.Environments[env] = newEnvCfg
	return config.Save(cfg, cfgPath)
}

// resolveWithFlags loads env-var and CLI-flag overrides on top of the stored
// config. The --environment flag takes precedence over PADDLE_ENVIRONMENT and
// the stored current_environment; an invalid value is rejected explicitly so
// the user sees a clear error rather than a silent empty API URL.
func resolveWithFlags(cmd *cobra.Command, cfg *config.Config) (*config.Config, error) {
	resolved := config.ResolveConfig(cfg)
	envFlag, _ := cmd.Flags().GetString(flagEnvironment)
	if envFlag == "" {
		return resolved, nil
	}
	if _, err := config.EnvironmentURL(envFlag); err != nil {
		return nil, err
	}
	resolved.CurrentEnvironment = envFlag
	return resolved, nil
}

// detectColorAuto reports whether stdout looks like an interactive terminal
// capable of ANSI styling. Delegates to output.IsStdoutTTY (which the
// non-TTY auto-detection workstream installed) so the same TTY signal that
// gates output format also gates color.
func detectColorAuto() bool {
	return output.IsStdoutTTY()
}

func parseHeaders(headerFlags []string) map[string]string {
	headers := make(map[string]string)
	for _, h := range headerFlags {
		k, v, ok := strings.Cut(h, ":")
		if ok {
			headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return headers
}
