package cmd

import (
	"fmt"

	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:     "add [url]...",
	Aliases: []string{"get"},
	Short:   "Add a new download to the running Surge instance",
	Long:    `Add one or more URLs to the download queue of a running Surge instance.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		//initializeGlobally is required to ensure that the config and logger are set up before we attempt to resolve the API connection or read the batch file.
		if err := initializeGlobalState(); err != nil {
			return err
		}

		batchFile, _ := cmd.Flags().GetString("batch")
		output, _ := cmd.Flags().GetString("output")
		confirm, _ := cmd.Flags().GetBool("confirm")

		var urls []string
		urls = append(urls, args...)

		// 2. URLs from batch file
		if batchFile != "" {
			fileUrls, err := utils.ReadURLsFromFile(batchFile)
			if err != nil {
				return fmt.Errorf("error reading batch file: %w", err)
			}
			urls = append(urls, fileUrls...)
		}

		if len(urls) == 0 {
			_ = cmd.Help()
			return nil
		}

		baseURL, token, err := resolveAPIConnection(true)
		if err != nil {
			return err
		}
		resolvedOutput := resolveClientOutputPath(output)

		if batchFile != "" && confirm {
			if err := sendBatchToServer(urls, resolvedOutput, baseURL, token, false); err != nil {
				return err
			}
			fmt.Printf("Batch confirmation requested for %d downloads.\n", len(urls))
			return nil
		}

		// Send downloads to server
		count := 0
		attempted := 0
		for _, arg := range urls {
			url, mirrors := ParseURLArg(arg)
			if url == "" {
				continue
			}
			attempted++
			if err := sendToServerWithApproval(url, mirrors, resolvedOutput, baseURL, token, !confirm); err != nil {
				fmt.Printf("Error adding %s: %v\n", url, err)
				continue
			}
			count++
		}

		if count > 0 {
			fmt.Printf("Successfully added %d downloads.\n", count)
			return nil
		}

		if attempted > 0 {
			return fmt.Errorf("failed to add any downloads")
		}

		return fmt.Errorf("no valid URLs to add")
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringP("batch", "b", "", "File containing URLs to download (one per line)")
	addCmd.Flags().StringP("output", "o", "", "Output directory (defaults to current working directory)")
	addCmd.Flags().Bool("confirm", false, "Show confirmation prompt before starting downloads")
}
