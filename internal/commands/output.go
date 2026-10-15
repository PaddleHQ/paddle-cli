package commands

import (
	"fmt"

	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
)

// NewOutputCmd is a shortcut for `paddle config set default_output <value>`.
// Parallels the environment shortcuts (paddle sandbox, paddle production, …).
func NewOutputCmd(root *Root) *cobra.Command {
	return &cobra.Command{
		Use:     "output <table|json|csv>",
		Short:   "Set the default output format",
		GroupID: GroupIDConfig,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value := args[0]
			if _, err := output.ParseFormat(value); err != nil {
				return err
			}

			path := configPath(root)
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			cfg.DefaultOutput = value
			if err := config.Save(cfg, path); err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), root.OutputOpts().Profile.Success("Default output set to "+value))
			return nil
		},
	}
}
