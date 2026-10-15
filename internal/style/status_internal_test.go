package style

import (
	"strings"
	"testing"
)

// TestStatus_KnownValues asserts a representative sample of values from
// each family color correctly. Catches misclassification (e.g. paid in
// negative) that a per-map-iteration test would miss.
func TestStatus_KnownValues(t *testing.T) {
	p := Enabled()
	cases := []struct {
		value    string
		wantANSI string
	}{
		// Positive / live → green (ANSI 2)
		{"active", "\x1b[32m"},
		{"paid", "\x1b[32m"},
		{"completed", "\x1b[32m"},
		{"ready", "\x1b[32m"},
		{"delivered", "\x1b[32m"},
		{"granted", "\x1b[32m"},
		{"won", "\x1b[32m"},
		// Pending / transitional → yellow (ANSI 3)
		{"trialing", "\x1b[33m"},
		{"past_due", "\x1b[33m"},
		{"pending", "\x1b[33m"},
		{"billed", "\x1b[33m"},
		{"draft", "\x1b[33m"},
		{"in_progress", "\x1b[33m"},
		{"needs_retry", "\x1b[33m"},
		// Negative / terminal → red (ANSI 1)
		{"canceled", "\x1b[31m"},
		{"cancelled", "\x1b[31m"}, //nolint:misspell // an internal API enum value
		{"failed", "\x1b[31m"},
		{"archived", "\x1b[31m"},
		{"paused", "\x1b[31m"},
		{"expired", "\x1b[31m"},
		{"voided", "\x1b[31m"},
		{"lost", "\x1b[31m"},
		{"revoked", "\x1b[31m"},
	}
	for _, c := range cases {
		got := p.Status(c.value)
		if !strings.Contains(got, c.wantANSI) {
			t.Errorf("Status(%q): expected ANSI %q, got %q", c.value, c.wantANSI, got)
		}
		if !strings.Contains(got, c.value) {
			t.Errorf("Status(%q): output missing the value text: %q", c.value, got)
		}
	}
}

// TestStatus_EveryMapEntryIsColoured guarantees that whatever values are
// in statusColours actually render with an ANSI escape; a misclassified
// family value (e.g. statusFamily(99)) would show up here as plain text.
func TestStatus_EveryMapEntryIsColoured(t *testing.T) {
	p := Enabled()
	for value := range statusColours {
		got := p.Status(value)
		if !strings.ContainsRune(got, '\x1b') {
			t.Errorf("Status(%q): map entry rendered without ANSI: %q", value, got)
		}
	}
}

func TestStatus_UnknownValueIsPlain(t *testing.T) {
	p := Enabled()
	got := p.Status("frobbed")
	if strings.ContainsRune(got, '\x1b') {
		t.Errorf("Status(unknown): expected plain text, got %q", got)
	}
	if got != "frobbed" {
		t.Errorf("Status(unknown): expected %q, got %q", "frobbed", got)
	}
}

func TestStatus_CaseInsensitive(t *testing.T) {
	p := Enabled()
	got := p.Status("ACTIVE")
	if !strings.Contains(got, "\x1b[32m") {
		t.Errorf("Status(ACTIVE): expected green ANSI, got %q", got)
	}
}

func TestStatus_DisabledProfileIsIdentity(t *testing.T) {
	var p Profile
	if got := p.Status("active"); got != "active" {
		t.Errorf("disabled Status(active) = %q, want plain", got)
	}
}
