// Package style centralizes every ANSI styling decision in the CLI.
// Lipgloss is imported only here; other packages use the semantic helpers
// on Profile so color choices live in one place.
package style

import (
	"github.com/PaddleHQ/paddle-cli/internal/config"

	"github.com/charmbracelet/lipgloss"
)

// Profile encapsulates the active styling mode for one CLI invocation.
// The zero value is disabled: every helper returns input unchanged.
// Callers building an Options or Root that hasn't been threaded through
// Resolve get safe, plain output by default.
type Profile struct {
	enabled  bool
	renderer *lipgloss.Renderer
}

// Enabled returns a Profile that emits ANSI-16 styling.
// Used by tests; production code goes through Resolve.
func Enabled() Profile {
	return Profile{enabled: true, renderer: lipgloss.DefaultRenderer()}
}

// IsEnabled reports whether this profile emits styling.
func (p Profile) IsEnabled() bool { return p.enabled }

// ANSI-16 color names, referenced via lipgloss.Color so user terminal
// themes apply. Numbers correspond to ANSI color codes 0-15.
const (
	ansiRed           = "1"
	ansiGreen         = "2"
	ansiYellow        = "3"
	ansiCyan          = "6"
	ansiBrightRed     = "9"
	ansiBrightBlue    = "12"
	ansiBrightMagenta = "13"
)

func (p Profile) style() lipgloss.Style {
	if p.renderer != nil {
		return p.renderer.NewStyle()
	}
	return lipgloss.NewStyle()
}

func (p Profile) apply(s string, fn func(lipgloss.Style) lipgloss.Style) string {
	if !p.enabled {
		return s
	}
	return fn(p.style()).Render(s)
}

// Error renders a message that signals a failure the user must address;
// bright red and bold to draw the eye.
func (p Profile) Error(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Foreground(lipgloss.Color(ansiBrightRed)).Bold(true)
	})
}

// Warn renders a non-fatal advisory the user should notice; yellow.
func (p Profile) Warn(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Foreground(lipgloss.Color(ansiYellow))
	})
}

// Success renders confirmation that an operation completed; green.
func (p Profile) Success(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Foreground(lipgloss.Color(ansiGreen))
	})
}

// Info renders neutral context: tips, doc URLs, and the like; cyan.
func (p Profile) Info(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Foreground(lipgloss.Color(ansiCyan))
	})
}

// Dim renders metadata, hint text, and other content the eye should pass
// over on the way to something more important; faint.
func (p Profile) Dim(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Faint(true)
	})
}

// Bold emphasizes content without coloring it (typographic-only emphasis).
func (p Profile) Bold(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Bold(true)
	})
}

// Prompt renders an interactive input prefix (e.g. "?", "Enter code:");
// cyan and bold so it stands out from surrounding output.
func (p Profile) Prompt(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Foreground(lipgloss.Color(ansiCyan)).Bold(true)
	})
}

// FieldName renders a structural label such as a JSON Schema property
// name in --schema output; bright blue.
func (p Profile) FieldName(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Foreground(lipgloss.Color(ansiBrightBlue))
	})
}

// Bool renders a boolean string value: "true" in green, "false" in red.
func (p Profile) Bool(s string) string {
	switch s {
	case "true":
		return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
			return st.Foreground(lipgloss.Color(ansiGreen))
		})
	case "false":
		return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
			return st.Foreground(lipgloss.Color(ansiRed))
		})
	}
	return s
}

// Email renders an email address field; magenta.
func (p Profile) Email(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Foreground(lipgloss.Color(ansiBrightMagenta))
	})
}

// ID renders identifier strings (e.g. "pro_01h..."); cyan.
func (p Profile) ID(s string) string {
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		return st.Foreground(lipgloss.Color(ansiCyan))
	})
}

// EnvName renders an environment name in the color its configuration
// declares; unknown or uncoloured environments render dimmed.
func (p Profile) EnvName(s string) string {
	if s == "" || !p.enabled {
		return s
	}
	e, ok := config.LookupEnvironment(s)
	if !ok || e.Color == 0 {
		return p.Dim(s)
	}
	return p.apply(s, func(st lipgloss.Style) lipgloss.Style {
		st = st.Foreground(lipgloss.ANSIColor(e.Color))
		if e.Bold {
			st = st.Bold(true)
		}
		return st
	})
}
