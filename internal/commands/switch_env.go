package commands

import (
	"fmt"

	"github.com/PaddleHQ/paddle-cli/internal/config"

	"github.com/spf13/cobra"
)

func runSwitch(cmd *cobra.Command, root *Root, env string) error {
	if _, err := config.EnvironmentURL(env); err != nil {
		return err
	}

	cfgPath := root.Config.ConfigPath
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	cfg.CurrentEnvironment = env
	if err := config.Save(cfg, cfgPath); err != nil {
		return err
	}

	root.Environment = env
	root.BannerSuppressed = true
	profile := root.OutputOpts().Profile
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), profile.Success("Switched to")+` `+profile.EnvName(env))
	return nil
}

// NewSwitchCmd builds `paddle switch <environment>`.
func NewSwitchCmd(root *Root) *cobra.Command {
	return &cobra.Command{
		Use:     "switch <environment>",
		Short:   "Switch the active environment",
		GroupID: GroupIDSession,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSwitch(cmd, root, args[0])
		},
	}
}

// NewEnvShortcutCmd builds a `paddle <env>` shortcut equivalent to `paddle switch <env>`.
func NewEnvShortcutCmd(root *Root, env, description string) *cobra.Command {
	return &cobra.Command{
		Use:     env,
		Short:   description,
		GroupID: GroupIDSession,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSwitch(cmd, root, env)
		},
	}
}
