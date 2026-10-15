package style_test

import (
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
	"github.com/PaddleHQ/paddle-cli/internal/style"
)

// The zero value MUST be disabled: every helper is the identity function.
// This is what lets us migrate output.Options.NoColor → Profile without
// changing every test that constructs Options{} by struct literal.
func TestProfile_ZeroValueIsDisabled(t *testing.T) {
	var p style.Profile
	cases := []struct {
		name string
		got  string
	}{
		{"Error", p.Error("boom")},
		{"Warn", p.Warn("careful")},
		{"Success", p.Success("ok")},
		{"Info", p.Info("fyi")},
		{"Dim", p.Dim("muted")},
		{"Bold", p.Bold("loud")},
		{"Prompt", p.Prompt("?")},
		{"FieldName", p.FieldName("amount")},
		{"ID", p.ID("pro_01")},
		{"EnvName", p.EnvName(configtest.TestEnv)},
	}
	for _, c := range cases {
		if strings.ContainsRune(c.got, '\x1b') {
			t.Errorf("%s: zero-value Profile produced ANSI escapes: %q", c.name, c.got)
		}
	}
}

func TestProfile_Enabled_EmitsANSI(t *testing.T) {
	p := style.Enabled()
	for _, c := range []struct {
		name string
		got  string
	}{
		{"Error", p.Error("boom")},
		{"Warn", p.Warn("careful")},
		{"Success", p.Success("ok")},
		{"Info", p.Info("fyi")},
		{"Dim", p.Dim("muted")},
		{"Bold", p.Bold("loud")},
		{"Prompt", p.Prompt("?")},
		{"FieldName", p.FieldName("amount")},
		{"ID", p.ID("pro_01")},
	} {
		if !strings.ContainsRune(c.got, '\x1b') {
			t.Errorf("%s: enabled Profile produced no ANSI escape: %q", c.name, c.got)
		}
	}
}

func TestProfile_EnvName_ZeroValue(t *testing.T) {
	var p style.Profile
	for _, env := range []string{configtest.TestEnv, configtest.TestEnvAPIKeyOnly, "unknown", ""} {
		if got := p.EnvName(env); got != env {
			t.Errorf("zero-value Profile.EnvName(%q) = %q, want %q", env, got, env)
		}
	}
}

// Colors come from each environment's config: test_env is bright red (91)
// and bold, test_env_apikey_only is yellow (33).
func TestProfile_EnvName_Enabled(t *testing.T) {
	p := style.Enabled()

	if got := p.EnvName(configtest.TestEnv); !strings.Contains(got, "91") || !strings.Contains(got, "1;") {
		t.Errorf("EnvName(test_env) = %q, want bright red and bold", got)
	}
	if got := p.EnvName(configtest.TestEnvAPIKeyOnly); !strings.Contains(got, "33m") || strings.Contains(got, "1;") {
		t.Errorf("EnvName(test_env_apikey_only) = %q, want yellow, not bold", got)
	}
	if got, want := p.EnvName("unknown"), p.Dim("unknown"); got != want {
		t.Errorf("EnvName(unknown) = %q, want dimmed %q", got, want)
	}
	if got := p.EnvName(""); strings.ContainsRune(got, '\x1b') {
		t.Errorf("empty EnvName: expected no ANSI escape, got %q", got)
	}
}
