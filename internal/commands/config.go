package commands

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/PaddleHQ/paddle-cli/internal/config"
	"github.com/PaddleHQ/paddle-cli/internal/output"

	"github.com/spf13/cobra"
)

var (
	errInvalidColor     = errors.New("must be one of auto, always, never")
	errUnknownConfigKey = errors.New("unknown config key")
)

// configKey describes a user-settable entry in the config file. Token fields
// and environment state are intentionally excluded: those are owned by the
// login/logout/switch commands.
type configKey struct {
	name  string
	get   func(c *config.Config) string
	set   func(c *config.Config, v string) error
	unset func(c *config.Config)
}

var configKeys = map[string]configKey{
	"color": {
		name: "color",
		get:  func(c *config.Config) string { return c.Color },
		set: func(c *config.Config, v string) error {
			switch v {
			case "auto", "always", "never":
				c.Color = v
				return nil
			}
			return errInvalidColor
		},
		unset: func(c *config.Config) { c.Color = "" },
	},
	"default_output": {
		name: "default_output",
		get:  func(c *config.Config) string { return c.DefaultOutput },
		set: func(c *config.Config, v string) error {
			if _, err := output.ParseFormat(v); err != nil {
				return err
			}
			c.DefaultOutput = v
			return nil
		},
		unset: func(c *config.Config) { c.DefaultOutput = "" },
	},
	"api_version": {
		name: "api_version",
		get:  func(c *config.Config) string { return c.APIVersion },
		set: func(c *config.Config, v string) error {
			c.APIVersion = v
			return nil
		},
		unset: func(c *config.Config) { c.APIVersion = "" },
	},
}

func knownKeysList() string {
	names := make([]string, 0, len(configKeys))
	for k := range configKeys {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func resolveKey(name string) (configKey, error) {
	k, ok := configKeys[name]
	if !ok {
		return configKey{}, fmt.Errorf("%w %q (known: %s)", errUnknownConfigKey, name, knownKeysList())
	}
	return k, nil
}

func configPath(root *Root) string {
	if root.Config.ConfigPath != "" {
		return root.Config.ConfigPath
	}
	return config.DefaultPath()
}

// NewConfigCmd builds `paddle config` and its get/set/unset/list subcommands.
func NewConfigCmd(root *Root) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "View or modify CLI config values",
		GroupID: GroupIDConfig,
		Long: fmt.Sprintf(`View or modify values in the Paddle CLI config file (%s).

Settable keys: %s

Other fields (environments, tokens, current environment) are managed by the
login, logout, and switch commands.`, config.DefaultPath(), knownKeysList()),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigList(cmd, root)
		},
	}

	cmd.AddCommand(
		newConfigGetCmd(root),
		newConfigSetCmd(root),
		newConfigUnsetCmd(root),
		newConfigListCmd(root),
	)

	return cmd
}

func newConfigGetCmd(root *Root) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print the current value of a config key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveKey(args[0])
			if err != nil {
				return err
			}
			cfg, err := config.Load(configPath(root))
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), key.get(cfg))
			return nil
		},
	}
}

func newConfigSetCmd(root *Root) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config key to a value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveKey(args[0])
			if err != nil {
				return err
			}
			path := configPath(root)
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			if err := key.set(cfg, args[1]); err != nil {
				return fmt.Errorf("invalid value for %s: %w", key.name, err)
			}
			if err := config.Save(cfg, path); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), root.OutputOpts().Profile.Success(fmt.Sprintf("Set %s = %s", key.name, args[1])))
			return nil
		},
	}
}

func newConfigUnsetCmd(root *Root) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Clear a config key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := resolveKey(args[0])
			if err != nil {
				return err
			}
			path := configPath(root)
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			key.unset(cfg)
			if err := config.Save(cfg, path); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), root.OutputOpts().Profile.Success("Unset "+key.name))
			return nil
		},
	}
}

func newConfigListCmd(root *Root) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all settable config keys and their current values",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigList(cmd, root)
		},
	}
}

func runConfigList(cmd *cobra.Command, root *Root) error {
	cfg, err := config.Load(configPath(root))
	if err != nil {
		return err
	}
	profile := root.OutputOpts().Profile
	names := make([]string, 0, len(configKeys))
	for k := range configKeys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, n := range names {
		v := configKeys[n].get(cfg)
		if v == "" {
			v = "<unset>"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", profile.FieldName(fmt.Sprintf("%-16s", n)), profile.Dim(v))
	}
	return nil
}
