# Notes for agents

The [README](README.md) is the source of truth for build commands, architecture, package layout, design decisions, conventions, and outstanding work. **Read it first.** Everything below is agent-specific guidance that sits on top of the README; do not duplicate the README's content here.

## Hard rules

- **Never hand-edit files under `internal/generated/`.** See the README's [Generated commands](README.md#generated-commands) section.

## Where things live

When you need to work on…

- **A new hand-crafted command** → `internal/commands/`, registered in `cmd/paddle/main.go`.
- **Built-in help topics** (`paddle help <topic>`) → `internal/commands/help_topics.go`.
- **Request-body schema rendering in `--help` / `--schema`** → `internal/schema/`.
- **Output formatting (table / JSON / CSV, `--jq` filter)** → `internal/output/`.
- **HTTP client (auth headers, error parsing)** → `internal/client/`.
- **Config file + env var overrides** → `internal/config/`.

## Working style in this repo

- Test files sit next to source files (`foo.go` / `foo_test.go`). Follow the existing patterns: `httptest.NewServer` for HTTP, `t.TempDir()` for config files, `t.Setenv()` for env vars.
- Errors wrap with `fmt.Errorf("...: %w", err)`.
