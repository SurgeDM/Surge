package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/surge-downloader/surge/internal/engine/state"
)

var rmCmd = &cobra.Command{
	Use:     "rm <ID>",
	Aliases: []string{"kill"},
	Short:   "Remove a download",
	Long:    `Remove a download by its ID. Use --clean to remove all completed downloads.`,
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mustInitializeGlobalState()

		clean, _ := cmd.Flags().GetBool("clean")

		if !clean && len(args) == 0 {
			fmt.Fprintln(os.Stderr, "Error: provide a download ID or use --clean")
			os.Exit(1)
		}

		if clean {
			// Remove completed downloads from DB
			count, err := state.RemoveCompletedDownloads()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error cleaning downloads: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Removed %d completed downloads.\n", count)
			return
		}

		_, _, err := resolveAPIConnection(true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		ExecuteAPIAction(args[0], "/delete", http.MethodPost, "Removed download")
	},
}

func init() {
	rootCmd.AddCommand(rmCmd)
	rmCmd.Flags().Bool("clean", false, "Remove all completed downloads")
}
