package output

import (
	"encoding/csv"
	"fmt"
)

// printCSV renders data as CSV with a header row followed by data rows.
func printCSV(data []byte, columns []ColumnDef, opts Options) error {
	rows, err := extractRows(data)
	if err != nil {
		return err
	}

	if len(columns) == 0 && len(rows) > 0 {
		columns = inferColumns(rows[0])
	}

	if len(columns) == 0 {
		_, _ = fmt.Fprintln(opts.Writer, "No results.")
		return nil
	}

	w := csv.NewWriter(opts.Writer)

	if err := w.Write(columnHeaders(columns)); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}

	// Write data rows.
	for _, row := range rows {
		record := make([]string, len(columns))
		for i, col := range columns {
			record[i] = extractField(row, col.Field)
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
