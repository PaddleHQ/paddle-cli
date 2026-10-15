package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
	"github.com/PaddleHQ/paddle-cli/internal/output"
	"github.com/PaddleHQ/paddle-cli/internal/style"
)

func TestWriteBanner_UsesEnvironmentColour(t *testing.T) {
	var buf bytes.Buffer
	output.WriteBanner(&buf, output.BannerOptions{
		Environment: configtest.TestEnv,
		StderrTTY:   true,
		Profile:     style.Enabled(),
	})
	got := buf.String()
	if !strings.Contains(got, "91") || !strings.Contains(got, "1;") {
		t.Errorf("expected bright-red bold environment name, got %q", got)
	}
	if !strings.Contains(got, "→ ") || !strings.Contains(got, configtest.TestEnv) {
		t.Errorf("expected arrow and %q in output, got %q", configtest.TestEnv, got)
	}
}

func TestWriteBanner_NonTTYIsPlain(t *testing.T) {
	for _, env := range []string{configtest.TestEnv, configtest.TestEnvAPIKeyOnly} {
		var buf bytes.Buffer
		output.WriteBanner(&buf, output.BannerOptions{Environment: env, StderrTTY: false})

		got := buf.String()
		if contains(got, "\x1b[") {
			t.Errorf("env=%s: expected no ANSI escapes when non-TTY, got %q", env, got)
		}
		if got != "→ "+env+"\n" {
			t.Errorf("env=%s: got %q, want %q", env, got, "→ "+env+"\n")
		}
	}
}

func TestWriteBanner_SuppressedWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	output.WriteBanner(&buf, output.BannerOptions{Environment: configtest.TestEnv, StderrTTY: true, Suppressed: true})
	if buf.Len() != 0 {
		t.Errorf("expected no output when Suppressed, got %q", buf.String())
	}
}

func TestWriteBanner_EmptyEnvironmentWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	output.WriteBanner(&buf, output.BannerOptions{Environment: "", StderrTTY: true})
	if buf.Len() != 0 {
		t.Errorf("expected no output when Environment empty, got %q", buf.String())
	}
}

func TestIsStderrTTY_DefaultUnderGoTest(t *testing.T) {
	// Mirrors TestIsStdoutTTY_DefaultUnderGoTest: under `go test` stderr
	// is also a pipe, so the default implementation must report false.
	if output.IsStderrTTY() {
		t.Error("expected IsStderrTTY() to be false under `go test`")
	}
}

// contains is a thin wrapper to keep test assertions readable.
func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
