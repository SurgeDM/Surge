package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func runRemoteFetch(cmd *cobra.Command, ctx context.Context, force <-chan struct{}, requests []DownloadRequest, opts fetchOptions) error {
	client, baseURL, token, err := lifecycleConnection()
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	caps, err := readServerCapabilities(ctx, client, baseURL, token)
	if err != nil {
		return err
	}
	if !caps.Features["finalized_results"] || !caps.Features["destination_policy"] {
		return clioutput.NewError(clioutput.Conflict, "Upgrade this server to support foreground artifact requests.", nil)
	}
	if opts.idempotencyKey != "" && !caps.Features["idempotency"] {
		return clioutput.NewError(clioutput.Conflict, "This server does not support durable idempotency.", nil)
	}
	for _, request := range requests {
		if request.Artifact.ExpectedSHA256 != "" && !caps.Features["artifact_sha256"] {
			return clioutput.NewError(clioutput.Conflict, "The server does not support SHA-256 verification.", nil)
		}
	}
	progress := startFetchProgress(cmd)
	defer progress.stop()
	started := time.Now()
	items := make([]fetchItem, 0, len(requests))
	ids := make([]string, 0, len(requests))
	positions := make(map[string]int, len(requests))
	for i, request := range requests {
		item := fetchItem{waitItem: waitItem{Status: "not_submitted", DownloadedBytes: "0"}, InputIndex: i + 1, ClientRef: request.ClientRef}
		if request.inputFailure != nil {
			item.Error = request.inputFailure
			items = append(items, item)
			continue
		}
		if ctx.Err() != nil {
			failure := fetchFailure(context.Cause(ctx))
			item.Error = &failure
			items = append(items, item)
			continue
		}
		data, marshalErr := json.Marshal(request)
		if marshalErr != nil {
			return marshalErr
		}
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/download?id="+uuid.NewString(), bytes.NewReader(data))
		if requestErr != nil {
			return requestErr
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if opts.idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", itemIdempotencyKey(opts.idempotencyKey, request.ClientRef, i+1))
		}
		resp, submitErr := client.Do(req)
		if submitErr != nil {
			item.Status = "unknown"
			failure := clioutput.Describe(uncertainSubmission())
			item.Error = &failure
		} else {
			receipt, receiptErr := readSubmissionReceipt(resp)
			_ = resp.Body.Close()
			item.ID, item.Status = receipt.ID, receipt.Status
			if receiptErr != nil {
				failure := clioutput.Describe(receiptErr)
				item.Error = &failure
			} else if receipt.Status != "queued" {
				failure := clioutput.Failure{Code: clioutput.Conflict, Message: "Foreground work requires immediate acceptance; approval was requested."}
				item.Error = &failure
			} else {
				positions[item.ID] = len(items)
				ids = append(ids, item.ID)
			}
		}
		items = append(items, item)
	}
	if len(ids) > 0 && ctx.Err() == nil {
		observed, observeErr := observeDownloads(ctx, ids, func(ctx context.Context, id string) (*types.DownloadStatus, error) {
			status, err := getWaitStatus(ctx, client, baseURL, token, id)
			progress.update(status)
			return status, err
		}, time.Second, "remote", baseURL)
		for _, item := range observed {
			items[positions[item.ID]].waitItem = item
		}
		err = observeErr
	}
	var actions []lifecycleItem
	if ctx.Err() != nil {
		err = context.Cause(ctx)
		code := clioutput.Describe(err).Code
		if (code == clioutput.Interrupted || code == clioutput.Terminated) && (opts.onInterrupt == "pause" || opts.onInterrupt == "cancel") {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			for _, id := range ids {
				select {
				case <-force:
					cancel()
				default:
				}
				actions = append(actions, requestLifecycleAction(cleanup, client, baseURL, token, opts.onInterrupt, id))
			}
			cancel()
		}
	}
	finishFetchMetrics(items, nil, time.Since(started))
	if err == nil {
		err = fetchBatchFailure(items)
	}
	progress.stop()
	return writeFetchResult(cmd, items, "", nil, actions, err)
}
