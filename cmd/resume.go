package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <ID>",
	Short: "Resume a paused download",
	Long:  `Resume a paused download by its ID. Use --all to resume all paused downloads.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mustInitializeGlobalState()

		all, _ := cmd.Flags().GetBool("all")

		if !all && len(args) == 0 {
			fmt.Fprintln(os.Stderr, "Error: provide a download ID or use --all")
			os.Exit(1)
		}

		if all {
			fmt.Println("Resuming all downloads is not yet implemented for running server.")
			return
		}

		ExecuteAPIAction(args[0], "/resume", http.MethodPost, "Resumed download")
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().Bool("all", false, "Resume all paused downloads")
}
