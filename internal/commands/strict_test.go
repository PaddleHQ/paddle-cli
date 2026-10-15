package commands_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/commands"

	"github.com/spf13/cobra"
)

func newStrictTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "paddle", SilenceErrors: true, SilenceUsage: true}

	parent := &cobra.Command{Use: "customers"}
	parent.AddCommand(&cobra.Command{
		Use:  "list",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	root.AddCommand(parent)

	commands.EnforceUnknownSubcommandError(root)
	return root
}

func TestEnforceUnknownSubcommand_UnknownChild(t *testing.T) {
	root := newStrictTestRoot()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"customers", "subfoo"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, commands.ErrUnknownSubcommand) {
		t.Errorf("expected ErrUnknownSubcommand, got %v", err)
	}
	if !strings.Contains(err.Error(), `"subfoo"`) {
		t.Errorf("error should mention the unknown subcommand, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "paddle customers") {
		t.Errorf("error should mention the parent path, got %q", err.Error())
	}
}

func TestEnforceUnknownSubcommand_ParentNoArgsPrintsHelp(t *testing.T) {
	root := newStrictTestRoot()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"customers"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected help output to mention 'list' subcommand, got %q", stdout.String())
	}
}

func TestEnforceUnknownSubcommand_LeafStillRuns(t *testing.T) {
	root := newStrictTestRoot()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"customers", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnforceUnknownSubcommand_RootLevelUnknown(t *testing.T) {
	root := newStrictTestRoot()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"nonsense"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown root-level command")
	}
	if !errors.Is(err, commands.ErrUnknownSubcommand) {
		t.Errorf("expected ErrUnknownSubcommand, got %v", err)
	}
}

func TestEnforceUnknownSubcommand_DoesNotOverrideExistingRunE(t *testing.T) {
	called := false
	parent := &cobra.Command{
		Use: "thing",
		RunE: func(_ *cobra.Command, _ []string) error {
			called = true
			return nil
		},
	}
	parent.AddCommand(&cobra.Command{Use: "child", RunE: func(_ *cobra.Command, _ []string) error { return nil }})
	root := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(parent)

	commands.EnforceUnknownSubcommandError(root)

	root.SetArgs([]string{"thing", "anything"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("existing RunE was overridden: walker must skip commands that already have Run/RunE")
	}
}

func TestErrUnknownSubcommandWrapsErrUsage(t *testing.T) {
	if !errors.Is(commands.ErrUnknownSubcommand, commands.ErrUsage) {
		t.Error("ErrUnknownSubcommand must wrap ErrUsage so a single ErrUsage check covers both")
	}
}

func TestEnforceArgsExitCode_NoArgsLeafRejectsExtra(t *testing.T) {
	root := &cobra.Command{Use: "paddle", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{
		Use:  "leaf",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	commands.EnforceArgsExitCode(root)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"leaf", "extra"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when leaf with NoArgs receives extra arg, got nil")
	}
	if !errors.Is(err, commands.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}
}

func TestEnforceArgsExitCode_ExactArgsCountWrong(t *testing.T) {
	root := &cobra.Command{Use: "paddle", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{
		Use:  "needs-one",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	commands.EnforceArgsExitCode(root)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"needs-one", "a", "b"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when ExactArgs(1) receives 2, got nil")
	}
	if !errors.Is(err, commands.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}
}

func TestEnforceArgsExitCode_ValidArgsPassThrough(t *testing.T) {
	called := false
	root := &cobra.Command{Use: "paddle", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{
		Use:  "needs-one",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			called = true
			return nil
		},
	})
	commands.EnforceArgsExitCode(root)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"needs-one", "the-id"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error on valid args: %v", err)
	}
	if !called {
		t.Error("RunE was not called despite valid args: wrapper should pass through on success")
	}
}

func TestEnforceArgsExitCode_NilArgsLeftAlone(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "noargsvalidator",
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	}
	root := &cobra.Command{Use: "paddle", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(cmd)
	commands.EnforceArgsExitCode(root)

	if cmd.Args != nil {
		t.Error("walker installed an Args wrapper on a command with no original validator: should leave it nil")
	}
}

func TestEnforceArgsExitCode_ComposesWithStrictWalker(t *testing.T) {
	parent := &cobra.Command{Use: "thing"}
	parent.AddCommand(&cobra.Command{
		Use:  "leaf",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return nil },
	})
	root := &cobra.Command{Use: "paddle", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(parent)

	commands.EnforceUnknownSubcommandError(root)
	commands.EnforceArgsExitCode(root)

	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"thing", "leaf", "extra"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error from leaf NoArgs + extra arg, got nil")
	}
	if !errors.Is(err, commands.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v (msg=%q)", err, err.Error())
	}
}
