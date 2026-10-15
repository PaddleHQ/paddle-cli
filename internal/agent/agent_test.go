package agent_test

import (
	"os"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/agent"
)

// clearAgentEnv unsets every variable Detect reads, including ambient ones
// from the harness running the tests (e.g. CLAUDECODE under Claude Code).
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"AI_AGENT", "AGENT", "CLAUDECODE", "CURSOR_AGENT", "CODEX_CI", "GEMINI_CLI", "OPENCODE", "CI"} {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unsetting %s: %v", k, err)
		}
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "AIDER_") {
			key, _, _ := strings.Cut(e, "=")
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unsetting %s: %v", key, err)
			}
		}
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"nothing set", nil, ""},
		{"claude code", map[string]string{"CLAUDECODE": "1"}, "claude-code"},
		{"cursor", map[string]string{"CURSOR_AGENT": "1"}, "cursor"},
		{"codex", map[string]string{"CODEX_CI": "1"}, "codex"},
		{"gemini cli", map[string]string{"GEMINI_CLI": "1"}, "gemini-cli"},
		{"opencode sets OPENCODE and a generic AGENT=1", map[string]string{"OPENCODE": "1", "AGENT": "1"}, "opencode"},
		{"aider config var", map[string]string{"AIDER_MODEL": "gpt"}, "aider"},
		{"ci", map[string]string{"CI": "true"}, "ci"},
		{"AI_AGENT with version suffix", map[string]string{"AI_AGENT": "claude-code_2-1-280_agent"}, "claude-code"},
		{"AI_AGENT with @version", map[string]string{"AI_AGENT": "devin@1"}, "devin"},
		{"AI_AGENT alias", map[string]string{"AI_AGENT": "cursor-cli"}, "cursor"},
		{"AI_AGENT unknown", map[string]string{"AI_AGENT": "my-homegrown-bot@2.0"}, "other"},
		{"AGENT known", map[string]string{"AGENT": "goose"}, "goose"},
		{"AGENT unknown is ignored", map[string]string{"AGENT": "jenkins-node-7"}, ""},
		{"AI_AGENT wins over CI", map[string]string{"AI_AGENT": "codex", "CI": "true"}, "codex"},
		{"named agent wins over CI", map[string]string{"CLAUDECODE": "1", "CI": "true"}, "claude-code"},
		{"CI with unknown AGENT", map[string]string{"AGENT": "jenkins-node-7", "CI": "true"}, "ci"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAgentEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := agent.Detect(); got != tc.want {
				t.Errorf("Detect() = %q, want %q", got, tc.want)
			}
		})
	}
}
