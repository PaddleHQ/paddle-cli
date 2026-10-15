package commands_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
)

func newDestructiveTestCmd(t *testing.T, stdin io.Reader) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "cancel"}
	cmd.Flags().String("confirm", "", "")
	if stdin != nil {
		cmd.SetIn(stdin)
	}
	return cmd
}

func TestGuardDestructive_FlagMatch(t *testing.T) {
	original := output.IsStderrTTY
	output.IsStderrTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStderrTTY = original })

	cmd := newDestructiveTestCmd(t, nil)
	if err := cmd.Flags().Set("confirm", "sub_01"); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if err := commands.GuardDestructive(cmd, "sub_01"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr write on flag-match, got %q", stderr.String())
	}
}

func TestGuardDestructive_FlagMismatch(t *testing.T) {
	original := output.IsStderrTTY
	output.IsStderrTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStderrTTY = original })

	cmd := newDestructiveTestCmd(t, nil)
	if err := cmd.Flags().Set("confirm", "sub_typo"); err != nil {
		t.Fatal(err)
	}

	err := commands.GuardDestructive(cmd, "sub_01")
	if err == nil {
		t.Fatal("expected error on mismatch")
	}
	if !errors.Is(err, commands.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}
	if !strings.Contains(err.Error(), "sub_typo") || !strings.Contains(err.Error(), "sub_01") {
		t.Errorf("error should mention both supplied and expected IDs, got %q", err.Error())
	}
}

func TestGuardDestructive_NonTTYWithoutFlag(t *testing.T) {
	original := output.IsStderrTTY
	output.IsStderrTTY = func() bool { return false }
	t.Cleanup(func() { output.IsStderrTTY = original })

	cmd := newDestructiveTestCmd(t, nil)

	err := commands.GuardDestructive(cmd, "sub_01")
	if err == nil {
		t.Fatal("expected error in non-TTY mode without --confirm")
	}
	if !errors.Is(err, commands.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}
	if !strings.Contains(err.Error(), "--confirm") {
		t.Errorf("error should advise --confirm, got %q", err.Error())
	}
}

func TestGuardDestructive_TTYPromptAccepts(t *testing.T) {
	original := output.IsStderrTTY
	output.IsStderrTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStderrTTY = original })

	cmd := newDestructiveTestCmd(t, strings.NewReader("y\n"))
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if err := commands.GuardDestructive(cmd, "sub_01"); err != nil {
		t.Fatalf("expected nil after y/Y prompt, got %v", err)
	}
	if !strings.Contains(stderr.String(), "Are you sure") {
		t.Errorf("expected prompt on stderr, got %q", stderr.String())
	}
}

func TestGuardDestructive_TTYPromptRejects(t *testing.T) {
	original := output.IsStderrTTY
	output.IsStderrTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStderrTTY = original })

	for _, input := range []string{"\n", "n\n", "no\n", "anything\n"} {
		cmd := newDestructiveTestCmd(t, strings.NewReader(input))
		var stderr bytes.Buffer
		cmd.SetErr(&stderr)

		err := commands.GuardDestructive(cmd, "sub_01")
		if err == nil {
			t.Errorf("input %q: expected non-nil error on rejection", input)
			continue
		}
		if !errors.Is(err, commands.ErrUsage) {
			t.Errorf("input %q: expected ErrUsage, got %v", input, err)
		}
		if !strings.Contains(err.Error(), "aborted") {
			t.Errorf("input %q: expected error to mention 'aborted', got %q", input, err.Error())
		}
	}
}

func TestGuardDestructive_TTYPromptCaseInsensitive(t *testing.T) {
	original := output.IsStderrTTY
	output.IsStderrTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStderrTTY = original })

	for _, input := range []string{"y\n", "Y\n", "yes\n", "YES\n", "Yes\n"} {
		cmd := newDestructiveTestCmd(t, strings.NewReader(input))
		cmd.SetErr(&bytes.Buffer{})

		if err := commands.GuardDestructive(cmd, "sub_01"); err != nil {
			t.Errorf("input %q: expected nil, got %v", input, err)
		}
	}
}

func TestGuardDestructive_NonInteractiveFlag_RejectsWithoutConfirm(t *testing.T) {
	original := output.IsStderrTTY
	output.IsStderrTTY = func() bool { return true }
	t.Cleanup(func() { output.IsStderrTTY = original })

	root := commands.NewRoot(commands.RootConfig{})
	if err := root.Cmd.PersistentFlags().Set("non-interactive", "true"); err != nil {
		t.Fatalf("setting flag: %v", err)
	}

	child := &cobra.Command{Use: "delete <id>"}
	child.Flags().String("confirm", "", "")
	root.Cmd.AddCommand(child)

	err := commands.GuardDestructive(child, "sub_123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, commands.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}
}
