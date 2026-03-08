package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var pauseCmd = &cobra.Command{
	Use:   "pause <ID>",
	Short: "Pause a download",
	Long:  `Pause a download by its ID. Use --all to pause all downloads.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mustInitializeGlobalState()

		all, _ := cmd.Flags().GetBool("all")

		if !all && len(args) == 0 {
			fmt.Fprintln(os.Stderr, "Error: provide a download ID or use --all")
			os.Exit(1)
		}


		if all {
			// TODO: Implement /pause-all endpoint or iterate
			fmt.Println("Pausing all downloads is not yet implemented for running server.")
			return
		}

		ExecuteAPIAction(args[0], "/pause", http.MethodPost, "Paused download")
	},
}

func init() {
	rootCmd.AddCommand(pauseCmd)
	pauseCmd.Flags().Bool("all", false, "Pause all downloads")
}
