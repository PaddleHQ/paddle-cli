package schema_test

import (
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/schema"
	"github.com/PaddleHQ/paddle-cli/internal/style"
)

// normaliseTree collapses runs of whitespace inside each line (tabwriter
// spacing) while preserving leading indentation, so tests can compare
// content without being brittle about tabwriter's column widths.
func normaliseTree(s string) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		fields := strings.Fields(ln)
		if len(fields) == 0 {
			out = append(out, "")
			continue
		}
		indent := len(ln) - len(strings.TrimLeft(ln, " "))
		out = append(out, strings.Repeat(" ", indent)+strings.Join(fields, " "))
	}
	return out
}

// equalLines returns true if a and b match element-wise.
func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRenderTree_FlatPrimitives(t *testing.T) {
	schemaJSON := `{
		"type": "object",
		"required": ["email"],
		"properties": {
			"email": {"type": "string", "description": "The customer's email address."},
			"name": {"type": "string", "description": "Optional name."},
			"age": {"type": "integer"}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	// Required first, then alphabetical within each group.
	want := []string{
		"  email string required The customer's email address.",
		"  age integer",
		"  name string Optional name.",
	}
	if !equalLines(got, want) {
		t.Errorf("tree mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_CollapsesNewlinesInDescription(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"properties":{
			"email":{"type":"string","description":"Line one.\nLine two."}
		}
	}`
	got := schema.RenderTree(schemaJSON, style.Profile{})
	if strings.Contains(got, "\nLine two") {
		t.Errorf("expected newline inside description to be collapsed, got:\n%s", got)
	}
	if !strings.Contains(got, "Line one. Line two.") {
		t.Errorf("expected collapsed description, got:\n%s", got)
	}
}

func TestRenderTree_EmptyAndInvalid(t *testing.T) {
	if got := schema.RenderTree("", style.Profile{}); got != "" {
		t.Errorf("expected empty string for empty input, got: %q", got)
	}
	if got := schema.RenderTree("not json", style.Profile{}); got != "" {
		t.Errorf("expected empty string for invalid input, got: %q", got)
	}
}

func TestRenderTree_NumericBounds(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"properties":{
			"ranged":{"type":"integer","minimum":1,"maximum":100},
			"atleast":{"type":"integer","minimum":1},
			"atmost":{"type":"integer","maximum":100},
			"plain":{"type":"integer"}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	want := []string{
		"  atleast integer (min 1)",
		"  atmost integer (max 100)",
		"  plain integer",
		"  ranged integer (1-100)",
	}
	if !equalLines(got, want) {
		t.Errorf("bounds enrichment mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_Enum(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"properties":{
			"status":{"type":"string","enum":["active","archived"],"description":"Customer status."}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	want := []string{
		"  status enum Customer status.",
		"    values: active | archived",
	}
	if !equalLines(got, want) {
		t.Errorf("enum mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_Nullable(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"properties":{
			"name":{"type":["string","null"]}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	want := []string{
		"  name string|null",
	}
	if !equalLines(got, want) {
		t.Errorf("nullable should render as X|null\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_NestedObject(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"properties":{
			"address":{
				"type":"object",
				"required":["line1"],
				"properties":{
					"line1":{"type":"string","description":"First line."},
					"country_code":{"type":"string"}
				}
			}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	// Required field (line1) sorts before optional (country_code).
	want := []string{
		"  address object",
		"    line1 string required First line.",
		"    country_code string",
	}
	if !equalLines(got, want) {
		t.Errorf("nested object mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_ArrayOfPrimitive(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"properties":{
			"tags":{"type":"array","items":{"type":"string"}}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	want := []string{
		"  tags array[string]",
	}
	if !equalLines(got, want) {
		t.Errorf("array[string] mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_ArrayOfObject(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"required":["items"],
		"properties":{
			"items":{
				"type":"array",
				"minItems":1,
				"items":{
					"type":"object",
					"required":["price_id"],
					"properties":{
						"price_id":{"type":"string"},
						"quantity":{"type":"integer","minimum":1}
					}
				}
			}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	want := []string{
		"  items array[object] (min 1) required",
		"    price_id string required",
		"    quantity integer (min 1)",
	}
	if !equalLines(got, want) {
		t.Errorf("array[object] mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_DeepNesting(t *testing.T) {
	// Three levels: billing_details > payment_terms > frequency
	schemaJSON := `{
		"type":"object",
		"properties":{
			"billing_details":{
				"type":"object",
				"properties":{
					"payment_terms":{
						"type":"object",
						"properties":{
							"frequency":{"type":"integer","minimum":1}
						}
					}
				}
			}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	want := []string{
		"  billing_details object",
		"    payment_terms object",
		"      frequency integer (min 1)",
	}
	if !equalLines(got, want) {
		t.Errorf("deep nesting mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_OneOf(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"required":["item"],
		"properties":{
			"item":{
				"oneOf":[
					{"type":"object","required":["price_id"],"properties":{
						"price_id":{"type":"string"},
						"quantity":{"type":"integer"}
					}},
					{"type":"object","required":["price"],"properties":{
						"price":{"type":"object","properties":{"amount":{"type":"string"}}},
						"quantity":{"type":"integer"}
					}}
				]
			}
		}
	}`
	got := schema.RenderTree(schemaJSON, style.Profile{})
	if !strings.Contains(got, "item") || !strings.Contains(got, "oneOf") {
		t.Fatalf("expected 'item oneOf' header, got:\n%s", got)
	}
	if !strings.Contains(got, "variant 1 (object)") || !strings.Contains(got, "variant 2 (object)") {
		t.Errorf("expected both variants labeled, got:\n%s", got)
	}
	if !strings.Contains(got, "price_id") || !strings.Contains(got, "price ") {
		t.Errorf("expected properties from both variants, got:\n%s", got)
	}
}

func TestRenderTree_AdditionalProperties(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"properties":{
			"custom_data":{
				"type":"object",
				"additionalProperties":{"type":"string"}
			}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	want := []string{
		"  custom_data object",
		"    <additional properties> string (any extra keys allowed)",
	}
	if !equalLines(got, want) {
		t.Errorf("additionalProperties mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_RecursiveSentinel(t *testing.T) {
	// The parser will emit a $ref sentinel for cycles; the renderer
	// renders it as a placeholder. Simulate the sentinel directly.
	schemaJSON := `{
		"type":"object",
		"properties":{
			"children":{
				"type":"array",
				"items":{"$ref":"#/components/schemas/Node"}
			}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	want := []string{
		"  children array[any]",
		"    (recursive reference to Node)",
	}
	if !equalLines(got, want) {
		t.Errorf("recursion sentinel mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_Format(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"properties":{
			"billed_at":{"type":"string","format":"date-time","description":"When it was billed."},
			"contact":{"type":"string","format":"email"},
			"url":{"type":"string","format":"uri"}
		}
	}`
	got := normaliseTree(schema.RenderTree(schemaJSON, style.Profile{}))
	want := []string{
		"  billed_at string (date-time) When it was billed.",
		"  contact string (email)",
		"  url string (uri)",
	}
	if !equalLines(got, want) {
		t.Errorf("format enrichment mismatch\nwant:\n%v\ngot:\n%v", want, got)
	}
}

func TestRenderTree_AppliesProfileColours(t *testing.T) {
	schemaJSON := `{"properties":{"amount":{"type":"integer","description":"Amount in minor units"}},"required":["amount"]}`
	out := schema.RenderTree(schemaJSON, style.Enabled())
	// Required marker via Warn → yellow (\x1b[33m).
	// Type via Dim → faint (\x1b[2m).
	// Field name via FieldName → bright blue (\x1b[94m).
	for _, seq := range []string{"\x1b[33m", "\x1b[2m", "\x1b[94m"} {
		if !strings.Contains(out, seq) {
			t.Errorf("expected ANSI %q in rendered tree:\n%s", seq, out)
		}
	}
	if !strings.Contains(out, "amount") {
		t.Errorf("field name missing from rendered tree:\n%s", out)
	}
}

func TestRenderTree_DisabledProfileIsPlain(t *testing.T) {
	schemaJSON := `{"properties":{"amount":{"type":"integer"}}}`
	out := schema.RenderTree(schemaJSON, style.Profile{})
	if strings.ContainsRune(out, '\x1b') {
		t.Errorf("disabled profile produced ANSI:\n%s", out)
	}
}
