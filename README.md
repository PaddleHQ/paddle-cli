# Paddle CLI

[Paddle Billing](https://developer.paddle.com/?utm_source=dx&utm_medium=paddle-cli) is the developer-first merchant of
record, designed for modern SaaS, AI, mobile app, and digital product businesses. We take care of payments, tax,
subscriptions, and metrics with one unified API that does it all.

This is a command-line interface for the Paddle Billing API, for people and agents. Resource commands are generated from
Paddle's API specification.

> **Important:** This package works with Paddle Billing. It does not support Paddle Classic. To work with Paddle
> Classic, see: [Paddle Classic dev docs](https://classic.paddle.com/?utm_source=dx&utm_medium=paddle-cli)

## Learn more

- [Paddle API reference](https://developer.paddle.com/api-reference/overview?utm_source=dx&utm_medium=paddle-cli)
- [Sign up for Paddle Billing](https://login.paddle.com/signup?utm_source=dx&utm_medium=paddle-cli)

## Before you begin

If you've used this CLI, we'd love to hear how you found it! Email us at [team-dx@paddle.com](mailto:team-dx@paddle.com)
with any thoughts.

## Install

macOS and Linux, with Homebrew:

```bash
brew install paddlehq/tap/paddle
```

macOS and Linux, with the install script:

```bash
curl -fsSL https://raw.githubusercontent.com/PaddleHQ/paddle-cli/main/install.sh | sh
```

The script downloads the latest release for your OS and architecture, checks it against the release's `checksums.txt`,
and installs `paddle` into `/usr/local/bin` if that's writable, or `~/.local/bin` otherwise. To choose the version or
directory, pass options after `sh -s --`, or set `PADDLE_INSTALL_VERSION` and `PADDLE_INSTALL_DIR`:

```bash
curl -fsSL https://raw.githubusercontent.com/PaddleHQ/paddle-cli/main/install.sh | sh -s -- --version v0.1.0 --dir ~/bin
```

To install by hand, download the `paddle_<version>_<os>_<arch>.tar.gz` archive for your platform from the
[latest release](https://github.com/PaddleHQ/paddle-cli/releases/latest), check it against `checksums.txt`, and put the
`paddle` binary on your `PATH`. On Windows, download `paddle_<version>_windows_<arch>.zip` instead and put `paddle.exe`
on your `PATH`.

Releases are built for macOS, Linux and Windows on amd64 and arm64. To check that a downloaded archive was built by this
repository's release workflow:

```bash
gh attestation verify paddle_<version>_<os>_<arch>.tar.gz --repo PaddleHQ/paddle-cli
```

## Install from local

```bash
git clone https://github.com/PaddleHQ/paddle-cli.git
cd paddle-cli
make build                    # build first (runs as you, never as root)
make install-local-build      # symlinks bin/paddle into $INSTALL_DIR (default: /usr/local/bin)
make uninstall-local-build    # remove the symlink
```

To pick a different directory (e.g. a user-writable one so you don't need sudo):

```bash
make install-local-build INSTALL_DIR=~/.local/bin
```

The installation target checks whether `$INSTALL_DIR` is on your `PATH` and warns if it isn't. Run `make path` to list
writable directories already on your `PATH`:

```bash
make path
# →   /Users/you/.local/bin                              writable
#     /opt/homebrew/bin                                  writable
#     /usr/local/bin
#     /usr/bin
```

Verify:

```bash
which paddle        # → the symlink path
paddle --version
```

The symlink points at the build output in `bin/paddle`, so `make build` after pulling new changes is enough to pick them
up, with no need to re-run `make install-local-build`.

## Usage

```bash
# choose your environment
paddle production                      # switch to production
paddle sandbox                         # switch to sandbox

# authenticate
paddle login                           # via OAuth
paddle login --api-key <key>           # via API key

# get help
paddle help                            # see usage
paddle customers --help                # see individual command help
paddle help environment                # built-in help topics: environment, exit-codes, formatting, api-versions

# example usage
paddle customers list                  # list customers
paddle customers get ctm_abc123        # get a customer
paddle api /transactions               # raw API call
paddle whoami                          # show the environment and who you're authenticated as (--full expands scope list)
```

### Agent mode

Agent mode forces JSON output and disables colors / prompts, so responses stay small enough for an LLM to reason over.

```bash
paddle --agent customers list          # JSON output, no colours/prompts
PADDLE_AGENT=1 paddle customers list   # equivalent
```

Agent mode is also activated automatically when any of these environment variables are set: `AI_AGENT`, `AGENT` (when it
names a known agent), `CLAUDECODE`, `CURSOR_AGENT`, `CODEX_CI`, `GEMINI_CLI`, `OPENCODE`, `CI`, or any variable prefixed
`AIDER_`. No flag needed when running inside most agent harnesses.

### Output format

Precedence (highest → lowest):

1. `--output` (`-o`) flag.
2. `PADDLE_OUTPUT` env var.
3. `default_output` in `~/.config/paddle/config.yaml`.
4. `--agent` / `PADDLE_AGENT=1`: forces JSON.
5. **Non-TTY stdout** (piped, redirected, captured by an agent harness): forces JSON, ANSI escapes stripped, output
   compact. Same shape as `--agent`. The `--agent` flag becomes a no-op when stdout is already a pipe, but it's still safe to
   pass for clarity.
6. TTY stdout: table.

An environment banner is printed to stderr after every invocation (e.g. `→ sandbox`). Suppress it with `--no-banner`,
`PADDLE_NO_BANNER=1`, or `no_banner: true` in `~/.config/paddle/config.yaml`. It goes to stderr, so it doesn't affect
JSON output; agents can also read `meta.environment`.

Output can be filtered with `--jq '<expr>'` in any format (uses [gojq](https://github.com/itchyny/gojq)). The expression
runs as a preprocessing step before rendering, so it works uniformly across JSON, table, and CSV. For example,
`--jq '.data | map({id, email})'` reshapes the rows of a table as well as the JSON.

### Global flags

These flags live on the root command and apply to every subcommand:

| Flag                    | Env var              | Config key        | Description                                                                                                                                                                       |
|-------------------------|----------------------|-------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--agent`               | `PADDLE_AGENT=1`     |                   | Force JSON output, disable colours and prompts                                                                                                                                    |
| `--environment <env>`   | `PADDLE_ENVIRONMENT` |                   | Per-invocation environment override (doesn't change stored current env)                                                                                                           |
| `--output <fmt>` / `-o` | `PADDLE_OUTPUT`      | `default_output`  | Output format: `table`, `json`, `csv`                                                                                                                                             |
| `--jq <expr>`           |                      |                   | Filter/reshape output before rendering                                                                                                                                            |
| `--api-key <key>`       | `PADDLE_API_KEY`     |                   | API key for this invocation, overrides any stored credential (a real key here lands in shell history and `ps` output, so prefer `PADDLE_API_KEY` or `paddle login --api-key <key>`) |
| `--non-interactive`     |                      |                   | Disable TTY prompts without changing output format                                                                                                                                |
| `--no-banner`           | `PADDLE_NO_BANNER=1` | `no_banner: true` | Suppress the environment banner printed to stderr                                                                                                                                 |
| `--header 'K: V'`       |                      |                   | Custom HTTP header, repeatable                                                                                                                                                    |

### Destructive operations

Generated commands for destructive verbs (delete, archive, cancel, and similar) require confirmation:

- **Interactive (TTY):** a prompt asks you to type the resource ID to confirm.
- **Non-interactive / scripted:** pass `--confirm=<resource-id>` to skip the prompt. Required whenever stdout is not a
  TTY, `--non-interactive` is set, or `--agent` / `PADDLE_AGENT` is active.

```bash
paddle customers delete ctm_abc123 --confirm ctm_abc123
paddle --agent subscriptions cancel sub_xyz --confirm sub_xyz
```

### Discovering request-body schemas

Every command that takes a request body (create / update ops) exposes two discovery surfaces:

- **`--help`** includes a `Body schema:` section with every field, type, required marker, description, and nesting
  level. `format` (e.g. `date-time`, `email`) and numeric bounds appear inline in the type column. Enum values list on a
  follow-up line.
- **`--schema`** prints the full resolved JSON Schema and exits without making an HTTP call. Pretty-printed by default;
  compact under `--agent` for piping into tools.

```bash
paddle customers create --help             # human-readable tree
paddle customers create --schema           # pretty JSON Schema
paddle --agent customers create --schema   # compact JSON Schema
```

List / get / delete commands don't take a body, so they don't expose `--schema`.

## Generated commands

The resource commands in `internal/generated/` are generated from Paddle's API specification, and updated through pull requests from Paddle.
If a generated command is wrong or missing, [open an issue](CONTRIBUTING.md).
