package output

import (
	"fmt"
	"io"
	"os"

	"github.com/PaddleHQ/paddle-cli/internal/style"
)

// IsStderrTTY reports whether os.Stderr is connected to an interactive
// terminal. Mirrors IsStdoutTTY; a package var so tests can override.
var IsStderrTTY = func() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// BannerOptions controls a single environment-banner write.
type BannerOptions struct {
	Environment string        // resolved environment name (e.g. "sandbox", "production")
	Suppressed  bool          // when true, WriteBanner is a no-op
	StderrTTY   bool          // whether stderr is a TTY; false forces plain text
	Profile     style.Profile // color profile; zero value disables color
}

// WriteBanner writes the one-line stderr footer announcing the active
// environment. The format is "→ <env>". Color is applied when StderrTTY is
// true and the Profile has color enabled; otherwise plain text is emitted.
func WriteBanner(w io.Writer, opts BannerOptions) {
	if opts.Suppressed || opts.Environment == "" {
		return
	}
	if !opts.StderrTTY {
		_, _ = fmt.Fprintf(w, "→ %s\n", opts.Environment)
		return
	}
	_, _ = fmt.Fprintf(w, "→ %s\n", opts.Profile.EnvName(opts.Environment))
}
