package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"sort"
	"strings"

	"github.com/PaddleHQ/paddle-cli/internal/style"
)

// JSON Schema type names that the renderer branches on or emits.
const (
	typeAny    = "any"
	typeArray  = "array"
	typeNull   = "null"
	typeObject = "object"
)

// RenderTree parses schemaJSON (a JSON Schema) and returns an indented,
// human-readable representation suitable for inclusion in cobra's
// cmd.Long. Returns an empty string if schemaJSON is empty or malformed.
//
// profile controls ANSI styling. The zero-value Profile produces plain
// output; an enabled Profile colors field names (FieldName), type info
// (Dim), and the required marker (Warn). Free-text descriptions stay
// plain; they're already legible without color.
func RenderTree(schemaJSON string, profile style.Profile) string {
	if schemaJSON == "" {
		return ""
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(schemaJSON), &root); err != nil {
		return ""
	}
	var sb strings.Builder
	renderObject(&sb, root, 1, profile)
	return sb.String()
}

// row is the intermediate form for one displayed line at a given
// depth: name + type + required + description columns, plus an
// optional follow-up closure that emits nested content (enum values,
// recursive-ref placeholder, child properties, combinator variants).
// Collecting rows before emitting lets renderObject compute column
// widths up front, so every sibling aligns no matter how much nested
// content sits between them.
type row struct {
	name  string
	typ   string
	req   string
	desc  string
	after func(io.Writer)
}

// renderObject writes each property in schema (which must be an
// object schema) as an aligned row at the given indent depth.
// Properties are sorted required-first then alphabetical.
// additionalProperties (if present) is emitted as a trailing
// pseudo-row after the named fields.
//
// Rows are collected first, their column widths computed, then every
// row is printed with manual padding, no tabwriter. This scopes
// alignment to each object's direct children without being sensitive
// to nested content or flush points.
func renderObject(w io.Writer, schema map[string]any, depth int, profile style.Profile) {
	props, _ := schema["properties"].(map[string]any)
	required := requiredSet(schema)
	names := sortedFieldNames(props, required)

	rows := make([]row, 0, len(names)+1)
	for _, name := range names {
		field, _ := props[name].(map[string]any)
		rows = append(rows, buildRow(name, field, required[name], depth, profile))
	}

	if r, ok := additionalPropertiesRow(schema, strings.Repeat("  ", depth)); ok {
		rows = append(rows, r)
	}

	nameW, typeW, reqW := columnWidths(rows)

	for _, r := range rows {
		// Pad against the unstyled lengths first so siblings align, then
		// wrap each column in its semantic style. ANSI escapes have zero
		// display width, so coloring the padded run keeps the layout.
		// Trailing spaces inside a styled run carry no foreground color
		// in any terminal we care about.
		nameCol := profile.FieldName(padRight(r.name, nameW))
		typeCol := profile.Dim(padRight(r.typ, typeW))
		reqCol := profile.Warn(padRight(r.req, reqW))
		line := nameCol + "  " + typeCol + "  " + reqCol + "  " + r.desc
		_, _ = fmt.Fprintln(w, strings.TrimRight(line, " "))
		if r.after != nil {
			r.after(w)
		}
	}
}

// sortedFieldNames orders required fields first, then alphabetically within
// each group, so the fields an agent must supply sit at the top of the tree.
func sortedFieldNames(props map[string]any, required map[string]bool) []string {
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	sort.SliceStable(names, func(i, j int) bool {
		ri, rj := required[names[i]], required[names[j]]
		if ri != rj {
			return ri
		}
		return names[i] < names[j]
	})
	return names
}

// additionalPropertiesRow returns the trailing pseudo-row for a schema's
// additionalProperties, if it allows extra keys.
func additionalPropertiesRow(schema map[string]any, indent string) (row, bool) {
	t := ""
	if ap, ok := schema["additionalProperties"].(map[string]any); ok {
		t = typeName(ap)
	} else if b, ok := schema["additionalProperties"].(bool); !ok || !b {
		return row{}, false
	}
	if t == "" {
		t = typeAny
	}
	return row{
		name: indent + "<additional properties>",
		typ:  t,
		desc: "(any extra keys allowed)",
	}, true
}

// columnWidths returns the widest name, type and required column across
// rows, which are one object's direct children only.
func columnWidths(rows []row) (nameW, typeW, reqW int) {
	for _, r := range rows {
		nameW = max(nameW, len(r.name))
		typeW = max(typeW, len(r.typ))
		reqW = max(reqW, len(r.req))
	}
	return nameW, typeW, reqW
}

// buildRow returns the displayed row for one field plus, when needed,
// an after-closure that writes nested content below the row (enum
// values, recursive-ref placeholder, child properties, combinator
// variants). The field is not rendered by buildRow itself; that
// happens in renderObject's emit loop after widths are known.
func buildRow(name string, field map[string]any, isRequired bool, depth int, profile style.Profile) row {
	indent := strings.Repeat("  ", depth)
	req := ""
	if isRequired {
		req = "required"
	}

	// Sentinel: raw $ref left by the parser's cycle protection.
	if refTarget, ok := refName(field); ok {
		return row{
			name:  indent + name,
			typ:   typeObject,
			req:   req,
			after: recursiveRefAfter(indent, refTarget),
		}
	}

	// Collapse anyOf/oneOf with exactly two variants where one is null
	// into the non-null variant, which mirrors the type-array nullable
	// encoding and cuts the noisy variant block Paddle specs use for
	// every nullable field.
	field = collapseNullableCombinator(field)

	// Combinator: render as "<name> oneOf" header with each variant
	// expanded below.
	if combinator, variants := pickCombinator(field); combinator != "" {
		desc := collapseWhitespace(stringField(field, "description"))
		return row{
			name: indent + name,
			typ:  combinator,
			req:  req,
			desc: desc,
			after: func(w io.Writer) {
				renderVariants(w, variants, indent+"  ", depth+2, profile)
			},
		}
	}

	typ := renderType(field)
	desc := collapseWhitespace(stringField(field, "description"))
	r := row{
		name: indent + name,
		typ:  typ,
		req:  req,
		desc: desc,
	}

	// Enum: field row plus a values line. Enum is a leaf, with no further
	// recursion even if the schema pathologically declares object or
	// array shape on the same node.
	if vals := enumValues(field); len(vals) > 0 {
		r.after = func(w io.Writer) {
			_, _ = fmt.Fprintf(w, "%s  values: %s\n", indent, strings.Join(vals, " | "))
		}
		return r
	}

	// Object / array recursion. Skip the after-closure entirely when
	// there's nothing to render below the row.
	switch typeName(field) {
	case typeObject:
		if hasRenderableContent(field) {
			r.after = func(w io.Writer) {
				renderObject(w, field, depth+1, profile)
			}
		}
	case typeArray:
		if items, ok := field["items"].(map[string]any); ok {
			r.after = arrayItemsAfter(items, indent, depth, profile)
		}
	}

	return r
}

// recursiveRefAfter writes the placeholder for a $ref left by the parser's
// cycle protection.
func recursiveRefAfter(indent, refTarget string) func(io.Writer) {
	return func(w io.Writer) {
		_, _ = fmt.Fprintf(w, "%s  (recursive reference to %s)\n", indent, refTarget)
	}
}

// arrayItemsAfter returns the closure that renders an array's items schema
// below its row, or nil when the items need nothing beyond the type column.
func arrayItemsAfter(items map[string]any, indent string, depth int, profile style.Profile) func(io.Writer) {
	if refTarget, isRef := refName(items); isRef {
		return recursiveRefAfter(indent, refTarget)
	}
	if typeName(items) == typeObject && hasRenderableContent(items) {
		return func(w io.Writer) {
			renderObject(w, items, depth+1, profile)
		}
	}
	combinator, variants := pickCombinator(items)
	if combinator == "" {
		return nil
	}
	return func(w io.Writer) {
		_, _ = fmt.Fprintf(w, "%s  items %s\n", indent, combinator)
		renderVariants(w, variants, indent+"    ", depth+3, profile)
	}
}

// renderVariants writes a "variant N (<type>)" header at labelIndent for each
// combinator branch, followed by its properties at depth.
func renderVariants(w io.Writer, variants []any, labelIndent string, depth int, profile style.Profile) {
	for i, v := range variants {
		variantMap, _ := v.(map[string]any)
		_, _ = fmt.Fprintf(w, "%svariant %d (%s)\n", labelIndent, i+1, variantTypeLabel(variantMap))
		renderObject(w, variantMap, depth, profile)
	}
}

// padRight pads s on the right with spaces up to width. Assumes
// schema names, type labels, and the "required" marker are all ASCII
// (true for every Paddle schema), so len() is the correct width.
func padRight(s string, width int) string {
	n := width - len(s)
	if n <= 0 {
		return s
	}
	return s + strings.Repeat(" ", n)
}

// hasRenderableContent reports whether an object schema has any
// children that renderObject would actually emit: either named
// properties or a usable additionalProperties block. Empty objects
// are common after the nullable-anyOf collapse ({"type": "object"}
// with no further detail).
func hasRenderableContent(field map[string]any) bool {
	if props, ok := field["properties"].(map[string]any); ok && len(props) > 0 {
		return true
	}
	if _, ok := field["additionalProperties"].(map[string]any); ok {
		return true
	}
	if b, ok := field["additionalProperties"].(bool); ok && b {
		return true
	}
	return false
}

// collapseNullableCombinator folds an anyOf/oneOf of exactly two
// variants where one is {"type": "null"} into the non-null variant's
// schema, carrying the outer field's description forward if the
// variant doesn't supply its own. Returns the original field
// unchanged when no collapse applies.
func collapseNullableCombinator(field map[string]any) map[string]any {
	for _, key := range []string{"anyOf", "oneOf"} {
		arr, ok := field[key].([]any)
		if !ok || len(arr) != 2 {
			continue
		}
		if nonNull, ok := nullableVariant(arr); ok {
			return mergeNullable(field, nonNull)
		}
	}
	return field
}

// nullableVariant returns the non-null variant when exactly one of variants
// is {"type": "null"} and another is a schema.
func nullableVariant(variants []any) (map[string]any, bool) {
	var nonNull map[string]any
	nullCount := 0
	for _, v := range variants {
		m, _ := v.(map[string]any)
		if m == nil {
			continue
		}
		if typeName(m) == typeNull {
			nullCount++
		} else {
			nonNull = m
		}
	}
	return nonNull, nullCount == 1 && nonNull != nil
}

// mergeNullable copies nonNull, inheriting field's description when the
// variant has none.
func mergeNullable(field, nonNull map[string]any) map[string]any {
	merged := make(map[string]any, len(nonNull)+1)
	maps.Copy(merged, nonNull)
	if _, hasDesc := merged["description"]; !hasDesc {
		if d, ok := field["description"].(string); ok {
			merged["description"] = d
		}
	}
	// Mark the merged schema as nullable via the type-array form
	// so renderType can show "X|null", the same signal as the spec's
	// native `type: ["X","null"]` nullable encoding.
	if innerT := typeName(nonNull); innerT != "" {
		merged["type"] = []any{innerT, typeNull}
	}
	return merged
}

// pickCombinator returns ("oneOf", variants) or ("anyOf", variants) if
// the schema node uses one of those combinators, else ("", nil). oneOf
// is preferred when both are present (unlikely in practice).
func pickCombinator(field map[string]any) (combinator string, variants []any) {
	if arr, ok := field["oneOf"].([]any); ok && len(arr) > 0 {
		return "oneOf", arr
	}
	if arr, ok := field["anyOf"].([]any); ok && len(arr) > 0 {
		return "anyOf", arr
	}
	return "", nil
}

// variantTypeLabel returns the type name of a oneOf/anyOf branch for use
// in the "variant N (<type>)" header, defaulting to "object" when the
// branch is an object-shaped schema.
func variantTypeLabel(variant map[string]any) string {
	if t := typeName(variant); t != "" {
		return t
	}
	if _, ok := variant["properties"].(map[string]any); ok {
		return typeObject
	}
	return typeAny
}

// enumValues returns the schema's enum values as strings (in schema order).
// Returns nil if not an enum or the enum is empty.
func enumValues(field map[string]any) []string {
	arr, ok := field["enum"].([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		out = append(out, fmt.Sprintf("%v", v))
	}
	return out
}

// renderType produces the string that goes in the "type" column for a
// single schema node. Nullable types (e.g. ["string","null"]) append
// "|null" to the non-null type: both the native type-array form and
// the collapsed-from-anyOf form carry the same signal. Returns "" for
// schemas without a discoverable type (bare-enum schemas,
// combinator-only schemas) so callers can layer their own labels on
// top without a silent "string" fallback.
func renderType(field map[string]any) string {
	if _, hasEnum := field["enum"]; hasEnum {
		return "enum"
	}
	t := typeName(field)
	if t == "" {
		return ""
	}
	t, suffix := typeLabelAndSuffix(t, field)
	if isNullable(field) {
		t += "|null"
	}
	return t + suffix
}

// typeLabelAndSuffix returns the type label for t and the trailing format or
// bounds annotation, e.g. ("array[string]", " [1..5]").
func typeLabelAndSuffix(t string, field map[string]any) (label, suffix string) {
	switch t {
	case "integer", "number":
		if f, ok := field["format"].(string); ok && f != "" {
			return t, " (" + f + ")"
		}
		if s := numericBoundsSuffix(field); s != "" {
			return t, " " + s
		}
	case typeArray:
		elem, _ := field["items"].(map[string]any)
		label = "array[" + elementType(elem) + "]"
		if s := arrayBoundsSuffix(field); s != "" {
			return label, " " + s
		}
		return label, ""
	default:
		if f, ok := field["format"].(string); ok && f != "" {
			return t, " (" + f + ")"
		}
	}
	return t, ""
}

// isNullable reports whether the schema node declares a null type
// alongside its main type, either via `type: ["X","null"]` natively
// or via the nullable-combinator collapse.
func isNullable(field map[string]any) bool {
	arr, ok := field["type"].([]any)
	if !ok {
		return false
	}
	for _, v := range arr {
		if s, ok := v.(string); ok && s == typeNull {
			return true
		}
	}
	return false
}

// elementType returns the displayable element type for an array's items schema.
func elementType(items map[string]any) string {
	if items == nil {
		return typeAny
	}
	if _, hasEnum := items["enum"]; hasEnum {
		return "enum"
	}
	if t := typeName(items); t != "" {
		return t
	}
	return typeAny
}

// boundsSuffix reads a lo/hi numeric pair from the given schema keys and
// returns a parenthesised annotation like "(1-100)", "(min 1)",
// "(max 100)", or "" if neither bound is present. Used for both numeric
// minimum/maximum and array minItems/maxItems.
func boundsSuffix(field map[string]any, loKey, hiKey string) string {
	lo, hasLo := numericField(field, loKey)
	hi, hasHi := numericField(field, hiKey)
	switch {
	case hasLo && hasHi:
		return fmt.Sprintf("(%s-%s)", lo, hi)
	case hasLo:
		return fmt.Sprintf("(min %s)", lo)
	case hasHi:
		return fmt.Sprintf("(max %s)", hi)
	}
	return ""
}

// numericBoundsSuffix formats minimum/maximum on an integer/number schema.
func numericBoundsSuffix(field map[string]any) string {
	return boundsSuffix(field, "minimum", "maximum")
}

// arrayBoundsSuffix formats minItems/maxItems on an array schema.
func arrayBoundsSuffix(field map[string]any) string {
	return boundsSuffix(field, "minItems", "maxItems")
}

// numericField reads a number/integer field and returns its shortest
// string form (no trailing .0 for integer-valued floats). JSON numbers
// decode as float64 by default, so handle that case first. The
// integer-valued-float detection assumes bounds fit in int64; floats
// near math.MaxInt64 are rare in API schemas and would fall through to
// %g, which is acceptable.
func numericField(field map[string]any, key string) (string, bool) {
	raw, ok := field[key]
	if !ok {
		return "", false
	}
	switch v := raw.(type) {
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v)), true
		}
		return fmt.Sprintf("%g", v), true
	case int:
		return fmt.Sprintf("%d", v), true
	case int64:
		return fmt.Sprintf("%d", v), true
	}
	return "", false
}

// typeName extracts a printable type from a schema node, handling a
// single string and nullable arrays like ["string","null"]. Returns ""
// when no type is discoverable.
func typeName(field map[string]any) string {
	switch t := field["type"].(type) {
	case string:
		return t
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok && s != typeNull {
				return s
			}
		}
	}
	return ""
}

// requiredSet turns the "required" array on an object schema into a set
// for O(1) lookup. Missing or malformed returns an empty set.
func requiredSet(schema map[string]any) map[string]bool {
	out := map[string]bool{}
	arr, ok := schema["required"].([]any)
	if !ok {
		return out
	}
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out[s] = true
		}
	}
	return out
}

// stringField reads a string field, returning "" when missing or of
// another type.
func stringField(field map[string]any, key string) string {
	s, _ := field[key].(string)
	return s
}

// collapseWhitespace replaces any run of whitespace (including embedded
// newlines from multi-line descriptions) with a single space and trims
// leading/trailing space. Keeps the tree row-per-field.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// refName reports whether the schema node is a raw {"$ref": "..."}, the
// sentinel left by the parser's cycle-protection pass. Returns the bare
// schema name (last path segment) when so, or ("", false) otherwise.
func refName(field map[string]any) (string, bool) {
	raw, ok := field["$ref"].(string)
	if !ok {
		return "", false
	}
	name := raw
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name, true
}
