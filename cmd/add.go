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

		summary := submitDownloads(urls, resolvedOutput, baseURL, token, confirm)

		if summary.queued > 0 {
			fmt.Printf("Successfully added %d downloads.\n", summary.queued)
		}
		if summary.awaitingApproval > 0 {
			fmt.Printf("Confirmation requested for %d downloads.\n", summary.awaitingApproval)
		}
		if summary.succeeded() > 0 {
			return nil
		}

		if summary.failed > 0 {
			return fmt.Errorf("failed to add any downloads")
		}

		return fmt.Errorf("no valid URLs to add")
	},
}

type addSummary struct {
	queued           int
	awaitingApproval int
	failed           int
}

func (s addSummary) succeeded() int {
	return s.queued + s.awaitingApproval
}

func submitDownloads(urls []string, output, baseURL, token string, confirm bool) addSummary {
	var summary addSummary
	for _, arg := range urls {
		url, mirrors, err := parseAndNormalizeURLArg(arg)
		if err != nil {
			fmt.Printf("Error adding %s: %v\n", arg, err)
			summary.failed++
			continue
		}
		if url == "" {
			continue
		}

		pendingApproval, err := sendToServerWithApproval(url, mirrors, output, baseURL, token, !confirm)
		if err != nil {
			fmt.Printf("Error adding %s: %v\n", url, err)
			summary.failed++
			continue
		}
		if pendingApproval {
			summary.awaitingApproval++
		} else {
			summary.queued++
		}
	}
	return summary
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringP("batch", "b", "", "File containing URLs to download (one per line)")
	addCmd.Flags().StringP("output", "o", "", "Output directory (defaults to current working directory)")
	addCmd.Flags().Bool("confirm", false, "Show confirmation prompt before starting downloads")
}
