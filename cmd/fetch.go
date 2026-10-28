package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/spf13/cobra"
)

type fetchOptions struct {
	resume      string
	timeout     time.Duration
	onInterrupt string
}
type fetchItem struct {
	waitItem
	InputIndex            int      `json:"input_index"`
	ClientRef             string   `json:"client_ref,omitempty"`
	ReusedBytes           string   `json:"reused_bytes"`
	DurationSeconds       float64  `json:"duration_seconds"`
	AverageBytesPerSecond *float64 `json:"average_bytes_per_second"`
}
type fetchResult struct {
	InterruptActions []lifecycleItem `json:"interrupt_actions,omitempty"`
	clioutput.Result[fetchItem]
	SessionID           string `json:"session_id,omitempty"`
	CheckpointConfirmed *bool  `json:"checkpoint_confirmed,omitempty"`
}

func newFetchCommand() *cobra.Command {
	command := &cobra.Command{Use: "fetch URL...", Short: "Download artifacts now, without a daemon", Annotations: map[string]string{"surge.contract": "1"},
		Long: "Download with an embedded engine and no listening server. Ctrl+C pauses invocation-owned work; --resume SESSION restores its saved checkpoint. Explicit --host submits daemon-owned work and detaches on interruption by default.",
		Args: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputVersion(cmd); err != nil {
				return err
			}
			output, _ := cmd.Flags().GetString("output")
			filename, _ := cmd.Flags().GetString("filename")
			digest, _ := cmd.Flags().GetString("sha256")
			onExists, _ := cmd.Flags().GetString("on-exists")
			opts := fetchOptions{}
			opts.resume, _ = cmd.Flags().GetString("resume")
			opts.timeout, _ = cmd.Flags().GetDuration("timeout")
			opts.onInterrupt, _ = cmd.Flags().GetString("on-interrupt")
			if opts.timeout < 0 {
				return usageError("--timeout must be nonnegative.")
			}
			if opts.resume != "" {
				if len(args) > 0 || resolveHostTarget() != "" || filename != "" || digest != "" || cmd.Flags().Changed("output") || cmd.Flags().Changed("on-exists") {
					return usageError("--resume accepts only a local session ID and observation/output options.")
				}
			} else if len(args) == 0 || len(args) > maxWaitIDs {
				return usageError("Provide between 1 and 256 URLs, or --resume SESSION.")
			}
			if (filename != "" || digest != "") && len(args) > 1 {
				return usageError("--filename and --sha256 require one URL; use a manifest for per-item options.")
			}
			if opts.onInterrupt != "" && opts.onInterrupt != "pause" && opts.onInterrupt != "cancel" && opts.onInterrupt != "detach" {
				return usageError("--on-interrupt must be pause, cancel, or detach.")
			}
			if resolveHostTarget() == "" && opts.onInterrupt != "" && opts.onInterrupt != "pause" {
				return usageError("Embedded fetch pauses on interruption; detach/cancel overrides require an explicit remote host.")
			}
			requests := make([]DownloadRequest, 0, len(args))
			for _, input := range args {
				url, mirrors, err := parseAndNormalizeURLArg(input)
				if err != nil || url == "" {
					return usageError("Each input must be an HTTP or HTTPS download URL.")
				}
				request, err := validateDownloadRequest(DownloadRequest{URL: url, Mirrors: mirrors, Path: output, Filename: filename, SkipApproval: true, SkipDuplicateWarning: true, IsExplicitCategory: true, Artifact: types.ArtifactPolicy{OnExists: onExists, ExpectedSHA256: digest}})
				if err != nil {
					return usageError("Invalid filename, destination policy, or expected SHA-256.")
				}
				requests = append(requests, request)
			}
			return runFetch(cmd, requests, opts)
		}}
	command.Flags().StringP("output", "o", "", "Destination directory (default: current directory)")
	command.Flags().String("filename", "", "Explicit filename, for one URL")
	command.Flags().String("sha256", "", "Expected SHA-256, for one URL; verify before publication")
	command.Flags().String("on-exists", "fail", "Destination policy: fail or rename")
	command.Flags().String("resume", "", "Resume a saved local fetch session by full ID")
	command.Flags().Duration("timeout", 0, "Observation limit; embedded work checkpoints, daemon jobs continue")
	command.Flags().String("on-interrupt", "", "Remote-only override: detach (default), pause, or cancel created jobs")
	command.Flags().Bool("json", false, "Write one version 1 artifact result")
	command.Flags().String("output-version", "", "Use a versioned JSON contract (supported: 1)")
	command.Flags().Bool("non-interactive", false, "Never prompt (fetch always runs without prompts)")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clioutput.NewError(clioutput.InvalidArguments, "Invalid fetch flags.", err)
	})
	return command
}

func usageError(message string) error {
	return clioutput.NewError(clioutput.InvalidArguments, message, nil)
}
func validateOutputVersion(cmd *cobra.Command) error {
	version, _ := cmd.Flags().GetString("output-version")
	if cmd.Flags().Changed("output-version") && version != "1" {
		return usageError("Only --output-version 1 is supported.")
	}
	return nil
}

func runFetch(cmd *cobra.Command, requests []DownloadRequest, opts fetchOptions) error {
	ctx := cmd.Context()
	if opts.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}
	signals := captureCommandSignals(ctx)
	defer signals.stop()
	ctx = signals.Context
	if resolveHostTarget() != "" {
		return runRemoteFetch(cmd, ctx, signals.Force, requests, opts)
	}
	owner, err := newFetchOwner(opts.resume, len(requests))
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			owner.close(signals.Force)
		}
	}()
	started := time.Now()
	items := make([]fetchItem, len(owner.session.Jobs))
	ids := make([]string, 0, len(items))
	positions := make(map[string]int, len(items))
	reused := make(map[string]int64, len(items))
	for i, job := range owner.session.Jobs {
		items[i] = fetchItem{waitItem: waitItem{ID: job.ID, Status: "not_submitted", DownloadedBytes: "0"}, InputIndex: job.InputIndex, ClientRef: job.ClientRef, ReusedBytes: "0"}
		positions[job.ID] = i
	}
	if opts.resume != "" {
		err = owner.resume(ctx)
		if err == nil {
			for _, job := range owner.session.Jobs {
				record, readErr := owner.service.GetStatus(job.ID)
				if readErr == nil && record != nil {
					reused[job.ID] = record.Downloaded
				}
				ids = append(ids, job.ID)
			}
		}
	} else {
		for i, request := range requests {
			if ctx.Err() != nil {
				err = ctx.Err()
				break
			}
			path := request.Path
			if path == "" {
				path = "."
			}
			path, pathErr := filepath.Abs(path)
			if pathErr != nil {
				err = pathErr
				break
			}
			_, _, enqueueErr := owner.manager.EnqueueWithID(ctx, downloadRequestForEngine(request, path), items[i].ID)
			if enqueueErr != nil {
				failure := fetchFailure(enqueueErr)
				items[i].Error = &failure
				items[i].Status = "failed"
				continue
			}
			ids = append(ids, items[i].ID)
			items[i].Status = "queued"
		}
	}
	if err == nil && len(ids) > 0 {
		observed, observeErr := observeDownloads(ctx, ids, func(ctx context.Context, id string) (*types.DownloadStatus, error) {
			return owner.service.GetResult(id)
		}, 250*time.Millisecond, "local", "")
		for _, item := range observed {
			items[positions[item.ID]].waitItem = item
		}
		err = observeErr
	}
	confirmed := owner.close(signals.Force)
	closed = true
	if ctx.Err() != nil {
		err = context.Cause(ctx)
	}
	if err != nil {
		for i := range items {
			if items[i].Artifact == nil && items[i].Error == nil {
				failure := fetchFailure(err)
				items[i].Error = &failure
			}
		}
	}
	finishFetchMetrics(items, reused, time.Since(started))
	if err == nil {
		err = fetchBatchFailure(items)
	}
	if !confirmed && err == nil {
		err = clioutput.NewError(clioutput.OperationFailed, "Shutdown exceeded its grace period; the last checkpoint was not confirmed.", nil)
	}
	return writeFetchResult(cmd, items, owner.session.ID, &confirmed, nil, err)
}

func downloadRequestForEngine(request DownloadRequest, path string) *orchestrator.DownloadRequest {
	return &orchestrator.DownloadRequest{URL: request.URL, Path: path, Filename: request.Filename, Mirrors: request.Mirrors, Headers: request.Headers, IsExplicitCategory: true, SkipApproval: true, Workers: request.Workers, MinChunkSize: request.MinChunkSize, Artifact: request.Artifact}
}
func fetchFailure(err error) clioutput.Failure {
	if errors.Is(err, types.ErrDestinationExists) || errors.Is(err, types.ErrIntegrityMismatch) {
		return clioutput.Describe(clioutput.NewError(clioutput.Conflict, err.Error(), nil))
	}
	return clioutput.Describe(err)
}
func finishFetchMetrics(items []fetchItem, reused map[string]int64, elapsed time.Duration) {
	for i := range items {
		items[i].DurationSeconds = elapsed.Seconds()
		items[i].ReusedBytes = strconv.FormatInt(reused[items[i].ID], 10)
		if items[i].Artifact != nil && elapsed > 0 {
			size, _ := strconv.ParseInt(items[i].Artifact.SizeBytes, 10, 64)
			rate := float64(max(0, size-reused[items[i].ID])) / elapsed.Seconds()
			items[i].AverageBytesPerSecond = &rate
		}
	}
}
func fetchBatchFailure(items []fetchItem) error {
	waitItems := make([]waitItem, 0, len(items))
	for _, item := range items {
		waitItems = append(waitItems, item.waitItem)
	}
	return waitFailure(waitItems)
}
func writeFetchResult(cmd *cobra.Command, items []fetchItem, session string, checkpoint *bool, actions []lifecycleItem, err error) error {
	result := fetchResult{Result: clioutput.Success("fetch", items), SessionID: session, CheckpointConfirmed: checkpoint, InterruptActions: actions}
	if err != nil {
		result.Result = clioutput.FailedItems("fetch", items, err)
	}
	jsonOutput, _ := cmd.Flags().GetBool("json")
	version, _ := cmd.Flags().GetString("output-version")
	if jsonOutput || version == "1" {
		if err != nil {
			return &commandResultError{result: result, cause: err}
		}
		return clioutput.WriteJSON(cmd.OutOrStdout(), result)
	}
	for _, item := range items {
		text := fmt.Sprintf("%q: %s", item.ID, item.Status)
		if item.Artifact != nil {
			text += fmt.Sprintf(" -> %s %q (%s bytes, %s)", item.Artifact.Location, item.Artifact.Path, item.Artifact.SizeBytes, item.Artifact.Verification)
		}
		if item.Error != nil {
			text += ": " + item.Error.Message
		}
		if writeErr := clioutput.WriteText(cmd.OutOrStdout(), text+"\n"); writeErr != nil {
			return writeErr
		}
	}
	if session != "" {
		if writeErr := clioutput.WriteText(cmd.OutOrStdout(), fmt.Sprintf("Session %s; resume with: surge fetch --resume %s\n", session, session)); writeErr != nil {
			return writeErr
		}
	}
	return err
}
func init() { rootCmd.AddCommand(newFetchCommand()) }
