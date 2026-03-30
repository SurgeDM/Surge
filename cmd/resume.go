package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <ID>",
	Short: "Resume a paused download",
	Long:  `Resume a paused download by its ID. Use --all to resume all paused downloads.`,
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
			fmt.Println("Resuming all downloads is not yet implemented for running server.")
			return nil
		}

		return ExecuteAPIAction(args[0], "/resume", http.MethodPost, "Resumed download")
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().Bool("all", false, "Resume all paused downloads")
}
