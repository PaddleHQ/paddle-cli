// Command paddle is the Paddle CLI.
package main

import (
	"fmt"
	"os"

	"github.com/PaddleHQ/paddle-cli/internal/client"
	"github.com/PaddleHQ/paddle-cli/internal/commands"
	"github.com/PaddleHQ/paddle-cli/internal/generated"
	"github.com/PaddleHQ/paddle-cli/internal/output"
)

func main() {
	root := commands.NewRoot(commands.RootConfig{})

	generated.RegisterAll(root.Cmd, root.ClientPtr, root.OutputPtr)

	root.Cmd.AddCommand(commands.NewWhoamiCmd(root))
	root.Cmd.AddCommand(commands.NewVersionCmd(root))
	root.Cmd.AddCommand(commands.NewEnvironmentCmd(root))
	root.Cmd.AddCommand(commands.NewSwitchCmd(root))
	root.Cmd.AddCommand(commands.NewAPICmd(root))
	root.Cmd.AddCommand(commands.NewLoginCmd(root))
	root.Cmd.AddCommand(commands.NewLogoutCmd(root))
	root.Cmd.AddCommand(commands.NewConfigCmd(root))
	root.Cmd.AddCommand(commands.NewOutputCmd(root))
	commands.RegisterEnvShortcuts(root)

	// Generated resource commands don't set a GroupID; bucket them here so
	// they render under "Resources:" in --help without touching codegen.
	commands.AssignDefaultGroup(root.Cmd, commands.GroupIDResources)

	// Make `paddle <parent> <unknown>` exit 2 instead of silently dumping help,
	// and surface arg-validation failures (cobra.NoArgs, cobra.ExactArgs, etc.)
	// as ErrUsage so they exit 2 too rather than the generic 1.
	commands.EnforceUnknownSubcommandError(root.Cmd)
	commands.EnforceArgsExitCode(root.Cmd)
	commands.InstallHelpTopics(root)

	err := root.Cmd.Execute()

	writeBanner := func() {
		output.WriteBanner(os.Stderr, output.BannerOptions{
			Environment: root.Environment,
			Suppressed:  root.BannerSuppressed,
			StderrTTY:   output.IsStderrTTY(),
			Profile:     root.OutputOpts().Profile,
		})
	}

	if err != nil {
		code, apiErr := client.ExitCodeFor(err, commands.ErrUsage)
		if apiErr != nil {
			commands.WriteAPIError(os.Stderr, apiErr, root.OutputOpts())
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		writeBanner()
		output.WritePaginationHint(os.Stderr, root.OutputPtr.NextCommand)
		os.Exit(code)
	}

	writeBanner()
	output.WritePaginationHint(os.Stderr, root.OutputPtr.NextCommand)
}
