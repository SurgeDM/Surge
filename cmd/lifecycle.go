package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/spf13/cobra"
)

type lifecycleItem struct {
	ID     string             `json:"id"`
	Status string             `json:"status"`
	Error  *clioutput.Failure `json:"error,omitempty"`
}

func newLifecycleCommand(action, short string) *cobra.Command {
	command := &cobra.Command{
		Use: action + " <ID> | --all", Short: short,
		Long: short + ". --all selects eligible jobs from one live server snapshot. A successful receipt confirms the request, not a persisted checkpoint.",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return clioutput.NewError(clioutput.InvalidArguments, "Expected one download ID or --all.", err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			all, _ := cmd.Flags().GetBool("all")
			version, _ := cmd.Flags().GetString("output-version")
			if cmd.Flags().Changed("output-version") && version != "1" {
				return clioutput.NewError(clioutput.InvalidArguments, "Only --output-version 1 is supported.", nil)
			}
			if !all && len(args) == 0 {
				return clioutput.NewError(clioutput.InvalidArguments, "provide a download ID or use --all", nil)
			}
			if all && len(args) != 0 {
				return clioutput.NewError(clioutput.InvalidArguments, "--all cannot be used with a download ID", nil)
			}
			if resolveHostTarget() == "" {
				if err := initializeGlobalState(); err != nil {
					return err
				}
			}
			if all {
				return runLifecycleAll(ctx, cmd, action)
			}
			client, baseURL, token, err := lifecycleConnection()
			if err != nil {
				return err
			}
			defer client.CloseIdleConnections()
			id := args[0]
			if len(id) < 32 {
				statuses, err := getLiveDownloads(ctx, client, baseURL, token)
				if err != nil {
					return err
				}
				ids := make([]string, 0, len(statuses))
				for _, status := range statuses {
					ids = append(ids, status.ID)
				}
				id, err = resolveIDFromCandidates(id, ids)
				if err != nil {
					return err
				}
			}
			return writeLifecycleResult(cmd, action, []lifecycleItem{requestLifecycleAction(ctx, client, baseURL, token, action, id)})
		},
	}
	command.Flags().Bool("all", false, "Apply to eligible downloads from one server snapshot")
	command.Flags().String("output-version", "", "Use a versioned JSON contract (supported: 1)")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clioutput.NewError(clioutput.InvalidArguments, "Invalid command flags.", err)
	})
	return command
}

func lifecycleConnection() (*http.Client, string, string, error) {
	baseURL, token, err := resolveAPIConnection(true)
	if err != nil {
		return nil, "", "", err
	}
	client, err := newRemoteAPIHTTPClient()
	return client, baseURL, token, err
}

func runLifecycleAll(ctx context.Context, cmd *cobra.Command, action string) error {
	client, baseURL, token, err := lifecycleConnection()
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	statuses, err := getLiveDownloads(ctx, client, baseURL, token)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(statuses))
	seen := make(map[string]bool, len(statuses))
	for _, status := range statuses {
		eligible := status.Status == "downloading" || status.Status == "queued"
		if action == "resume" {
			eligible = status.Status == "paused"
		}
		if eligible && !seen[status.ID] {
			seen[status.ID] = true
			ids = append(ids, status.ID)
		}
	}
	sort.Strings(ids)
	items := make([]lifecycleItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, requestLifecycleAction(ctx, client, baseURL, token, action, id))
	}
	return writeLifecycleResult(cmd, action, items)
}

// A lost mutation response is uncertain. Never automatically replay it or claim
// that a checkpoint was saved. Even legacy servers returning an empty 200 body
// acknowledge only the request, which is the weakest compatible guarantee.
func requestLifecycleAction(ctx context.Context, client *http.Client, baseURL, token, action, id string) lifecycleItem {
	item := lifecycleItem{ID: id, Status: "requested"}
	if ctx.Err() != nil {
		failure := clioutput.Describe(ctx.Err())
		item.Status, item.Error = "not_requested", &failure
		return item
	}
	resp, err := commandAPIRequest(ctx, client, http.MethodPost, baseURL, token, "/"+action+"?id="+url.QueryEscape(id))
	if err != nil {
		item.Status = "unknown"
		err = clioutput.NewError(clioutput.OperationFailed, "No acknowledgment was received; inspect the job before retrying.", nil)
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			item.Status = "failed"
			if resp.StatusCode >= 500 {
				item.Status = "unknown"
			}
			err = remoteStatusError(resp.StatusCode, fmt.Errorf("server returned HTTP %d", resp.StatusCode))
		}
	}
	if err != nil {
		failure := clioutput.Describe(err)
		if item.Status == "unknown" {
			failure.Retryable = false
		}
		item.Error = &failure
	}
	return item
}

func writeLifecycleResult(cmd *cobra.Command, action string, items []lifecycleItem) error {
	failures := 0
	code := clioutput.Code("")
	for _, item := range items {
		if item.Error != nil {
			failures++
			if code == "" {
				code = item.Error.Code
			} else if code != item.Error.Code {
				code = clioutput.OperationFailed
			}
		}
	}
	var err error
	if failures > 0 {
		if failures < len(items) {
			code = clioutput.PartialFailure
		}
		err = clioutput.NewError(code, "Some lifecycle requests failed or have an unknown outcome; inspect the per-job receipts.", nil)
	}
	version, _ := cmd.Flags().GetString("output-version")
	if version == "1" {
		if err != nil {
			return &commandResultError{result: clioutput.FailedItems(action, items, err), cause: err}
		}
		return clioutput.WriteJSON(cmd.OutOrStdout(), clioutput.Success(action, items))
	}
	for _, item := range items {
		text := fmt.Sprintf("%s %q: %s\n", action, item.ID, item.Status)
		if writeErr := clioutput.WriteText(cmd.OutOrStdout(), text); writeErr != nil {
			return writeErr
		}
	}
	if len(items) == 0 {
		return clioutput.WriteText(cmd.OutOrStdout(), "No eligible downloads.\n")
	}
	return err
}

var cancelCmd = newLifecycleCommand("cancel", "Stop a download and retain its resumable data and record")

func init() { rootCmd.AddCommand(cancelCmd) }
