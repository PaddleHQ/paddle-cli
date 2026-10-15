package commands

import (
	"os"
	"strings"
	"testing"
)

func TestRootSetup_AgentEnvAutoDetect(t *testing.T) {
	allTriggerKeys := []string{"CI", "CLAUDECODE", "CURSOR_AGENT", "CODEX_CI", "GEMINI_CLI", "OPENCODE", "AI_AGENT", "AGENT"}
	cases := []struct {
		name   string
		envKey string
		envVal string
	}{
		{"CI", "CI", "1"},
		{"CLAUDECODE", "CLAUDECODE", "1"},
		{"CURSOR_AGENT", "CURSOR_AGENT", "1"},
		{"CODEX_CI", "CODEX_CI", "1"},
		{"GEMINI_CLI", "GEMINI_CLI", "1"},
		{"OPENCODE", "OPENCODE", "1"},
		{"AI_AGENT", "AI_AGENT", "claude-code_2-1-280_agent"},
		{"AIDER_FOO", "AIDER_FOO", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear all triggers including ambient AIDER_* vars.
			for _, k := range allTriggerKeys {
				t.Setenv(k, "")
			}
			for _, e := range os.Environ() {
				if strings.HasPrefix(e, "AIDER_") {
					key, _, _ := strings.Cut(e, "=")
					t.Setenv(key, "")
				}
			}
			t.Setenv("PADDLE_AGENT", "")
			// Now set only the key under test.
			t.Setenv(tc.envKey, tc.envVal)
			if !isAgentEnvEnabled() {
				t.Errorf("expected isAgentEnvEnabled()=true when %s=%s", tc.envKey, tc.envVal)
			}
		})
	}
}

func TestRootSetup_AgentEnvAutoDetect_NoneSet(t *testing.T) {
	for _, k := range []string{"CI", "CLAUDECODE", "CURSOR_AGENT", "CODEX_CI", "GEMINI_CLI", "OPENCODE", "AI_AGENT", "AGENT"} {
		t.Setenv(k, "")
	}
	// Clear any ambient AIDER_* variables that might be set in the test runner's environment.
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "AIDER_") {
			key, _, _ := strings.Cut(e, "=")
			t.Setenv(key, "")
		}
	}
	if isAgentEnvEnabled() {
		t.Error("expected isAgentEnvEnabled()=false when no agent env vars are set")
	}
}
