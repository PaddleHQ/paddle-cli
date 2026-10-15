package style

import (
	"github.com/tidwall/pretty"
)

const (
	sgrReset   = "\x1b[0m"
	sgrMagenta = "\x1b[35m"
)

// jsonStyle picks ANSI-16 colors for JSON tokens. Strings cyan,
// numbers yellow, booleans/null magenta-ish (ANSI 5), keys bright blue.
// Brackets and punctuation stay plain.
var jsonStyle = &pretty.Style{
	Key:      [2]string{"\x1b[94m", sgrReset},
	String:   [2]string{"\x1b[36m", sgrReset},
	Number:   [2]string{"\x1b[33m", sgrReset},
	True:     [2]string{sgrMagenta, sgrReset},
	False:    [2]string{sgrMagenta, sgrReset},
	Null:     [2]string{sgrMagenta, sgrReset},
	Escape:   [2]string{sgrMagenta, sgrReset},
	Brackets: [2]string{"", ""},
	Append:   func(dst []byte, c byte) []byte { return append(dst, c) },
}

// JSON returns input bytes with ANSI styling applied to JSON tokens.
// When the profile is disabled, returns input unchanged.
// Callers must NOT pass compact JSON intended for agents/pipes:
// gating happens at the call site (output.printJSON).
func (p Profile) JSON(b []byte) []byte {
	if !p.enabled {
		return b
	}
	return pretty.Color(b, jsonStyle)
}
