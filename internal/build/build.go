// Package build holds the version details stamped into the binary at build time.
package build

// Set via -ldflags at build time.
var (
	Version = "dev"
	Commit  = "unknown"
)
