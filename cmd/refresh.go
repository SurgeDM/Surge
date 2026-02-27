package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
	"github.com/surge-downloader/surge/internal/utils"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh <ID> <NEW_URL>",
	Short: "Update the URL of a paused or errored download",
	Long:  `Update the source URL of a download by its ID. It must be paused or in an error state to be refreshed.`,
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		initializeGlobalState()

		id := args[0]
		newURL := args[1]

		baseURL, token, err := resolveAPIConnection(true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Resolve partial ID to full ID
		id, err = resolveDownloadID(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		reqBody := map[string]string{
			"url": newURL,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
			os.Exit(1)
		}

		// Send to running server
		path := fmt.Sprintf("/update-url?id=%s", url.QueryEscape(id))
		resp, err := doAPIRequest(http.MethodPut, baseURL, token, path, bytes.NewBuffer(jsonData))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to server: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				utils.Debug("Error closing response body: %v", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Error: server returned %s\n", resp.Status)
			os.Exit(1)
		}
		fmt.Printf("Successfully updated URL for download %s\n", id[:8])
	},
}

func init() {
	rootCmd.AddCommand(refreshCmd)
}
