package generated_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestGeneratedCommands_UseLinesIncludePositionalArgs walks every Go
// source file in internal/generated/ and asserts that for each
// cobra.ExactArgs(N) (with N>0) there is a preceding `Use:`
// string literal containing exactly N angle-bracket placeholders.
//
// Regression guard: the codegen template's Use line must
// include `<param>` for every PathParam so `--help` shows the required
// positional arg. Hand-crafted commands live in internal/commands/ and
// are not covered by this test - they already follow the convention.
//
// Structural assumption: the generator emits one cobra.Command per
// operation block, with `Use:` immediately preceding `Args:`. The
// regex-based scan pairs each ExactArgs / MinimumNArgs call with the
// nearest preceding `Use:` literal; if a future template restructure
// (shared scaffolding, helper vars, etc.) puts another `Use:` in
// between, the pairing will be wrong. The Use: regex also assumes the
// literal is a single double-quoted string - a future move to string
// concatenation would silently produce no `Use:` match and trigger the
// "no preceding Use: line" failure mode (loud, debuggable).
func TestGeneratedCommands_UseLinesIncludePositionalArgs(t *testing.T) {
	useRe := regexp.MustCompile(`Use:\s*"([^"]+)"`)
	exactRe := regexp.MustCompile(`cobra\.ExactArgs\((\d+)\)`)
	minRe := regexp.MustCompile(`cobra\.MinimumNArgs\((\d+)\)`)
	angleRe := regexp.MustCompile(`<[^>]+>`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading generated dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(".", entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		src := string(data)

		// Walk all ExactArgs / MinimumNArgs occurrences. For each,
		// find the nearest preceding Use: line and count placeholders.
		for _, match := range collectArgMatches(src, exactRe, minRe) {
			use := nearestPrecedingUse(src, match.offset, useRe)
			if use == "" {
				t.Errorf("%s: %s at offset %d has no preceding Use: line", path, match.kind, match.offset)
				continue
			}
			want := match.n
			got := len(angleRe.FindAllString(use, -1))
			if got != want {
				t.Errorf("%s: Use=%q paired with %s(%d) - found %d <…> placeholders, want %d",
					path, use, match.kind, match.n, got, want)
			}
		}
	}
}

type argMatch struct {
	kind   string
	n      int
	offset int
}

func collectArgMatches(src string, exactRe, minRe *regexp.Regexp) []argMatch {
	var out []argMatch
	for _, m := range exactRe.FindAllStringSubmatchIndex(src, -1) {
		n, _ := strconv.Atoi(src[m[2]:m[3]])
		if n > 0 {
			out = append(out, argMatch{kind: "ExactArgs", n: n, offset: m[0]})
		}
	}
	for _, m := range minRe.FindAllStringSubmatchIndex(src, -1) {
		n, _ := strconv.Atoi(src[m[2]:m[3]])
		if n > 0 {
			out = append(out, argMatch{kind: "MinimumNArgs", n: n, offset: m[0]})
		}
	}
	return out
}

func nearestPrecedingUse(src string, before int, useRe *regexp.Regexp) string {
	matches := useRe.FindAllStringSubmatchIndex(src[:before], -1)
	if len(matches) == 0 {
		return ""
	}
	last := matches[len(matches)-1]
	return src[last[2]:last[3]]
}
