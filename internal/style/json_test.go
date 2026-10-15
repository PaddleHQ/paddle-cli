package style_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/style"
)

func TestJSON_DisabledIsIdentity(t *testing.T) {
	var p style.Profile
	in := []byte(`{"id":"pro_01","amount":100}`)
	got := p.JSON(in)
	if !bytes.Equal(got, in) {
		t.Errorf("disabled JSON should be identity; got %q want %q", got, in)
	}
}

func TestJSON_EnabledColoursKeysAndValues(t *testing.T) {
	p := style.Enabled()
	in := []byte(`{"id":"pro_01","amount":100,"active":true}`)
	got := string(p.JSON(in))
	if !strings.ContainsRune(got, '\x1b') {
		t.Fatalf("expected ANSI escapes, got %q", got)
	}
	for _, want := range []string{"id", "pro_01", "amount", "100", "active", "true"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got %q", want, got)
		}
	}
}
