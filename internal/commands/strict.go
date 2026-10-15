package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// ErrUsage signals that the user invoked the CLI incorrectly: unknown
// subcommand, wrong number of positional args, or any other arg-validation
// failure. main() exits with code 2 for any error wrapping this sentinel.
var ErrUsage = errors.New("usage error")

// usageError makes err exit 2 like ErrUsage without prefixing its message.
type usageError struct{ error }

func (e usageError) Is(target error) bool { return target == ErrUsage }
func (e usageError) Unwrap() error        { return e.error }

// ErrUnknownSubcommand is the more specific kind of usage error: the user
// supplied a subcommand name that isn't registered. Wraps ErrUsage so a
// single errors.Is(err, ErrUsage) check covers both kinds.
var ErrUnknownSubcommand = fmt.Errorf("%w: unknown subcommand", ErrUsage)

// EnforceUnknownSubcommandError walks cmd's subtree and installs a strict
// RunE on every command that has subcommands but no Run/RunE. Without this,
// Cobra's default behavior for parent-with-no-Run is to print help and
// return nil, meaning `paddle customers subfoo` exits 0 and silently
// dumps help instead of erroring.
//
// Commands that already have Run/RunE are left alone. Leaf commands (no
// subcommands) are also left alone.
func EnforceUnknownSubcommandError(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		EnforceUnknownSubcommandError(sub)
	}
	if !cmd.HasSubCommands() {
		return
	}
	if cmd.Run != nil || cmd.RunE != nil {
		return
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return c.Help()
		}
		c.SilenceUsage = true
		return fmt.Errorf("%w %q for %q", ErrUnknownSubcommand, args[0], c.CommandPath())
	}
	// For root-level commands (no parent), Cobra's legacyArgs fires during
	// traverse before RunE and returns its own unwrapped error. Setting Args
	// to ArbitraryArgs suppresses that early check, allowing RunE to handle
	// the unknown arg and return ErrUnknownSubcommand properly.
	if !cmd.HasParent() {
		cmd.Args = cobra.ArbitraryArgs
	}
}

// EnforceArgsExitCode walks cmd's subtree and wraps each command's Args
// validator so arg-validation failures (e.g. "accepts 0 arg(s), received 1"
// from cobra.NoArgs / cobra.ExactArgs) surface as ErrUsage, which main()
// recognizes as a usage error and exits 2. Without this wrapper Cobra's
// validation errors fall through to the generic exit 1.
//
// Commands without an Args validator are left alone. The walker must run
// after EnforceUnknownSubcommandError so the strict-RunE-installed
// ArbitraryArgs on the root doesn't get re-wrapped (ArbitraryArgs never
// errors, so wrapping is a no-op anyway, but skipping keeps intent clear).
func EnforceArgsExitCode(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		EnforceArgsExitCode(sub)
	}
	if cmd.Args == nil {
		return
	}
	original := cmd.Args
	cmd.Args = func(c *cobra.Command, args []string) error {
		if err := original(c, args); err != nil {
			c.SilenceUsage = true
			return fmt.Errorf("%w: %w", ErrUsage, err)
		}
		return nil
	}
}
