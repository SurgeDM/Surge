package cmd

import (
	"fmt"
	"net/http"

	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:     "rm <ID>",
	Aliases: []string{"kill"},
	Short:   "Remove a download",
	Long:    `Remove a download by its ID. Use --clean to remove all completed downloads.`,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initializeGlobalState(); err != nil {
			return err
		}

		clean, _ := cmd.Flags().GetBool("clean")

		if !clean && len(args) == 0 {
			return fmt.Errorf("provide a download ID or use --clean")
		}

		if clean {
			// Remove completed downloads from DB
			count, err := state.RemoveCompletedDownloads()
			if err != nil {
				return fmt.Errorf("error cleaning downloads: %w", err)
			}
			fmt.Printf("Removed %d completed downloads.\n", count)
			return nil
		}

		return ExecuteAPIAction(args[0], "/delete", http.MethodPost, "Removed download")
	},
}

func init() {
	rootCmd.AddCommand(rmCmd)
	rmCmd.Flags().Bool("clean", false, "Remove all completed downloads")
}
