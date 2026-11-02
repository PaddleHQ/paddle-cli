package output

import (
	"encoding/csv"
	"fmt"
)

// printCSV renders data as CSV with a header row followed by data rows. When an
// object's array became the rows, its other fields lead every row.
func printCSV(data []byte, columns []ColumnDef, opts Options) error {
	l, err := buildLayout(data, columns)
	if err != nil {
		return err
	}
	if len(l.columns) == 0 && len(l.parentColumns) == 0 {
		_, _ = fmt.Fprintln(opts.Writer, "No results.")
		return nil
	}

	w := csv.NewWriter(opts.Writer)
	if err := w.Write(append(columnHeaders(l.parentColumns), columnHeaders(l.columns)...)); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}
	for _, row := range l.rows {
		record := make([]string, 0, len(l.parentColumns)+len(l.columns))
		for _, col := range l.parentColumns {
			record = append(record, csvCell(lookup(l.parent, col.Field)))
		}
		for _, col := range l.columns {
			record = append(record, csvCell(lookup(row, col.Field)))
		}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("writing CSV row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("writing CSV: %w", err)
	}
	return nil
}

func columnHeaders(columns []ColumnDef) []string {
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = col.Header
	}
	return headers
}
