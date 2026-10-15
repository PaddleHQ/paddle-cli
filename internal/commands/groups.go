package commands

import "github.com/spf13/cobra"

// Command group IDs used to bucket subcommands in `paddle --help` output.
const (
	GroupIDResources = "resources"
	GroupIDSession   = "session"
	GroupIDConfig    = "config"
	GroupIDOther     = "other"
)

// AddCommandGroups registers the four top-level command groups on the root
// command. Individual commands opt into a group by setting `GroupID` on their
// *cobra.Command. Generated resource commands are bucketed in bulk via
// AssignDefaultGroup after registration, so the codegen template doesn't need
// to know about groups.
func AddCommandGroups(cmd *cobra.Command) {
	cmd.AddGroup(
		&cobra.Group{ID: GroupIDSession, Title: "Session:"},
		&cobra.Group{ID: GroupIDResources, Title: "Resources:"},
		&cobra.Group{ID: GroupIDConfig, Title: "Configuration:"},
		&cobra.Group{ID: GroupIDOther, Title: "Other:"},
	)
	cmd.SetHelpCommandGroupID(GroupIDOther)
	cmd.SetCompletionCommandGroupID(GroupIDOther)
}

// AssignDefaultGroup sets GroupID on every direct subcommand of cmd that
// doesn't already have one. Call this after all commands have been registered.
func AssignDefaultGroup(cmd *cobra.Command, groupID string) {
	for _, c := range cmd.Commands() {
		if c.GroupID == "" {
			c.GroupID = groupID
		}
	}
}
