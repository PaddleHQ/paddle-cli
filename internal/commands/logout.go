package commands

import (
	"errors"
	"fmt"

	"github.com/PaddleHQ/paddle-cli/internal/config"

	"github.com/spf13/cobra"
)

var errNotLoggedIn = errors.New("not logged in, nothing to log out of")

// NewLogoutCmd builds `paddle logout`, which clears stored credentials for the current environment.
func NewLogoutCmd(root *Root) *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Short:   "Clear stored credentials for the current environment",
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
			env := resolved.CurrentEnvironment
			if _, ok := cfg.Environments[env]; !ok {
				return fmt.Errorf("%w (environment %q)", errNotLoggedIn, env)
			}

			delete(cfg.Environments, env)
			if err := config.Save(cfg, cfgPath); err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), root.OutputOpts().Profile.Success("Logged out of "+env))
			return nil
		},
	}
}
