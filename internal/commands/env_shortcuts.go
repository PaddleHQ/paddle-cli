package commands

import "github.com/PaddleHQ/paddle-cli/internal/config"

// RegisterEnvShortcuts adds a `paddle <env>` command for every configured
// environment, equivalent to `paddle switch <env>`.
func RegisterEnvShortcuts(root *Root) {
	for _, env := range config.ValidEnvironments() {
		root.Cmd.AddCommand(NewEnvShortcutCmd(root, env, "Switch to "+env+" environment"))
	}
}
