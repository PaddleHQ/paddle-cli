package output_test

import (
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/config/configtest"
	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
)

func TestParsePagination_HasMore(t *testing.T) {
	data := []byte(`{"data":[],"meta":{"pagination":{"per_page":50,"next":"ctm_456","has_more":true,"estimated_total":120}}}`)
	hasMore, cursor := output.ParsePagination(data)
	if !hasMore {
		t.Error("expected hasMore=true")
	}
	if cursor != "ctm_456" {
		t.Errorf("got cursor=%q, want %q", cursor, "ctm_456")
	}
}

func TestParsePagination_NextIsURL(t *testing.T) {
	data := []byte(`{"data":[],"meta":{"pagination":{"per_page":5,"next":"https://api.example.test/transactions?after=txn_01abc&per_page=5&status=completed","has_more":true}}}`)
	hasMore, cursor := output.ParsePagination(data)
	if !hasMore {
		t.Error("expected hasMore=true")
	}
	if cursor != "txn_01abc" {
		t.Errorf("got cursor=%q, want %q", cursor, "txn_01abc")
	}
}

func TestParsePagination_NoMore(t *testing.T) {
	data := []byte(`{"data":[],"meta":{"pagination":{"per_page":50,"has_more":false}}}`)
	hasMore, cursor := output.ParsePagination(data)
	if hasMore {
		t.Error("expected hasMore=false")
	}
	if cursor != "" {
		t.Errorf("got cursor=%q, want empty", cursor)
	}
}

func TestParsePagination_NoPaginationField(t *testing.T) {
	data := []byte(`{"data":{},"meta":{}}`)
	hasMore, cursor := output.ParsePagination(data)
	if hasMore {
		t.Error("expected hasMore=false for non-paginated response")
	}
	if cursor != "" {
		t.Errorf("got cursor=%q, want empty", cursor)
	}
}

func TestParsePagination_InvalidJSON(t *testing.T) {
	hasMore, cursor := output.ParsePagination([]byte("not json"))
	if hasMore || cursor != "" {
		t.Error("expected hasMore=false, cursor=empty for invalid JSON")
	}
}

func TestSynthesizeNextCommand_BasicList(t *testing.T) {
	root := cobra.Command{Use: "paddle"}
	customers := cobra.Command{Use: "customers"}
	list := cobra.Command{Use: "list"}
	list.Flags().String("after", "", "")
	list.Flags().String("status", "", "")
	list.Flags().Int("per-page", 50, "")

	customers.AddCommand(&list)
	root.AddCommand(&customers)

	_ = list.Flags().Set("status", "active")
	_ = list.Flags().Set("per-page", "10")

	got := output.SynthesizeNextCommand(&list, "ctm_abc")
	if !strings.Contains(got, "--status active") {
		t.Errorf("expected --status active in %q", got)
	}
	if !strings.Contains(got, "--per-page 10") {
		t.Errorf("expected --per-page 10 in %q", got)
	}
	if !strings.Contains(got, "--after ctm_abc") {
		t.Errorf("expected --after ctm_abc in %q", got)
	}
	if !strings.HasPrefix(got, "paddle customers list") {
		t.Errorf("expected command path prefix, got %q", got)
	}
}

func TestSynthesizeNextCommand_ReplacesExistingAfter(t *testing.T) {
	root := cobra.Command{Use: "paddle"}
	customers := cobra.Command{Use: "customers"}
	list := cobra.Command{Use: "list"}
	list.Flags().String("after", "", "")
	customers.AddCommand(&list)
	root.AddCommand(&customers)

	_ = list.Flags().Set("after", "ctm_old")

	got := output.SynthesizeNextCommand(&list, "ctm_new")
	if strings.Contains(got, "ctm_old") {
		t.Errorf("old cursor should be replaced; got %q", got)
	}
	if !strings.Contains(got, "--after ctm_new") {
		t.Errorf("expected new cursor in %q", got)
	}
}

func TestSynthesizeNextCommand_ShellQuotesMetachars(t *testing.T) {
	root := cobra.Command{Use: "paddle"}
	sellers := cobra.Command{Use: "sellers"}
	list := cobra.Command{Use: "list"}
	list.Flags().String("after", "", "")
	list.Flags().String("order-by", "", "")
	list.Flags().Int("per-page", 50, "")

	sellers.AddCommand(&list)
	root.AddCommand(&sellers)

	_ = list.Flags().Set("order-by", "id[DESC]")
	_ = list.Flags().Set("per-page", "10")

	got := output.SynthesizeNextCommand(&list, "326140")
	if !strings.Contains(got, `--order-by 'id[DESC]'`) {
		t.Errorf("expected quoted --order-by in %q", got)
	}
	if !strings.Contains(got, "--per-page 10") {
		t.Errorf("expected unquoted --per-page 10 in %q", got)
	}
}

func TestSynthesizeNextCommand_IncludesInheritedFlags(t *testing.T) {
	root := cobra.Command{Use: "paddle"}
	root.PersistentFlags().String("jq", "", "")
	root.PersistentFlags().String("environment", "", "")
	sellers := cobra.Command{Use: "sellers"}
	list := cobra.Command{Use: "list"}
	list.Flags().String("after", "", "")
	list.Flags().Int("per-page", 50, "")

	sellers.AddCommand(&list)
	root.AddCommand(&sellers)

	_ = root.PersistentFlags().Set("jq", ".data[] | {id}")
	_ = root.PersistentFlags().Set("environment", configtest.TestEnv)
	_ = list.Flags().Set("per-page", "10")

	// Trigger cobra's persistent-flag merge (normally done by Execute).
	_ = list.InheritedFlags()

	got := output.SynthesizeNextCommand(&list, "326140")
	if !strings.Contains(got, `--jq '.data[] | {id}'`) {
		t.Errorf("expected --jq in %q", got)
	}
	if !strings.Contains(got, "--environment test_env") {
		t.Errorf("expected --environment in %q", got)
	}
}

func TestSynthesizeNextCommand_OmitsSensitiveFlags(t *testing.T) {
	root := cobra.Command{Use: "paddle"}
	root.PersistentFlags().String("api-key", "", "")
	root.PersistentFlags().StringArray("header", nil, "")
	root.PersistentFlags().String("jq", "", "")
	for _, name := range []string{"api-key", "header"} {
		_ = root.PersistentFlags().SetAnnotation(name, output.FlagAnnotationSensitive, []string{"true"})
	}
	list := cobra.Command{Use: "list"}
	list.Flags().String("after", "", "")
	root.AddCommand(&list)

	_ = root.PersistentFlags().Set("api-key", "pdl_live_SECRET")
	_ = root.PersistentFlags().Set("header", "Authorization: Bearer TOKEN")
	_ = root.PersistentFlags().Set("header", "X-Debug: 1")
	_ = root.PersistentFlags().Set("jq", ".data")
	_ = list.InheritedFlags()

	got := output.SynthesizeNextCommand(&list, "ctm_123")
	for _, leaked := range []string{"--api-key", "pdl_live_SECRET", "--header", "Bearer TOKEN", "X-Debug"} {
		if strings.Contains(got, leaked) {
			t.Errorf("%q leaked into %q", leaked, got)
		}
	}
	if want := "paddle list --jq .data --after ctm_123"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWritePaginationHint_WritesHint(t *testing.T) {
	var buf strings.Builder
	output.WritePaginationHint(&buf, "paddle customers list --after ctm_123")
	got := buf.String()
	if !strings.Contains(got, "More results available") {
		t.Errorf("expected 'More results available' in %q", got)
	}
	if !strings.Contains(got, "paddle customers list --after ctm_123") {
		t.Errorf("expected command in %q", got)
	}
}

func TestWritePaginationHint_EmptyNextCmd_WritesNothing(t *testing.T) {
	var buf strings.Builder
	output.WritePaginationHint(&buf, "")
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty nextCmd, got %q", buf.String())
	}
}
