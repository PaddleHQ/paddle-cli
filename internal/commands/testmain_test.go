package commands_test

import (
	"os"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	// Clear env vars that trigger agent mode so individual tests control it explicitly.
	for _, k := range []string{"CI", "CLAUDECODE", "CURSOR_AGENT", "CODEX_CI", "GEMINI_CLI", "OPENCODE", "AI_AGENT", "AGENT", "PADDLE_AGENT", "PADDLE_API_KEY", "PADDLE_API_URL"} {
		if err := os.Unsetenv(k); err != nil {
			panic(err)
		}
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "AIDER_") {
			key, _, _ := strings.Cut(e, "=")
			if err := os.Unsetenv(key); err != nil {
				panic(err)
			}
		}
	}

	// Pin lipgloss to the ANSI profile so color assertions in tests are
	// stable regardless of $TERM and the absence of a TTY under `go test`.
	lipgloss.SetColorProfile(termenv.ANSI)
	configtest.Install()
	os.Exit(m.Run())
}
