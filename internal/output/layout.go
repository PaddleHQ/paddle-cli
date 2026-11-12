package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// keyField names the column that holds each key when a map becomes rows.
const keyField = "key"

// object is a decoded JSON object that keeps the server's key order.
type object struct {
	keys []string
	vals map[string]any
}

func (o *object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := marshalNoEscape(k)
		if err != nil {
			return nil, err
		}
		val, err := marshalNoEscape(o.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encoding JSON: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

var errTrailingData = errors.New("invalid JSON: trailing data")

// decodeOrdered decodes JSON into *object, []any, string, json.Number, bool
// or nil, so every level keeps the server's key order.
func decodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errTrailingData
	}
	return v, nil
}

func nextToken(dec *json.Decoder) (json.Token, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("reading JSON: %w", err)
	}
	return tok, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := nextToken(dec)
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		return decodeObject(dec)
	case json.Delim('['):
		return decodeArray(dec)
	default:
		return tok, nil
	}
}

func decodeObject(dec *json.Decoder) (*object, error) {
	o := &object{vals: map[string]any{}}
	for dec.More() {
		kt, err := nextToken(dec)
		if err != nil {
			return nil, err
		}
		k, _ := kt.(string)
		v, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		if _, dup := o.vals[k]; !dup {
			o.keys = append(o.keys, k)
		}
		o.vals[k] = v
	}
	_, err := nextToken(dec)
	return o, err
}

func decodeArray(dec *json.Decoder) ([]any, error) {
	arr := []any{}
	for dec.More() {
		v, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
	}
	_, err := nextToken(dec)
	return arr, err
}

// layout is how a response becomes rows. A single object is one row that a
// table shows vertically; parent holds an object's other fields when its
// one array became the rows.
type layout struct {
	columns       []ColumnDef
	rows          []*object
	single        bool
	parent        *object
	parentColumns []ColumnDef
}

func buildLayout(data []byte, columns []ColumnDef) (layout, error) {
	v, err := decodeOrdered(data)
	if err != nil {
		return layout{}, err
	}
	if o, ok := v.(*object); ok {
		if inner, ok := o.vals["data"]; ok {
			v = inner
		}
	}
	switch val := v.(type) {
	case nil:
		return layout{}, nil
	case []any:
		rows := objectsIn(val)
		if len(columns) == 0 && len(rows) > 0 {
			columns = columnsFromKeys(rows[0].keys)
		}
		return layout{columns: columns, rows: rows}, nil
	case *object:
		return objectLayout(val, columns), nil
	default:
		return layout{}, fmt.Errorf("%w: %T", errUnexpectedShape, v)
	}
}

func objectLayout(o *object, columns []ColumnDef) layout {
	if rows, shape, ok := mapRows(o); ok {
		return layout{columns: columnsFromKeys(append([]string{keyField}, shape...)), rows: rows}
	}
	if len(columns) == 0 {
		columns = columnsFromKeys(o.keys)
	}
	if field, rows, ok := soleArrayOfObjects(o); ok {
		parent := slices.DeleteFunc(slices.Clone(columns), func(c ColumnDef) bool { return c.Field == field })
		return layout{columns: columnsFromKeys(rows[0].keys), rows: rows, parent: o, parentColumns: parent}
	}
	return layout{columns: columns, rows: []*object{o}, single: true}
}

// mapRows turns an object keyed by data, such as currency codes, whose values
// all share one shape into a row per key. A lone field holding such an object
// (payment_methods, say) is unwrapped first, so a lone top-level object field
// is never itself read as a map of one row.
func mapRows(o *object) ([]*object, []string, bool) {
	if len(o.keys) == 1 {
		if inner, ok := o.vals[o.keys[0]].(*object); ok {
			return sameShapeRows(inner)
		}
	}
	return sameShapeRows(o)
}

func sameShapeRows(o *object) ([]*object, []string, bool) {
	if len(o.keys) == 0 {
		return nil, nil, false
	}
	shape := []string(nil)
	rows := make([]*object, 0, len(o.keys))
	for _, k := range o.keys {
		inner, ok := o.vals[k].(*object)
		if !ok || len(inner.keys) == 0 || slices.Contains(inner.keys, keyField) {
			return nil, nil, false
		}
		if shape == nil {
			shape = inner.keys
		} else if !sameKeys(shape, inner.keys) {
			return nil, nil, false
		}
		row := &object{keys: append([]string{keyField}, shape...), vals: map[string]any{keyField: k}}
		maps.Copy(row.vals, inner.vals)
		rows = append(rows, row)
	}
	return rows, shape, true
}

func sameKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, k := range a {
		if !slices.Contains(b, k) {
			return false
		}
	}
	return true
}

// soleArrayOfObjects finds the array of objects that is an object's content,
// such as a metric's timeseries: every other field is a plain value and the
// rows reuse none of their names. Entities (they have an id) and objects with
// more structure, like previews, stay one object.
func soleArrayOfObjects(o *object) (string, []*object, bool) {
	if _, entity := o.vals["id"]; entity {
		return "", nil, false
	}
	field, items, ok := onlyArrayField(o)
	if !ok {
		return "", nil, false
	}
	rows := objectsIn(items)
	if len(rows) == 0 || len(rows) != len(items) || reusesNames(o, rows) {
		return "", nil, false
	}
	return field, rows, true
}

// onlyArrayField finds the object's one array when every other field is a
// plain value.
func onlyArrayField(o *object) (field string, items []any, ok bool) {
	for _, k := range o.keys {
		arr, isArr := o.vals[k].([]any)
		if _, isObj := o.vals[k].(*object); isObj || (isArr && field != "") {
			return "", nil, false
		}
		if isArr {
			field, items = k, arr
		}
	}
	return field, items, field != ""
}

func reusesNames(o *object, rows []*object) bool {
	return slices.ContainsFunc(rows, func(r *object) bool {
		return slices.ContainsFunc(r.keys, func(k string) bool { _, clash := o.vals[k]; return clash })
	})
}

func objectsIn(items []any) []*object {
	rows := make([]*object, 0, len(items))
	for _, item := range items {
		if o, ok := item.(*object); ok {
			rows = append(rows, o)
		}
	}
	return rows
}

// lookup follows a dot-separated field path.
func lookup(o *object, field string) any {
	head, rest, nested := strings.Cut(field, ".")
	v, ok := o.vals[head]
	if !ok {
		return nil
	}
	if !nested {
		return v
	}
	if inner, ok := v.(*object); ok {
		return lookup(inner, rest)
	}
	return nil
}

// scalarText renders a non-nested value; ok is false for arrays and objects.
func scalarText(v any) (string, bool) {
	switch val := v.(type) {
	case nil:
		return "", true
	case string:
		return val, true
	case json.Number:
		return val.String(), true
	case bool:
		return strconv.FormatBool(val), true
	default:
		return "", false
	}
}

// csvCell renders a value in full, nested values as compact JSON.
func csvCell(v any) string {
	if s, ok := scalarText(v); ok {
		return s
	}
	b, err := marshalNoEscape(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// tableCell renders a value for a table, cutting anything longer than 64
// characters unless opts.NoTrunc asks for everything. UTC timestamps drop
// seconds; JSON and CSV keep the exact value.
func tableCell(v any, opts Options) string {
	if ts, ok := shortTimestamp(v); ok {
		return ts
	}
	s := csvCell(v)
	r := []rune(s)
	if opts.NoTrunc || len(r) <= 64 {
		return s
	}
	if opts.Truncated != nil {
		*opts.Truncated = true
	}
	return string(r[:61]) + "..."
}

func shortTimestamp(v any) (string, bool) {
	str, ok := v.(string)
	if !ok || !strings.HasSuffix(str, "Z") {
		return "", false
	}
	ts, err := time.Parse(time.RFC3339Nano, str)
	if err != nil {
		return "", false
	}
	return ts.Format("2006-01-02 15:04"), true
}

// lineWidth is the width a FIELD | VALUE value wraps at, and maxLines how
// many lines it shows before cutting.
const (
	lineWidth = 64
	maxLines  = 4
)

// fieldCell renders a value for the FIELD | VALUE view, where each value can
// take several lines. Text breaks only at spaces and JSON only after commas, so
// URLs, IDs and JSON values stay whole and copyable; both stop after four lines.
func fieldCell(v any, opts Options) string {
	if ts, ok := shortTimestamp(v); ok {
		return ts
	}
	if opts.NoTrunc {
		return csvCell(v)
	}
	var lines []string
	if s, ok := scalarText(v); ok {
		for p := range strings.SplitSeq(s, "\n") {
			pieces, cut := capPieces(strings.Split(p, " "), opts)
			lines = append(lines, packLines(pieces, " ", lineWidth)...)
			if cut {
				break
			}
		}
	} else {
		pieces, _ := capPieces(jsonPieces(csvCell(v)), opts)
		lines = packLines(pieces, "", lineWidth)
	}
	if len(lines) > maxLines {
		if opts.Truncated != nil {
			*opts.Truncated = true
		}
		lines = lines[:maxLines]
		lines[maxLines-1] += "..."
	}
	return strings.Join(lines, "\n")
}

// jsonPieces splits compact JSON after each comma between values, never after
// one inside a string, so a line break can't land inside a value.
func jsonPieces(j string) []string {
	var pieces []string
	start, inString, escaped := 0, false, false
	for i, r := range j {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == ',' && !inString:
			pieces = append(pieces, j[start:i+1])
			start = i + 1
		}
	}
	return append(pieces, j[start:])
}

// capPieces cuts the first unbreakable piece longer than a whole value's worth
// of lines, and ends the value there, so a spaceless blob can't widen the table
// without limit. URLs are never cut, however long, so they stay copyable.
func capPieces(pieces []string, opts Options) ([]string, bool) {
	limit := lineWidth * maxLines
	for i, p := range pieces {
		if lenOutsideURLs(p) <= limit {
			continue
		}
		if r := []rune(p); len(r) > limit {
			if opts.Truncated != nil {
				*opts.Truncated = true
			}
			pieces[i] = string(r[:limit-3]) + "..."
			return pieces[:i+1], true
		}
	}
	return pieces, false
}

// lenOutsideURLs counts a piece's characters outside its URLs, so a JSON piece
// that holds a long note as well as a URL is still capped.
func lenOutsideURLs(p string) int {
	n := 0
	for tok := range strings.FieldsSeq(p) {
		if !strings.Contains(tok, "http://") && !strings.Contains(tok, "https://") {
			n += utf8.RuneCountInString(tok) + 1
		}
	}
	return n
}

// packLines joins pieces into lines of at most width; a piece longer than
// width gets a line of its own rather than being split.
func packLines(pieces []string, sep string, width int) []string {
	var lines []string
	line, started := "", false
	for _, p := range pieces {
		switch {
		case !started:
			line, started = p, true
		case utf8.RuneCountInString(line+sep+p) <= width:
			line += sep + p
		default:
			lines = append(lines, line)
			line = p
		}
	}
	return append(lines, line)
}

// fieldRow is one FIELD | VALUE line of a vertical table.
type fieldRow struct {
	name  string
	value any
}

// flattenFields lists a single object's fields, expanding nested objects into
// dotted names so each value gets its own line.
func flattenFields(o *object, columns []ColumnDef) []fieldRow {
	var out []fieldRow
	for _, c := range columns {
		out = appendFlattened(out, c.Field, lookup(o, c.Field))
	}
	return out
}

func appendFlattened(out []fieldRow, name string, v any) []fieldRow {
	inner, ok := v.(*object)
	if !ok || len(inner.keys) == 0 {
		return append(out, fieldRow{name, v})
	}
	for _, k := range inner.keys {
		out = appendFlattened(out, name+"."+k, inner.vals[k])
	}
	return out
}

// WriteTruncationHint points the reader at the full data when a table cut a
// value short, unless the banner (and its footers) is off.
func WriteTruncationHint(w io.Writer, truncated *bool, suppressed bool) {
	if truncated == nil || !*truncated || suppressed {
		return
	}
	_, _ = fmt.Fprintln(w, "Some values are cut short. See them in full with -o json or -o csv.")
}
