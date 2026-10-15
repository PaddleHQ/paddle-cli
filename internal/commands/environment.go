package commands

import (
	"encoding/json"
	"fmt"

	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
)

// fieldEnvironment is the JSON/column key carrying the active environment name.
const fieldEnvironment = "environment"

// NewEnvironmentCmd builds `paddle environment`, which shows the active environment and API URL.
func NewEnvironmentCmd(root *Root) *cobra.Command {
	return &cobra.Command{
		Use:     "environment",
		Aliases: []string{"env"},
		Short:   "Show the active environment and API URL",
		GroupID: GroupIDSession,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfgPath := root.Config.ConfigPath
			if cfgPath == "" {
				cfgPath = config.DefaultPath()
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			resolved, err := resolveWithFlags(cmd, cfg)
			if err != nil {
				return err
			}
			apiURL, _ := config.EnvironmentURL(resolved.CurrentEnvironment)

			payload, err := json.Marshal(map[string]string{
				fieldEnvironment: resolved.CurrentEnvironment,
				"api_url":        apiURL,
			})
			if err != nil {
				return fmt.Errorf("encoding environment: %w", err)
			}
			opts := root.OutputOpts()
			opts.Writer = cmd.OutOrStdout()
			return output.Print(payload, nil, opts)
		},
	}
}
