package output

import "os"

// IsStdoutTTY reports whether os.Stdout is connected to an interactive
// terminal. It's a package-level var so tests can override it without
// mucking with file descriptors. The default implementation uses
// os.ModeCharDevice, which is correct on macOS and Linux (the only
// platforms we currently target).
var IsStdoutTTY = func() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
