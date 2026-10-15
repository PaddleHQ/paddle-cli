package output_test

import (
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/output"
)

func TestIsStdoutTTY_Override(t *testing.T) {
	original := output.IsStdoutTTY
	t.Cleanup(func() { output.IsStdoutTTY = original })
	output.IsStdoutTTY = func() bool { return true }
	if !output.IsStdoutTTY() {
		t.Error("override returning true was not honored")
	}
	output.IsStdoutTTY = func() bool { return false }
	if output.IsStdoutTTY() {
		t.Error("override returning false was not honored")
	}
}

func TestIsStdoutTTY_DefaultUnderGoTest(t *testing.T) {
	// `go test` redirects stdout to a pipe, so the default implementation
	// must report false here. This guards us against a regression where
	// someone hard-codes true.
	if output.IsStdoutTTY() {
		t.Error("expected IsStdoutTTY() to be false under `go test` (stdout is a pipe)")
	}
}
