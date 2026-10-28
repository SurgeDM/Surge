package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/spf13/cobra"
)

const maxWaitIDs = 256

type waitArtifact struct {
	Location     string `json:"location"`
	Server       string `json:"server,omitempty"`
	Path         string `json:"path"`
	SizeBytes    string `json:"size_bytes"`
	Verification string `json:"verification"`
	SHA256       string `json:"sha256,omitempty"`
}

type waitItem struct {
	ID              string             `json:"id"`
	Status          string             `json:"status"`
	DownloadedBytes string             `json:"downloaded_bytes"`
	TotalBytes      *string            `json:"total_bytes"`
	Artifact        *waitArtifact      `json:"artifact,omitempty"`
	Error           *clioutput.Failure `json:"error,omitempty"`
}

func newWaitCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "wait ID...", Short: "Wait for selected downloads to finish finalization",
		Long: "Wait for a fixed set of daemon jobs. Timeout and Ctrl+C detach observation and leave jobs running. Completed receipts identify artifacts on the daemon's filesystem; they include a verified SHA-256 only when the job requested verification.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 || len(args) > maxWaitIDs {
				return clioutput.NewError(clioutput.InvalidArguments, "Provide between 1 and 256 download IDs.", nil)
			}
			for _, id := range args {
				if strings.TrimSpace(id) == "" {
					return clioutput.NewError(clioutput.InvalidArguments, "Download IDs cannot be empty.", nil)
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			version, _ := cmd.Flags().GetString("output-version")
			if cmd.Flags().Changed("output-version") && version != "1" {
				return clioutput.NewError(clioutput.InvalidArguments, "Only --output-version 1 is supported.", nil)
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			if timeout < 0 {
				return clioutput.NewError(clioutput.InvalidArguments, "--timeout must be nonnegative; 0 waits indefinitely.", nil)
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			if err := initializeClientState(); err != nil {
				return err
			}
			client, baseURL, token, err := lifecycleConnection()
			if err != nil {
				return err
			}
			defer client.CloseIdleConnections()
			ids, err := resolveWaitIDs(ctx, client, baseURL, token, args)
			if err != nil {
				return err
			}
			lookup := func(ctx context.Context, id string) (*types.DownloadStatus, error) {
				return getWaitStatus(ctx, client, baseURL, token, id)
			}
			location, server := "local", ""
			if resolveHostTarget() != "" {
				location, server = "remote", baseURL
			}
			items, err := observeDownloads(ctx, ids, lookup, time.Second, location, server)
			return writeWaitResult(cmd, items, err)
		},
	}
	command.Flags().Duration("timeout", 0, "Limit observation time; jobs continue running (0: no limit)")
	command.Flags().Bool("json", false, "Output one version 1 JSON result")
	command.Flags().String("output-version", "", "Use a versioned JSON contract (supported: 1)")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clioutput.NewError(clioutput.InvalidArguments, "Invalid command flags.", err)
	})
	return command
}

func resolveWaitIDs(ctx context.Context, client *http.Client, baseURL, token string, args []string) ([]string, error) {
	var candidates []string
	needsSnapshot := false
	for _, id := range args {
		if len(id) < 32 {
			needsSnapshot = true
		}
	}
	if needsSnapshot {
		statuses, err := getLiveDownloads(ctx, client, baseURL, token)
		if err != nil {
			return nil, err
		}
		for _, status := range statuses {
			candidates = append(candidates, status.ID)
		}
	}
	ids := make([]string, 0, len(args))
	seen := make(map[string]bool, len(args))
	for _, id := range args {
		if len(id) < 32 {
			var err error
			id, err = resolveIDFromCandidates(id, candidates)
			if err != nil {
				return nil, err
			}
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, nil
}

func getWaitStatus(ctx context.Context, client *http.Client, baseURL, token, id string) (*types.DownloadStatus, error) {
	resp, err := commandAPIRequest(ctx, client, http.MethodGet, baseURL, token, "/download/result?id="+url.QueryEscape(id))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Header.Get("X-Surge-Result-Version") != "1" {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout, http.StatusTooManyRequests:
			return nil, remoteStatusError(resp.StatusCode, fmt.Errorf("server returned HTTP %d", resp.StatusCode))
		}
		if resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented {
			return nil, remoteStatusError(resp.StatusCode, fmt.Errorf("server returned HTTP %d", resp.StatusCode))
		}
		return nil, clioutput.NewError(clioutput.Conflict, "This server does not support finalized results; upgrade it before using wait.", nil)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, remoteStatusError(resp.StatusCode, fmt.Errorf("server returned HTTP %d", resp.StatusCode))
	}
	var status types.DownloadStatus
	if err := decodeCommandResponse(resp.Body, &status); err != nil {
		return nil, err
	}
	if status.ID != id || status.Status == "" || status.Downloaded < 0 {
		return nil, clioutput.NewError(clioutput.OperationFailed, "The server returned an invalid job result.", nil)
	}
	if status.Status == "completed" && status.DestPath == "" {
		return nil, clioutput.NewError(clioutput.OperationFailed, "The server did not provide the completed artifact path.", nil)
	}
	return &status, nil
}

func observeDownloads(ctx context.Context, ids []string, lookup func(context.Context, string) (*types.DownloadStatus, error), interval time.Duration, location, server string) ([]waitItem, error) {
	items := make([]waitItem, len(ids))
	done := make([]bool, len(ids))
	failures := make([]int, len(ids))
	for i, id := range ids {
		items[i] = waitItem{ID: id, Status: "unknown", DownloadedBytes: "0"}
	}
	remaining := len(ids)
	for remaining > 0 && ctx.Err() == nil {
		backoff := 0
		for i, id := range ids {
			if done[i] {
				continue
			}
			if ctx.Err() != nil {
				break
			}
			status, err := lookup(ctx, id)
			if ctx.Err() != nil {
				break
			}
			if err != nil {
				failure := clioutput.Describe(err)
				failures[i]++
				// Retry reads only, with a finite consecutive-failure budget. Lost
				// terminal events are irrelevant: every pass queries committed state.
				if (failure.Retryable || failure.Code == clioutput.Timeout) && failures[i] < 6 {
					if failures[i] > backoff {
						backoff = failures[i]
					}
					continue
				}
				items[i].Error = &failure
				done[i], remaining = true, remaining-1
				continue
			}
			failures[i] = 0
			items[i].Status, items[i].DownloadedBytes = status.Status, strconv.FormatInt(status.Downloaded, 10)
			if status.TotalSize > 0 {
				total := strconv.FormatInt(status.TotalSize, 10)
				items[i].TotalBytes = &total
			} else {
				items[i].TotalBytes = nil
			}
			switch status.Status {
			case "completed":
				items[i].Artifact = completedWaitArtifact(status, location, server)
			case "paused", "cancelled":
				failure := clioutput.Describe(clioutput.NewError(clioutput.Conflict, "The job is stopped; explicitly resume it before waiting for completion.", nil))
				items[i].Error = &failure
			case "error":
				code, message := clioutput.OperationFailed, "The download failed; inspect its state before retrying."
				if status.ErrorCode == "integrity_mismatch" {
					code, message = clioutput.Conflict, "Artifact verification failed; working data was retained for inspection."
				}
				if status.ErrorCode == "destination_exists" {
					code, message = clioutput.Conflict, "Destination already exists; working data was retained."
				}
				failure := clioutput.Describe(clioutput.NewError(code, message, nil))
				items[i].Error = &failure
			default:
				continue
			}
			done[i], remaining = true, remaining-1
		}
		if remaining > 0 && ctx.Err() == nil {
			if backoff > 3 {
				backoff = 3
			}
			timer := time.NewTimer(interval * time.Duration(1<<backoff))
			select {
			case <-ctx.Done():
			case <-timer.C:
			}
			timer.Stop()
		}
	}
	if remaining == 0 {
		return items, waitFailure(items)
	}
	if ctx.Err() != nil {
		for i := range items {
			if !done[i] {
				failure := clioutput.Describe(ctx.Err())
				items[i].Error = &failure
			}
		}
		code, message := clioutput.Timeout, "Observation timed out; daemon jobs were left running."
		if ctx.Err() == context.Canceled {
			code, message = clioutput.Interrupted, "Observation interrupted; daemon jobs were left running."
		}
		return items, clioutput.NewError(code, message, nil)
	}
	return items, waitFailure(items)
}

func waitFailure(items []waitItem) error {
	failed := 0
	code := clioutput.Code("")
	for _, item := range items {
		if item.Error == nil {
			continue
		}
		failed++
		if code == "" {
			code = item.Error.Code
		} else if code != item.Error.Code {
			code = clioutput.OperationFailed
		}
	}
	if failed == 0 {
		return nil
	}
	if failed < len(items) {
		code = clioutput.PartialFailure
	}
	return clioutput.NewError(code, "Some selected downloads did not complete; inspect the per-job results.", nil)
}

func writeWaitResult(cmd *cobra.Command, items []waitItem, err error) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	version, _ := cmd.Flags().GetString("output-version")
	if jsonOutput || version == "1" {
		if err != nil {
			return &commandResultError{result: clioutput.FailedItems("wait", items, err), cause: err}
		}
		return clioutput.WriteJSON(cmd.OutOrStdout(), clioutput.Success("wait", items))
	}
	for _, item := range items {
		text := fmt.Sprintf("%q: %s", item.ID, item.Status)
		if item.Artifact != nil {
			verification := "checksum not requested"
			if item.Artifact.Verification == "verified" {
				verification = "sha256 verified"
			}
			text += fmt.Sprintf(" -> %s %q (%s bytes, %s)", item.Artifact.Location, item.Artifact.Path, item.Artifact.SizeBytes, verification)
		}
		if item.Error != nil {
			text += ": " + item.Error.Message
		}
		if writeErr := clioutput.WriteText(cmd.OutOrStdout(), text+"\n"); writeErr != nil {
			return writeErr
		}
	}
	return err
}

func init() { rootCmd.AddCommand(newWaitCommand()) }

func completedWaitArtifact(status *types.DownloadStatus, location, server string) *waitArtifact {
	verification := "not_requested"
	if status.Artifact.Verification == "verified" {
		verification = "verified"
	}
	return &waitArtifact{Location: location, Server: server, Path: status.DestPath, SizeBytes: strconv.FormatInt(status.Downloaded, 10), Verification: verification, SHA256: status.Artifact.SHA256}
}
