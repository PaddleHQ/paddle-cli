package output

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

// ParsePagination extracts has_more and next cursor from a Paddle API response body.
// Returns false, "" when the field is absent or the body is not a Paddle envelope.
func ParsePagination(data []byte) (hasMore bool, cursor string) {
	var env struct {
		Meta struct {
			Pagination struct {
				HasMore bool   `json:"has_more"`
				Next    string `json:"next"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return false, ""
	}
	cursor = env.Meta.Pagination.Next
	if u, err := url.Parse(cursor); err == nil && u.Scheme != "" {
		if after := u.Query().Get("after"); after != "" {
			cursor = after
		}
	}
	return env.Meta.Pagination.HasMore, cursor
}

// shellQuote wraps s in single quotes if it contains characters the shell
// would interpret specially (spaces, brackets, pipes, etc.). Embedded single
// quotes are escaped by closing the quoted string, adding \' and reopening it.
func shellQuote(s string) string {
	for _, r := range s {
		if !isShellSafe(r) {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

// isShellSafe reports whether r can appear unquoted in a shell word.
func isShellSafe(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("_-.,:/+=", r)
}

// FlagAnnotationSensitive marks a flag whose value must never be echoed back,
// such as a credential. SynthesizeNextCommand leaves these flags out.
const FlagAnnotationSensitive = "paddle_sensitive"

// SynthesizeNextCommand builds the command string to fetch the next page.
// It replays the changed flags on cmd (local and inherited), except --after,
// which is replaced by cursor, and flags annotated FlagAnnotationSensitive.
// Values containing shell metacharacters are single-quoted.
func SynthesizeNextCommand(cmd *cobra.Command, cursor string) string {
	parts := strings.Fields(cmd.CommandPath())
	cmd.Flags().VisitAll(func(f *flag.Flag) {
		if !f.Changed {
			return
		}
		if f.Name == "after" {
			return // replaced below
		}
		if _, ok := f.Annotations[FlagAnnotationSensitive]; ok {
			return
		}
		parts = append(parts, "--"+f.Name, shellQuote(f.Value.String()))
	})
	parts = append(parts, "--after", cursor)
	return strings.Join(parts, " ")
}

// WritePaginationHint writes the "More results available" hint to w.
// No-op when nextCmd is empty.
func WritePaginationHint(w io.Writer, nextCmd string) {
	if nextCmd == "" {
		return
	}
	_, _ = fmt.Fprintf(w, "More results available: %s\n", nextCmd)
}
