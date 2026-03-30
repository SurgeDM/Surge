package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/surge-downloader/surge/internal/utils"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh <ID> <NEW_URL>",
	Short: "Update the URL of a paused or errored download",
	Long:  `Update the source URL of a download by its ID. It must be paused or in an error state to be refreshed.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initializeGlobalState(); err != nil {
			return err
		}

		id := args[0]
		newURL := args[1]

		baseURL, token, err := resolveAPIConnection(true)
		if err != nil {
			return err
		}

		// Resolve partial ID to full ID
		id, err = resolveDownloadID(id)
		if err != nil {
			return err
		}

		reqBody := map[string]string{
			"url": newURL,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("error creating request: %w", err)
		}

		// Send to running server
		path := fmt.Sprintf("/update-url?id=%s", url.QueryEscape(id))
		resp, err := doAPIRequest(http.MethodPut, baseURL, token, path, bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("error connecting to server: %w", err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				utils.Debug("Error closing response body: %v", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("server returned %s", resp.Status)
		}
		fmt.Printf("Successfully updated URL for download %s\n", id[:8])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(refreshCmd)
}
