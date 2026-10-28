package cmd

import "github.com/spf13/cobra"

// Install after all command init functions have run. Help and completion use
// metadata only, and never initialize an engine or query download history.
func configureCommandHelp() {
	if len(rootCmd.Groups()) > 0 {
		return
	}
	rootCmd.AddGroup(&cobra.Group{ID: "download", Title: "Download:"}, &cobra.Group{ID: "manage", Title: "Manage:"}, &cobra.Group{ID: "configure", Title: "Configure and inspect:"})
	ui := &cobra.Command{Use: "ui", Short: "Open the interactive download manager", GroupID: "download", Args: cobra.NoArgs, RunE: rootCmd.RunE}
	ui.Flags().AddFlagSet(rootCmd.LocalNonPersistentFlags())
	rootCmd.AddCommand(ui)
	for _, command := range rootCmd.Commands() {
		switch command.Name() {
		case "add", "fetch", "ui", "connect":
			command.GroupID = "download"
		case "ls", "wait", "events", "pause", "resume", "cancel", "rm", "refresh", "limit":
			command.GroupID = "manage"
		default:
			command.GroupID = "configure"
		}
		if command.ValidArgsFunction == nil {
			command.ValidArgsFunction = func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}
	}
	rootCmd.SetHelpCommandGroupID("configure")
	rootCmd.SetCompletionCommandGroupID("configure")
}
