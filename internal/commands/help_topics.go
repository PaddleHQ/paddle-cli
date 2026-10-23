package commands

import (
	"fmt"
	"strings"

	"github.com/PaddleHQ/paddle-cli/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// InstallHelpTopics replaces cobra's built-in help command with one that also
// handles topic pages (environment, exit-codes, formatting, api-versions).
func InstallHelpTopics(root *Root) {
	topics := buildTopics(root)
	rootCmd := root.Cmd

	helpCmd := &cobra.Command{
		Use:   "help [command or topic]",
		Short: "Help about any command or topic",
		Long: `Help provides help for any command or topic.

Available topics:
  environment   Named environments, paddle switch, config file, env vars
  exit-codes    Exit code reference
  formatting    Output formats, --jq, --output, non-TTY behavior
  api-versions  --api-version flag, PADDLE_API_VERSION, api_version config key

Type "paddle help <topic>" for a topic, or "paddle help <command>" for a command.`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// DisableFlagParsing hands us global flags too (paddle --agent help).
			args = withoutFlags(args, rootCmd.PersistentFlags())
			if len(args) == 0 {
				return rootCmd.Help()
			}
			if content, ok := topics[args[0]]; ok {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), content)
				return nil
			}
			found, rest, err := rootCmd.Find(args)
			// Words left over under a command group name a subcommand that doesn't exist.
			if err == nil && found != nil && found != rootCmd && (len(rest) == 0 || !found.HasSubCommands()) {
				return found.Help()
			}
			return fmt.Errorf("%w: unknown help topic or command %q", ErrUsage, strings.Join(args, " "))
		},
	}

	rootCmd.SetHelpCommand(helpCmd)
	rootCmd.AddCommand(helpCmd)
}

// withoutFlags drops flags from args, along with the separate value of any
// flag in flags that takes one (--environment sandbox, -o json).
func withoutFlags(args []string, flags *pflag.FlagSet) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
			continue
		}
		var f *pflag.Flag
		if name, ok := strings.CutPrefix(a, "--"); ok {
			f = flags.Lookup(name)
		} else if len(a) == 2 {
			f = flags.ShorthandLookup(a[1:])
		}
		if f != nil && f.NoOptDefVal == "" {
			i++
		}
	}
	return out
}

func buildTopics(root *Root) map[string]string {
	cfgPath := config.DefaultPath()
	if root.Config.ConfigPath != "" {
		cfgPath = root.Config.ConfigPath
	}
	return map[string]string{
		"environment":  environmentTopic(cfgPath),
		"exit-codes":   exitCodesTopic(),
		"formatting":   formattingTopic(),
		"api-versions": apiVersionsTopic(),
	}
}

func environmentTopic(cfgPath string) string {
	return fmt.Sprintf(`Environment: Named environments and configuration

NAMED ENVIRONMENTS

The Paddle CLI uses named environments to target different API endpoints.
Available environments: %s

SWITCHING ENVIRONMENTS

  paddle switch sandbox        Switch to sandbox and persist
  paddle switch production     Switch to production and persist
  paddle environment           Show the active environment and API URL

CONFIG FILE

  %s

Current environment is stored as current_environment. Override per-invocation with:

  --environment <name>         CLI flag (highest priority)
  PADDLE_ENVIRONMENT=<name>    Environment variable
  current_environment: <name>  Config file (lowest priority)

ENV VARS

  PADDLE_ENVIRONMENT   Override active environment for one invocation
  PADDLE_API_URL       Override API base URL (ignores environment)
  PADDLE_API_KEY       API key to use (overrides stored credential)
  PADDLE_NO_KEYCHAIN   Set to 1 to keep credentials out of the macOS Keychain (no reads or writes)

CREDENTIAL STORAGE

On macOS, tokens and API keys from paddle login are kept in the login
Keychain (service "paddle-cli", one item per environment). Elsewhere, or
when the Keychain is unavailable, they are written to the config file with
0600 permissions and paddle login prints a warning.

`, strings.Join(config.ValidEnvironments(), ", "), cfgPath)
}

func exitCodesTopic() string {
	return `Exit Codes: Paddle CLI exit code reference

  0   Success
  1   General error (unexpected error, request failed)
  2   Usage error (unknown flag, unknown subcommand, missing required argument,
      destructive op without --confirm in non-interactive context)
  3   Not found (API returned 404)
  4   Authentication error (not logged in, an API key for a different
      environment, or a missing, malformed or invalid key)
  5   Authorisation error (authenticated but not permitted)
  6   Rate limit (429: too many requests)
  7   Validation error (422: request body failed validation)
  8   Conflict (409: conflicting state, e.g. duplicate)
  9   Server error (5xx: Paddle API returned an internal error)
  124 Timeout (request timed out before a response was received)

Scripts can branch on exit codes without parsing error messages:

  paddle customers get "$ID"
  case $? in
    0) echo "found" ;;
    3) echo "not found" ;;
    4) echo "not logged in, run: paddle login" ;;
  esac

`
}

func formattingTopic() string {
	return `Formatting: Output formats, filtering, and non-TTY behavior

OUTPUT FORMATS

  --output table    Human-readable table (default on TTY)
  --output json     Structured JSON
  --output csv      CSV with headers
  -o json           Shorthand for --output json

Non-TTY stdout (pipe, redirect, agent harness): automatically switches to JSON
and strips ANSI escapes. paddle --agent forces the same JSON output explicitly.

  paddle customers list | jq '.data[].id'   # non-TTY → JSON automatically
  paddle --output csv customers list > out.csv

FORMAT PRECEDENCE (highest to lowest)

  1. --output / -o flag
  2. PADDLE_OUTPUT environment variable
  3. default_output in config (set via: paddle config set default_output json)
  4. --agent flag or PADDLE_AGENT=1 → forces JSON
  5. Non-TTY stdout → JSON
  6. TTY stdout → table

JQ FILTERING

  --jq '<expr>'   Apply a jq expression before rendering (works for all formats)

  paddle customers list --jq '.data | map({id, email})'
  paddle customers list --output table --jq '.data[0:5]'
  paddle customers list --jq '.data | length'

The expression runs before rendering, so it shapes what the table or CSV shows.

COLOR

  NO_COLOR=1                   Disable ANSI color
  --no-color                   Disable ANSI color for this invocation
  paddle config set color never   Persist color=never in config

AGENT MODE

  --agent                   JSON output + no color + no interactive prompts
  PADDLE_AGENT=1            Equivalent env var

  Automatically enabled when any of these are set: AI_AGENT, AGENT (naming a
  known agent), CLAUDECODE, CURSOR_AGENT, CODEX_CI, GEMINI_CLI, OPENCODE, CI,
  or any AIDER_* variable.

`
}

func apiVersionsTopic() string {
	return `API Versions: Controlling the Paddle API version header

The Paddle API honors a Paddle-Version header to pin behavior to a specific
API version. Use --api-version (or the equivalent env var / config key) to set it.

FLAG

  paddle --api-version 2025-01-01 customers list

ENV VAR

  PADDLE_API_VERSION=2025-01-01 paddle customers list

CONFIG KEY

  paddle config set api_version 2025-01-01   # persists across invocations
  paddle config get api_version
  paddle config unset api_version

PRIORITY (highest → lowest)

  1. --api-version flag
  2. PADDLE_API_VERSION env var
  3. api_version config key

When none is set, no Paddle-Version header is sent and the API uses its default.

`
}
