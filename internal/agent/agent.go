// Package agent detects whether paddle is being driven by an AI coding agent
// or CI, and which one.
package agent

import (
	"os"
	"strings"
)

// Other is reported when AI_AGENT names an agent that isn't in knownAgents.
const Other = "other"

const (
	claudeCode    = "claude-code"
	cursor        = "cursor"
	codex         = "codex"
	aider         = "aider"
	geminiCLI     = "gemini-cli"
	githubCopilot = "github-copilot"
	opencode      = "opencode"
)

// knownAgents maps names agents announce via AI_AGENT / AGENT to the name we
// report. Keeping the reported set fixed keeps downstream analytics bounded.
var knownAgents = map[string]string{
	claudeCode:           claudeCode,
	cursor:               cursor,
	"cursor-cli":         cursor,
	codex:                codex,
	aider:                aider,
	geminiCLI:            geminiCLI,
	githubCopilot:        githubCopilot,
	"github-copilot-cli": githubCopilot,
	"devin":              "devin",
	"goose":              "goose",
	"amp":                "amp",
	opencode:             opencode,
}

// Detect returns the name of the agent or automation running paddle, or ""
// when there's no sign of one.
func Detect() string {
	if v := os.Getenv("AI_AGENT"); v != "" {
		if name, ok := knownAgents[baseName(v)]; ok {
			return name
		}
		return Other
	}
	// AGENT is generic enough to be set by non-AI tooling, so only trust known names.
	if name, ok := knownAgents[baseName(os.Getenv("AGENT"))]; ok {
		return name
	}
	return detectFromMarkers()
}

// detectFromMarkers checks agent-specific environment variables, falling back
// to CI.
func detectFromMarkers() string {
	switch {
	case os.Getenv("CLAUDECODE") != "":
		return claudeCode
	case os.Getenv("CURSOR_AGENT") != "":
		return cursor
	case os.Getenv("CODEX_CI") != "":
		return codex
	case os.Getenv("GEMINI_CLI") != "":
		return geminiCLI
	case os.Getenv("OPENCODE") != "":
		return opencode
	case hasAiderEnv():
		return aider
	case os.Getenv("CI") != "":
		return "ci"
	}
	return ""
}

// baseName strips version suffixes such as "claude-code_2-1-280_agent" or
// "devin@1" down to the agent name.
func baseName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if i := strings.IndexAny(v, "_@/ "); i >= 0 {
		v = v[:i]
	}
	return v
}

func hasAiderEnv() bool {
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "AIDER_") {
			return true
		}
	}
	return false
}
