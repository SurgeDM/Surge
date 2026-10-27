package cmd

var resumeCmd = newLifecycleCommand("resume", "Resume a paused download")

func init() { rootCmd.AddCommand(resumeCmd) }
