package cmd

import (
	"context"

	"github.com/SurgeDM/Surge/internal/backup"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Preview or import a Surge bundle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initializeGlobalState(); err != nil {
			return err
		}

		apply, _ := cmd.Flags().GetBool("apply")
		replace, _ := cmd.Flags().GetBool("replace")
		rootDir, _ := cmd.Flags().GetString("root")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		transfer, isRemote, err := resolveTransferService()
		if err != nil {
			return err
		}

		previewOpts := backup.ImportOptions{
			RootDir: rootDir,
			Replace: replace,
		}
		preview, err := previewBundle(context.Background(), transfer, args[0], previewOpts)
		if err != nil {
			return err
		}

		if !apply {
			if jsonOutput {
				return printJSON(preview)
			}
			printImportPreview(preview)
			return nil
		}

		opts := previewOpts
		if isRemote {
			opts.SessionID = preview.SessionID
		}
		result, err := applyBundle(context.Background(), transfer, args[0], opts)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(result)
		}
		printImportResult(result)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().Bool("apply", false, "Apply the import after preview succeeds")
	importCmd.Flags().Bool("replace", false, "Replace existing Surge state instead of merging")
	importCmd.Flags().String("root", "", "Root directory for rebased imported paths")
	importCmd.Flags().Bool("json", false, "Output preview/result as JSON")
}
