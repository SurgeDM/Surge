package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/spf13/cobra"
)

var pauseCmd = &cobra.Command{
	Use:   "pause <ID> | --all",
	Short: "Pause a download",
	Long:  `Pause a download by its ID. Use --all to pause all downloads.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		all, err := cmd.Flags().GetBool("all")
		if err != nil {
			return fmt.Errorf("read --all flag: %w", err)
		}

		if !all && len(args) == 0 {
			return fmt.Errorf("provide a download ID or use --all")
		}
		if all && len(args) != 0 {
			return fmt.Errorf("--all cannot be used with a download ID")
		}

		if err := initializeGlobalState(); err != nil {
			return err
		}

		if all {
			return pauseAllDownloads()
		}

		return ExecuteAPIAction(args[0], "/pause", http.MethodPost, "Paused download")
	},
}

// pauseAllDownloads pauses every non-completed download reported by the running
// server. The API only supports pausing one download at a time, so this keeps
// the command compatible with older servers while still making --all useful.
func pauseAllDownloads() error {
	baseURL, token, err := resolveAPIConnection(true)
	if err != nil {
		return fmt.Errorf("failed to connect to Surge server: %w", err)
	}

	downloads, err := GetRemoteDownloads(baseURL, token)
	if err != nil {
		return fmt.Errorf("failed to list downloads: %w", err)
	}

	ids := make([]string, 0, len(downloads))
	for _, download := range downloads {
		if download.Status != "completed" {
			ids = append(ids, download.ID)
		}
	}
	sort.Strings(ids)

	if len(ids) == 0 {
		fmt.Println("No downloads to pause.")
		return nil
	}

	var failures []error
	paused := 0
	for _, id := range ids {
		resp, err := doAPIRequest(http.MethodPost, baseURL, token, "/pause?id="+url.QueryEscape(id), nil)
		if err != nil {
			failures = append(failures, fmt.Errorf("pause %s: %w", id, err))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			failures = append(failures, fmt.Errorf("pause %s: server returned %s", id, resp.Status))
			_ = resp.Body.Close()
			continue
		}
		if err := resp.Body.Close(); err != nil {
			failures = append(failures, fmt.Errorf("pause %s: close response body: %w", id, err))
			continue
		}
		paused++
	}

	if len(failures) != 0 {
		return fmt.Errorf("paused %d of %d downloads: %w", paused, len(ids), errors.Join(failures...))
	}

	fmt.Printf("Paused %d downloads.\n", paused)
	return nil
}

func init() {
	rootCmd.AddCommand(pauseCmd)
	pauseCmd.Flags().Bool("all", false, "Pause all downloads")
}
