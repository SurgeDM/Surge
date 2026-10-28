package cmd

var pauseCmd = newLifecycleCommand("pause", "Pause a download")

func init() { rootCmd.AddCommand(pauseCmd) }
