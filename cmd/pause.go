package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

var pauseCmd = &cobra.Command{
	Use:   "pause <ID>",
	Short: "Pause a download",
	Long:  `Pause a download by its ID. Use --all to pause all downloads.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initializeGlobalState(); err != nil {
			return err
		}

		all, _ := cmd.Flags().GetBool("all")

		if !all && len(args) == 0 {
			return fmt.Errorf("provide a download ID or use --all")
		}

		if all {
			// TODO: Implement /pause-all endpoint or iterate
			fmt.Println("Pausing all downloads is not yet implemented for running server.")
			return nil
		}

		return ExecuteAPIAction(args[0], "/pause", http.MethodPost, "Paused download")
	},
}

func init() {
	rootCmd.AddCommand(pauseCmd)
	pauseCmd.Flags().Bool("all", false, "Pause all downloads")
}
