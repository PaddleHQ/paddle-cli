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
// (payment_methods, say) is unwrapped first.
func mapRows(o *object) ([]*object, []string, bool) {
	if len(o.keys) == 1 {
		if inner, ok := o.vals[o.keys[0]].(*object); ok {
			return sameShapeRows(inner)
		}
	}
	return sameShapeRows(o)
}

func sameShapeRows(o *object) ([]*object, []string, bool) {
	if len(o.keys) < 2 {
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

// soleArrayOfObjects finds the one field holding a non-empty array of objects,
// such as a metric's timeseries, which is the content worth tabulating. An
// entity (it has an id) is the content itself, so its lists stay fields.
func soleArrayOfObjects(o *object) (string, []*object, bool) {
	if _, entity := o.vals["id"]; entity {
		return "", nil, false
	}
	field, items := "", []any(nil)
	for _, k := range o.keys {
		if arr, ok := o.vals[k].([]any); ok && len(arr) > 0 {
			if _, isObj := arr[0].(*object); isObj {
				if field != "" {
					return "", nil, false
				}
				field, items = k, arr
			}
		}
	}
	if field == "" {
		return "", nil, false
	}
	return field, objectsIn(items), true
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
	s := csvCell(v)
	if str, ok := v.(string); ok && strings.HasSuffix(str, "Z") {
		if ts, err := time.Parse(time.RFC3339Nano, str); err == nil {
			return ts.Format("2006-01-02 15:04")
		}
	}
	if opts.NoTrunc || len(s) <= 64 {
		return s
	}
	if opts.Truncated != nil {
		*opts.Truncated = true
	}
	return s[:61] + "..."
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
