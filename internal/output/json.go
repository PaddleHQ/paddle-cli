package output

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

// printJSON renders data as pretty-printed JSON, or applies a jq filter if set.
// With opts.Compact the output is single-line to save bytes for agent
// consumers, and never colored.
//
// Side-effect: when opts.Environment is non-empty and the document is a JSON
// object, meta.environment is set to opts.Environment (creating meta if
// absent). Array bodies and primitives pass through unchanged.
func printJSON(data []byte, opts Options) error {
	data = injectMetaEnvironment(data, opts.Environment)
	data = injectNextCommand(data, opts.NextCommand)

	if opts.JQ != "" {
		return printJSONWithJQ(data, opts)
	}

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return writeJSON(opts, v)
}

// writeJSON encodes v using the formatting rules of opts and applies
// syntax highlighting when the profile is enabled and output is not
// compact. Compact output is for machine consumers and stays plain.
func writeJSON(opts Options, v any) error {
	if opts.Compact || !opts.Profile.IsEnabled() {
		if err := newJSONEncoder(opts).Encode(v); err != nil {
			return fmt.Errorf("encoding JSON: %w", err)
		}
		return nil
	}
	// Encode to a buffer, colourise, then write. This keeps the
	// existing newline behavior (Encoder appends one).
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	if _, err := opts.Writer.Write(opts.Profile.JSON(buf.Bytes())); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

// injectMetaEnvironment ensures meta.environment is set to env on object
// bodies. Arrays and primitives are returned unchanged. Invalid JSON also
// passes through (caller's existing decode will report it).
func injectMetaEnvironment(data []byte, env string) []byte {
	if env == "" {
		return data
	}
	doc, ok := parseObject(data)
	if !ok {
		return data
	}
	meta := subObject(doc, "meta")
	if !setJSON(meta, "environment", env) || !setJSON(doc, "meta", meta) {
		return data
	}
	return marshalOr(doc, data)
}

// injectNextCommand sets meta.pagination.next_command on JSON object bodies.
// No-op when nextCmd is empty or the body is not a JSON object.
func injectNextCommand(data []byte, nextCmd string) []byte {
	if nextCmd == "" {
		return data
	}
	doc, ok := parseObject(data)
	if !ok {
		return data
	}
	meta := subObject(doc, "meta")
	pagination := subObject(meta, "pagination")
	if !setJSON(pagination, "next_command", nextCmd) ||
		!setJSON(meta, "pagination", pagination) ||
		!setJSON(doc, "meta", meta) {
		return data
	}
	return marshalOr(doc, data)
}

// parseObject decodes data as a JSON object, reporting false for any other
// shape or invalid JSON.
func parseObject(data []byte) (map[string]json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false
	}
	return doc, true
}

// subObject returns parent[key] decoded as an object, or an empty map when the
// key is absent, null or not an object, so callers can always write into it.
func subObject(parent map[string]json.RawMessage, key string) map[string]json.RawMessage {
	if existing, ok := parent[key]; ok {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(existing, &m); err == nil && m != nil {
			return m
		}
	}
	return make(map[string]json.RawMessage)
}

// setJSON marshals v into m[key], reporting false if marshaling fails.
func setJSON(m map[string]json.RawMessage, key string, v any) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	m[key] = b
	return true
}

// marshalOr marshals v, returning fallback if that fails.
func marshalOr(v any, fallback []byte) []byte {
	out, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return out
}

// newJSONEncoder builds an encoder matching the caller's compactness preference.
// Non-compact output uses two-space indentation; HTML escaping is always off.
func newJSONEncoder(opts Options) *json.Encoder {
	enc := json.NewEncoder(opts.Writer)
	enc.SetEscapeHTML(false)
	if !opts.Compact {
		enc.SetIndent("", "  ")
	}
	return enc
}

// printJSONWithJQ applies a jq expression to data and writes results.
// String results are written directly; other types are written as indented JSON.
func printJSONWithJQ(data []byte, opts Options) error {
	query, err := gojq.Parse(opts.JQ)
	if err != nil {
		return fmt.Errorf("invalid jq expression %q: %w", opts.JQ, err)
	}

	var input any
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	code, err := gojq.Compile(query)
	if err != nil {
		return fmt.Errorf("failed to compile jq expression: %w", err)
	}

	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return fmt.Errorf("jq error: %w", err)
		}
		switch val := v.(type) {
		case string:
			_, _ = fmt.Fprintln(opts.Writer, val)
		default:
			if err := writeJSON(opts, val); err != nil {
				return err
			}
		}
	}
	return nil
}
