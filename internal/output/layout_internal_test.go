package output

import (
	"bytes"
	"strings"
	"testing"
)

func render(t *testing.T, format Format, input string, opts Options) string {
	t.Helper()
	var buf bytes.Buffer
	opts.Format = format
	opts.Writer = &buf
	if err := Print([]byte(input), nil, opts); err != nil {
		t.Fatalf("Print: %v", err)
	}
	return buf.String()
}

// lineWith returns the first table line containing every one of parts.
func lineWith(out string, parts ...string) string {
	for line := range strings.SplitSeq(out, "\n") {
		found := true
		for _, p := range parts {
			found = found && strings.Contains(line, p)
		}
		if found {
			return line
		}
	}
	return ""
}

func TestPrintTable_SingleObjectIsVertical(t *testing.T) {
	out := render(t, FormatTable, `{"data":{"id":"pri_01","name":"Pro","billing_cycle":{"interval":"month","frequency":1},"trial_period":null}}`, Options{})

	if lineWith(out, "FIELD", "VALUE") == "" {
		t.Errorf("expected a FIELD | VALUE header:\n%s", out)
	}
	for _, want := range [][]string{{"id", "pri_01"}, {"name", "Pro"}, {"billing_cycle.interval", "month"}, {"billing_cycle.frequency", "1"}, {"trial_period"}} {
		if lineWith(out, want...) == "" {
			t.Errorf("expected a row with %v:\n%s", want, out)
		}
	}
	if strings.Index(out, "billing_cycle.interval") > strings.Index(out, "billing_cycle.frequency") {
		t.Errorf("nested fields should keep the server's order:\n%s", out)
	}
}

func TestPrint_MapOfSameShapeObjectsBecomesRows(t *testing.T) {
	input := `{"data":{"ARS":{"enabled":true},"AUD":{"enabled":false}}}`

	if got, want := render(t, FormatCSV, input, Options{}), "KEY,ENABLED\nARS,true\nAUD,false\n"; got != want {
		t.Errorf("CSV:\ngot  %q\nwant %q", got, want)
	}
	out := render(t, FormatTable, input, Options{})
	if lineWith(out, "KEY", "ENABLED") == "" || lineWith(out, "ARS", "true") == "" || lineWith(out, "AUD", "false") == "" {
		t.Errorf("expected one row per key:\n%s", out)
	}
}

func TestPrint_FieldHoldingAMapOfSameShapeObjectsBecomesRows(t *testing.T) {
	input := `{"data":{"payment_methods":{"alipay":{"available":true,"enabled":false},"card":{"available":true,"enabled":true}}}}`

	if got, want := render(t, FormatCSV, input, Options{}), "KEY,AVAILABLE,ENABLED\nalipay,true,false\ncard,true,true\n"; got != want {
		t.Errorf("CSV:\ngot  %q\nwant %q", got, want)
	}
}

func TestPrint_ObjectsOfDifferentShapesStayOneObject(t *testing.T) {
	out := render(t, FormatCSV, `{"data":{"a":{"x":1},"b":{"y":2}}}`, Options{})
	if !strings.HasPrefix(out, "A,B\n") {
		t.Errorf("objects of different shapes aren't a map of rows; got:\n%s", out)
	}
}

func TestPrint_SoleArrayOfObjectsBecomesRows(t *testing.T) {
	input := `{"data":{"currency_code":"GBP","interval":"day","timeseries":[{"timestamp":"2026-10-01T00:00:00Z","amount":"17"},{"timestamp":"2026-10-02T00:00:00Z","amount":"0"}]}}`

	want := "CURRENCY_CODE,INTERVAL,TIMESTAMP,AMOUNT\nGBP,day,2026-10-01T00:00:00Z,17\nGBP,day,2026-10-02T00:00:00Z,0\n"
	if got := render(t, FormatCSV, input, Options{}); got != want {
		t.Errorf("CSV:\ngot  %q\nwant %q", got, want)
	}

	out := render(t, FormatTable, input, Options{})
	if lineWith(out, "currency_code", "GBP") == "" {
		t.Errorf("expected the other fields above the rows:\n%s", out)
	}
	if lineWith(out, "TIMESTAMP", "AMOUNT") == "" || lineWith(out, "2026-10-02 00:00", "0") == "" {
		t.Errorf("expected one row per array item:\n%s", out)
	}
}

func TestPrintTable_CutsLongCells(t *testing.T) {
	long := strings.Repeat("x", 70)
	items := `[{"price_id":"pri_01","quantity":1},{"price_id":"pri_02","quantity":1}]`
	input := `{"data":[{"id":"txn_01","tags":["a","b"],"note":"` + long + `","items":` + items + `}]}`

	var truncated bool
	out := render(t, FormatTable, input, Options{Truncated: &truncated})
	if lineWith(out, `["a","b"]`) == "" {
		t.Errorf("a short nested value should show in full:\n%s", out)
	}
	if lineWith(out, items[:61]+"...") == "" {
		t.Errorf("a long nested value should show its first 61 characters:\n%s", out)
	}
	if lineWith(out, strings.Repeat("x", 61)+"...") == "" {
		t.Errorf("a long string should show its first 61 characters:\n%s", out)
	}
	if !truncated {
		t.Error("expected Truncated to be set")
	}

	truncated = false
	out = render(t, FormatTable, `{"data":[{"id":"txn_01","tags":["a","b"]}]}`, Options{Truncated: &truncated})
	if truncated {
		t.Errorf("nothing was cut, so Truncated should stay false:\n%s", out)
	}

	out = render(t, FormatTable, input, Options{Truncated: &truncated, NoTrunc: true})
	if lineWith(out, items) == "" || lineWith(out, long) == "" || truncated {
		t.Errorf("NoTrunc should show every value in full, without cutting:\n%s", out)
	}
}

func TestWriteTruncationHint(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name       string
		truncated  *bool
		suppressed bool
		want       bool
	}{
		{"truncated", &yes, false, true},
		{"nothing truncated", &no, false, false},
		{"no tracking", nil, false, false},
		{"banner suppressed", &yes, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteTruncationHint(&buf, tc.truncated, tc.suppressed)
			if got := strings.Contains(buf.String(), "-o json or -o csv"); got != tc.want {
				t.Errorf("hint written = %v, want %v (%q)", got, tc.want, buf.String())
			}
		})
	}
}

// An entity that contains a list, like a subscription's items, is still one
// object; only an id-less aggregate, like a metric, is shown as its array.
func TestPrint_EntityWithOneArrayStaysOneObject(t *testing.T) {
	input := `{"data":{"id":"sub_01","status":"active","items":[{"status":"active","quantity":1}]}}`

	if got := render(t, FormatCSV, input, Options{}); !strings.HasPrefix(got, "ID,STATUS,ITEMS\n") {
		t.Errorf("CSV should be the subscription's own columns, got:\n%s", got)
	}
	out := render(t, FormatTable, input, Options{})
	if lineWith(out, "FIELD", "VALUE") == "" || lineWith(out, "items", `[{"status":"active","quantity":1}]`) == "" {
		t.Errorf("table should show the subscription vertically, items as a value:\n%s", out)
	}
}

// Events wrap their content in data; a table shows data's fields as columns,
// but only those every row on the page has.
func TestPrintTable_ExpandsWrappedContent(t *testing.T) {
	mixed := `{"data":[
		{"event_id":"evt_1","event_type":"transaction.created","occurred_at":"2026-11-01","data":{"id":"txn_1","status":"draft","billed_at":null,"currency_code":"GBP"}},
		{"event_id":"evt_2","event_type":"address.created","occurred_at":"2026-11-02","data":{"id":"add_1","status":"active","city":"Leeds","country_code":"GB"}}
	]}`
	var truncated bool
	out := render(t, FormatTable, mixed, Options{Truncated: &truncated})
	if lineWith(out, "EVENT_ID", "EVENT_TYPE", "OCCURRED_AT", "DATA.ID", "DATA.STATUS") == "" || lineWith(out, "evt_2", "add_1", "active") == "" {
		t.Errorf("expected the shared data fields as columns:\n%s", out)
	}
	if strings.Contains(out, "DATA.CITY") || strings.Contains(out, "DATA.CURRENCY_CODE") || lineWith(out, "│ DATA ") != "" {
		t.Errorf("fields only some rows have, and the data column itself, shouldn't show:\n%s", out)
	}
	if !truncated {
		t.Error("leaving out fields some rows have should set Truncated")
	}

	sameType := `{"data":[
		{"event_id":"evt_1","event_type":"address.created","occurred_at":"2026-11-01","data":{"id":"add_1","status":"active","city":"Leeds","country_code":"GB"}},
		{"event_id":"evt_2","event_type":"address.created","occurred_at":"2026-11-02","data":{"id":"add_2","status":"active","city":"York","country_code":"GB"}}
	]}`
	truncated = false
	out = render(t, FormatTable, sameType, Options{Truncated: &truncated})
	if lineWith(out, "DATA.ID", "DATA.STATUS", "DATA.CITY", "DATA.COUNTRY_CODE") == "" || truncated {
		t.Errorf("one event type should show all of data's fields, with nothing left out:\n%s", out)
	}

	if got := render(t, FormatCSV, mixed, Options{}); !strings.HasPrefix(got, "EVENT_ID,EVENT_TYPE,OCCURRED_AT,DATA\n") {
		t.Errorf("CSV should keep data as one column, got:\n%s", got)
	}
}

// A small object next to more fields of the row's own, like a notification's
// payload, is a detail rather than a wrapper, so it stays one column.
func TestPrintTable_KeepsSmallObjectAsOneColumn(t *testing.T) {
	input := `{"data":[{"id":"ntf_1","type":"webhook","status":"delivered","origin":"event","payload":{"event_id":"evt_1","event_type":"x"}}]}`
	out := render(t, FormatTable, input, Options{})
	if lineWith(out, "PAYLOAD") == "" || strings.Contains(out, "PAYLOAD.EVENT_ID") {
		t.Errorf("payload should stay one column:\n%s", out)
	}
}

func TestTableCell_ShortensUTCTimestamps(t *testing.T) {
	cases := map[string]string{
		"2026-10-27T17:23:20.811Z":  "2026-10-27 17:23",
		"2026-10-02T00:00:00Z":      "2026-10-02 00:00",
		"2026-10-27T17:23:20+02:00": "2026-10-27T17:23:20+02:00",
		"2026-10-27":                "2026-10-27",
		"not a time":                "not a time",
	}
	for in, want := range cases {
		if got := tableCell(in, Options{}); got != want {
			t.Errorf("tableCell(%q) = %q, want %q", in, got, want)
		}
	}
	if got := csvCell("2026-10-27T17:23:20.811Z"); got != "2026-10-27T17:23:20.811Z" {
		t.Errorf("CSV should keep the exact timestamp, got %q", got)
	}
}

func TestFieldCell(t *testing.T) {
	prose := "Monthly subscription to the Pro plan, billed in arrears with a fourteen-day trial before the first charge."
	url := "https://example.com/checkout?_ptxn=txn_01m5wh4kjcrnzjs7e8zj8s5jn2&utm_source=newsletter&utm_campaign=autumn"
	jsonOf := func(n int) []any {
		items := make([]any, n)
		for i := range items {
			items[i] = &object{keys: []string{"price_id", "quantity"}, vals: map[string]any{"price_id": "pri_0000000000000000000000000" + string(rune('a'+i)), "quantity": "1"}}
		}
		return items
	}

	t.Run("prose wraps at spaces without cutting", func(t *testing.T) {
		var truncated bool
		got := fieldCell(prose, Options{Truncated: &truncated})
		if strings.ReplaceAll(got, "\n", " ") != prose || truncated {
			t.Errorf("expected every word, only line breaks added:\n%s", got)
		}
		for line := range strings.SplitSeq(got, "\n") {
			if len(line) > 64 {
				t.Errorf("line longer than 64: %q", line)
			}
		}
	})

	t.Run("text past four lines is cut", func(t *testing.T) {
		var truncated bool
		long := strings.Repeat(prose+" ", 4)
		got := fieldCell(long, Options{Truncated: &truncated})
		lines := strings.Split(got, "\n")
		if len(lines) != 4 || !strings.HasSuffix(got, "...") || !truncated {
			t.Errorf("expected four lines ending in ... and Truncated set:\n%s", got)
		}
		if !strings.HasPrefix(long, strings.TrimSuffix(strings.ReplaceAll(got, "\n", " "), "...")) {
			t.Errorf("the lines shown should be the start of the text, words whole:\n%s", got)
		}
	})

	t.Run("a long URL stays on one line", func(t *testing.T) {
		got := fieldCell("Pay here: "+url, Options{})
		if !strings.Contains(got, "\n"+url) || strings.Contains(got, "...") {
			t.Errorf("the URL should be whole on its own line:\n%s", got)
		}
	})

	t.Run("JSON breaks only after commas", func(t *testing.T) {
		v := []any{"address.read", "business.read", "customer.read", "subscription.read", "transaction.read", "verification.read"}
		got := fieldCell(v, Options{})
		if strings.ReplaceAll(got, "\n", "") != csvCell(v) || !strings.Contains(got, "\n") {
			t.Errorf("expected the full JSON over more than one line:\n%s", got)
		}
		for line := range strings.SplitSeq(got, "\n") {
			if len(line) > 64 {
				t.Errorf("line longer than 64: %q", line)
			}
			if !strings.HasSuffix(line, ",") && !strings.HasSuffix(line, "]") {
				t.Errorf("line should end after a comma or at the end, not mid-value: %q", line)
			}
		}
	})

	t.Run("a comma inside a JSON string isn't a break point", func(t *testing.T) {
		v := []any{&object{keys: []string{"id", "description"}, vals: map[string]any{
			"id": "sim_01", "description": "Sends address events to our CRM, then waits for a reply, then retries",
		}}}
		got := fieldCell(v, Options{})
		if strings.Contains(got, "CRM,\n") || strings.Contains(got, "reply,\n") {
			t.Errorf("broke inside a string value:\n%s", got)
		}
		if strings.ReplaceAll(got, "\n", "") != csvCell(v) {
			t.Errorf("expected the full JSON:\n%s", got)
		}
	})

	t.Run("a spaceless value up to 256 characters stays whole", func(t *testing.T) {
		long := "https://example.com/" + strings.Repeat("a", 200)
		if got := fieldCell(long, Options{}); got != long {
			t.Errorf("expected the %d-char value whole, got:\n%s", len(long), got)
		}
	})

	t.Run("a URL is never cut, however long", func(t *testing.T) {
		signed := "https://example.com/doc.pdf?X-Amz-Signature=" + strings.Repeat("a", 1600)
		if got := fieldCell(signed, Options{}); got != signed {
			t.Errorf("expected the %d-char URL whole, got %d chars", len(signed), len(got))
		}
		v := []any{&object{keys: []string{"url"}, vals: map[string]any{"url": signed}}}
		if got := fieldCell(v, Options{}); !strings.Contains(got, signed) {
			t.Errorf("expected the URL whole inside JSON too, got %d chars", len(got))
		}
	})

	t.Run("a spaceless value past 256 characters is cut", func(t *testing.T) {
		var truncated bool
		blob := strings.Repeat("x", 300)
		got := fieldCell(blob, Options{Truncated: &truncated})
		if got != blob[:253]+"..." || !truncated {
			t.Errorf("expected 253 chars and ..., with Truncated set; got %d chars: %q", len(got), got)
		}
	})

	t.Run("the value ends where a long piece is cut", func(t *testing.T) {
		for _, in := range []string{"start " + strings.Repeat("x", 300) + " tail", "start " + strings.Repeat("x", 300) + "\nnext paragraph"} {
			got := fieldCell(in, Options{})
			if !strings.HasSuffix(got, "...") || strings.Contains(got, "tail") || strings.Contains(got, "next") {
				t.Errorf("nothing should follow the cut:\n%s", got)
			}
		}
	})

	t.Run("JSON past four lines is cut", func(t *testing.T) {
		var truncated bool
		got := fieldCell(jsonOf(8), Options{Truncated: &truncated})
		lines := strings.Split(got, "\n")
		if len(lines) != 4 || !strings.HasSuffix(got, "...") || !truncated {
			t.Errorf("expected four lines ending in ... and Truncated set:\n%s", got)
		}
	})

	t.Run("timestamps still shorten", func(t *testing.T) {
		if got := fieldCell("2026-10-27T17:23:20.811Z", Options{}); got != "2026-10-27 17:23" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("NoTrunc shows everything on one line", func(t *testing.T) {
		v := jsonOf(8)
		if got := fieldCell(v, Options{NoTrunc: true}); got != csvCell(v) {
			t.Errorf("got %q", got)
		}
	})
}
