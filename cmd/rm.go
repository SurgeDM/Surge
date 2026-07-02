package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:     "rm <ID>",
	Aliases: []string{"kill"},
	Short:   "Remove a download",
	Long:    `Remove a download by its ID. Use --clean to remove all completed downloads. Use --clean-failed to remove all failed downloads. Use --purge to also delete the file(s) from disk.`,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initializeGlobalState(); err != nil {
			return err
		}

		clean, _ := cmd.Flags().GetBool("clean")
		cleanFailed, _ := cmd.Flags().GetBool("clean-failed")
		purge, _ := cmd.Flags().GetBool("purge")

		if clean && cleanFailed {
			return fmt.Errorf("--clean and --clean-failed are mutually exclusive")
		}

		if clean && purge {
			return fmt.Errorf("--clean and --purge are mutually exclusive; use --purge with an ID to also delete that download's files")
		}
		if cleanFailed && purge {
			return fmt.Errorf("--clean-failed and --purge are mutually exclusive; use --purge with an ID to also delete that download's files")
		}

		if !clean && !cleanFailed && len(args) == 0 {
			return fmt.Errorf("provide a download ID, or use --clean or --clean-failed")
		}

		if clean {
			baseURL, token, err := resolveAPIConnection(true)
			if err != nil {
				return err
			}
			resp, err := doAPIRequest(http.MethodPost, baseURL, token, "/clear-completed", nil)
			if err != nil {
				return fmt.Errorf("failed to send request to server: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error: %s", resp.Status)
			}
			var res map[string]int64
			_ = json.NewDecoder(resp.Body).Decode(&res)
			fmt.Printf("Removed %d completed downloads.\n", res["deleted"])
			return nil
		} else if cleanFailed {
			baseURL, token, err := resolveAPIConnection(true)
			if err != nil {
				return err
			}
			resp, err := doAPIRequest(http.MethodPost, baseURL, token, "/clear-failed", nil)
			if err != nil {
				return fmt.Errorf("failed to send request to server: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error: %s", resp.Status)
			}
			var res map[string]int64
			_ = json.NewDecoder(resp.Body).Decode(&res)
			fmt.Printf("Removed %d failed downloads.\n", res["deleted"])
			return nil
		}

		if purge {
			return ExecuteAPIAction(args[0], "/purge", http.MethodPost, "Purged download and deleted files")
		}
		return ExecuteAPIAction(args[0], "/delete", http.MethodPost, "Removed download")
	},
}

func init() {
	rootCmd.AddCommand(rmCmd)
	rmCmd.Flags().Bool("clean", false, "Remove all completed downloads")
	rmCmd.Flags().Bool("clean-failed", false, "Remove all failed downloads")
	rmCmd.Flags().BoolP("purge", "p", false, "Also delete the downloaded file(s) from disk")
}
