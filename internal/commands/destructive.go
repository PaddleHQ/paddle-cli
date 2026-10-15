package commands

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
)

// GuardDestructive runs the safety contract for an operation about to mutate
// or delete data. It must be called from inside a generated RunE *after*
// args validation and *before* the HTTP request fires.
//
// resourceID is the ID the user is acting on (typically args[0] of the
// cobra command for ops with a single path parameter). It's compared
// against the value of the --confirm flag for the script-friendly path.
// It does appear in error messages on rejection (mismatch / non-TTY),
// which is intentional: surfacing it helps the user diagnose typos.
//
// Returns nil to proceed; returns an ErrUsage-wrapped error (exit 2) for
// any rejection so callers don't need to know the specifics.
func GuardDestructive(cmd *cobra.Command, resourceID string) error {
	confirm, _ := cmd.Flags().GetString("confirm")

	if confirm != "" {
		if confirm == resourceID {
			return nil
		}
		cmd.SilenceUsage = true
		return fmt.Errorf("%w: --confirm value %q does not match resource ID %q", ErrUsage, confirm, resourceID)
	}

	nonInteractive, _ := cmd.Root().PersistentFlags().GetBool("non-interactive")
	if !output.IsStderrTTY() || nonInteractive {
		cmd.SilenceUsage = true
		return fmt.Errorf("%w: destructive operation requires --confirm=%s in non-interactive contexts", ErrUsage, resourceID)
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Are you sure? [y/N] ")
	reader := bufio.NewReader(cmd.InOrStdin())
	line, _ := reader.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "y" || answer == "yes" {
		return nil
	}
	cmd.SilenceUsage = true
	return fmt.Errorf("%w: aborted by user", ErrUsage)
}
