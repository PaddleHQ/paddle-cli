package schema_test

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	// lipgloss/termenv decides the active color profile from $TERM/$NO_COLOR
	// at first use. Pin it to ANSI so tests assert on stable byte sequences
	// and do not vary with the host shell or `go test` running without a TTY.
	lipgloss.SetColorProfile(termenv.ANSI)
	os.Exit(m.Run())
}
