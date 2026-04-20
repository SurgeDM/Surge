package cmd

import (
	"context"
	"fmt"

	"github.com/SurgeDM/Surge/internal/backup"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export <file>",
	Short: "Export Surge data to a bundle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initializeGlobalState(); err != nil {
			return err
		}

		includeLogs, _ := cmd.Flags().GetBool("include-logs")
		includePartials, _ := cmd.Flags().GetBool("include-partials")
		leavePaused, _ := cmd.Flags().GetBool("leave-paused")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		transfer, _, err := resolveTransferService()
		if err != nil {
			return err
		}

		manifest, err := exportBundle(context.Background(), transfer, args[0], backup.ExportOptions{
			IncludeLogs:     includeLogs,
			IncludePartials: includePartials,
			LeavePaused:     leavePaused,
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(manifest)
		}
		fmt.Printf("Exported bundle to %s\n", ensureExportPath(args[0]))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
	exportCmd.Flags().Bool("include-logs", false, "Include Surge log files in the export bundle")
	exportCmd.Flags().Bool("include-partials", false, "Include paused .surge partial files for resumable restore")
	exportCmd.Flags().Bool("leave-paused", false, "Leave downloads paused after export")
	exportCmd.Flags().Bool("json", false, "Output manifest as JSON")
}

