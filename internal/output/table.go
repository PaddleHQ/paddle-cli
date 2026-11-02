package output

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/PaddleHQ/paddle-cli/internal/style"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

var errUnexpectedShape = errors.New("unexpected JSON shape")

// styleField applies the profile's semantic helper appropriate for the
// column field name. Status-like columns use Status(); id-like columns
// use ID(); everything else passes through.
func styleField(profile style.Profile, field, value string) string {
	if !profile.IsEnabled() || value == "" {
		return value
	}
	switch {
	case isFieldKind(field, "status") || isFieldKind(field, "state"):
		return profile.Status(value)
	case isFieldKind(field, "id"):
		return profile.ID(value)
	case isFieldKind(field, "email"):
		return profile.Email(value)
	case value == "true" || value == "false":
		return profile.Bool(value)
	}
	return value
}

// isFieldKind reports whether field is kind itself or a "_kind"-suffixed name.
func isFieldKind(field, kind string) bool {
	return field == kind || strings.HasSuffix(field, "_"+kind)
}

// printTable renders data as a styled table using go-pretty: a single object
// as FIELD | VALUE lines, anything else as one row per item.
func printTable(data []byte, columns []ColumnDef, opts Options) error {
	l, err := buildLayout(data, columns)
	if err != nil {
		return err
	}
	if len(l.rows) == 0 {
		_, _ = fmt.Fprintln(opts.Writer, "No results.")
		return nil
	}
	if l.single {
		renderFields(flattenFields(l.rows[0], l.columns), opts)
		return nil
	}
	if l.parent != nil {
		renderFields(flattenFields(l.parent, l.parentColumns), opts)
		_, _ = fmt.Fprintln(opts.Writer)
	}
	shown := dropEmptyColumns(l.rows, expandWrapper(l.rows, l.columns, opts))
	t := newTable(opts)
	header := make(table.Row, len(shown))
	for i, col := range shown {
		header[i] = col.Header
	}
	t.AppendHeader(header)
	for _, row := range l.rows {
		cells := make(table.Row, len(shown))
		for i, col := range shown {
			cells[i] = styleField(opts.Profile, col.Field, tableCell(lookup(row, col.Field), opts))
		}
		t.AppendRow(cells)
	}
	t.Render()
	return nil
}

// expandWrapper turns a column that wraps each row's content, like an event's
// data, into a column per field every row shares. It only applies when that
// object is bigger than the row's own fields, so details like a notification's
// payload stay one column. Fields some rows lack count as cut short.
func expandWrapper(rows []*object, columns []ColumnDef, opts Options) []ColumnDef {
	field, inners, ok := wrapperField(rows)
	if !ok {
		return columns
	}
	shared := slices.Clone(inners[0].keys)
	widest := 0
	for _, inner := range inners {
		shared = slices.DeleteFunc(shared, func(k string) bool { _, has := inner.vals[k]; return !has })
		widest = max(widest, len(inner.keys))
	}
	i := slices.IndexFunc(columns, func(c ColumnDef) bool { return c.Field == field })
	if i < 0 || len(shared) == 0 {
		return columns
	}
	if widest > len(shared) && opts.Truncated != nil {
		*opts.Truncated = true
	}
	expanded := make([]ColumnDef, len(shared))
	for j, k := range shared {
		expanded[j] = ColumnDef{Header: strings.ToUpper(field + "." + k), Field: field + "." + k}
	}
	return slices.Concat(columns[:i], expanded, columns[i+1:])
}

// wrapperField finds the one field that holds an object on every row, where
// no other field ever does, and that object outsizes the row's own fields.
func wrapperField(rows []*object) (string, []*object, bool) {
	field, ok := onlyObjectField(rows)
	if !ok {
		return "", nil, false
	}
	inners := make([]*object, 0, len(rows))
	for _, r := range rows {
		inner, isObj := r.vals[field].(*object)
		if !isObj || len(inner.keys) < len(r.keys) {
			return "", nil, false
		}
		inners = append(inners, inner)
	}
	return field, inners, true
}

func onlyObjectField(rows []*object) (string, bool) {
	field := ""
	for _, r := range rows {
		for _, k := range r.keys {
			if inner, ok := r.vals[k].(*object); ok && len(inner.keys) > 0 {
				if field != "" && field != k {
					return "", false
				}
				field = k
			}
		}
	}
	return field, field != ""
}

// dropEmptyColumns keeps a list's columns that have a value on at least one
// row, since an all-empty column costs a column of width and shows nothing.
// false and 0 count as values. If every column is empty, all are kept.
func dropEmptyColumns(rows []*object, columns []ColumnDef) []ColumnDef {
	kept := slices.DeleteFunc(slices.Clone(columns), func(c ColumnDef) bool {
		return !slices.ContainsFunc(rows, func(r *object) bool { return !isEmptyValue(lookup(r, c.Field)) })
	})
	if len(kept) == 0 {
		return columns
	}
	return kept
}

func isEmptyValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case *object:
		return len(val.keys) == 0
	}
	return false
}

func renderFields(fields []fieldRow, opts Options) {
	t := newTable(opts)
	t.AppendHeader(table.Row{"FIELD", "VALUE"})
	for _, f := range fields {
		leaf := f.name[strings.LastIndex(f.name, ".")+1:]
		t.AppendRow(table.Row{f.name, styleField(opts.Profile, leaf, fieldCell(f.value, opts))})
	}
	t.Render()
}

func newTable(opts Options) table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(opts.Writer)
	t.SetStyle(table.StyleRounded)
	// Preserve header text exactly as supplied: the caller has already
	// chosen the casing (inferred columns are uppercased, explicit columns
	// keep whatever the caller set).
	t.Style().Format.Header = text.FormatDefault
	// Header styling now goes via the profile so the no-color path is the
	// same one all other surfaces use. Keep this conditional: go-pretty's
	// renderer uses the Color block to keep column padding stable.
	if opts.Profile.IsEnabled() {
		t.Style().Color.Header = text.Colors{text.Bold}
	}
	return t
}
