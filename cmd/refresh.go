package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/spf13/cobra"
)

const maxRefreshErrorResponseBytes = 1024

var refreshCmd = &cobra.Command{
	Use:   "refresh <ID> <NEW_URL>",
	Short: "Update the URL of a paused or errored download",
	Long:  `Update the source URL of a download by its ID. It must be paused or in an error state to be refreshed.`,
	Args:  cobra.ExactArgs(2),
	RunE:  runRefreshCommand,
}

func runRefreshCommand(_ *cobra.Command, args []string) error {
	if err := initializeGlobalState(); err != nil {
		return err
	}

	id := args[0]
	newURL, err := ValidateAndNormalizeURL(args[1])
	if err != nil {
		return fmt.Errorf("invalid replacement URL: %w", err)
	}

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
		return refreshServerError(resp)
	}
	fmt.Printf("Successfully updated URL for download %s\n", truncateID(id))
	return nil
}

func refreshServerError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRefreshErrorResponseBytes+1))
	if err != nil {
		return fmt.Errorf("server error: %s (failed to read response body: %w)", resp.Status, err)
	}
	truncated := len(body) > maxRefreshErrorResponseBytes
	body = body[:min(len(body), maxRefreshErrorResponseBytes)]
	message := strings.TrimSpace(string(body))
	if truncated {
		message += "..."
	}
	if message == "" {
		return fmt.Errorf("server error: %s", resp.Status)
	}
	return fmt.Errorf("server error: %s - %s", resp.Status, message)
}

func init() {
	rootCmd.AddCommand(refreshCmd)
}
