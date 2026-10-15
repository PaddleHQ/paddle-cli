package commands

import (
	"encoding/json"
	"fmt"

	"github.com/PaddleHQ/paddle-cli/internal/build"
	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
)

// NewVersionCmd builds `paddle version`, which prints the build version and commit.
func NewVersionCmd(root *Root) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Show version and commit",
		GroupID: GroupIDOther,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := json.Marshal(map[string]string{
				"version": build.Version,
				"commit":  build.Commit,
			})
			if err != nil {
				return fmt.Errorf("encoding version: %w", err)
			}
			opts := root.OutputOpts()
			opts.Writer = cmd.OutOrStdout()
			return output.Print(payload, nil, opts)
		},
	}
}
