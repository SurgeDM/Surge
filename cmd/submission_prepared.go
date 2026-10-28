package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/spf13/cobra"
)

func submitPrepared(cmd *cobra.Command, requests []DownloadRequest, baseURL, token, key string) ([]submissionItem, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	caps, err := readServerCapabilities(ctx, client, baseURL, token)
	if err != nil {
		return nil, err
	}
	if key != "" && !caps.Features["idempotency"] {
		return nil, clioutput.NewError(clioutput.Conflict, "This server does not support durable idempotency.", nil)
	}
	for _, r := range requests {
		if r.inputFailure == nil && r.Artifact.OnExists != "" && !caps.Features["destination_policy"] {
			return nil, clioutput.NewError(clioutput.Conflict, "This server does not support artifact destination policies.", nil)
		}
		if r.Artifact.ExpectedSHA256 != "" && !caps.Features["artifact_sha256"] {
			return nil, clioutput.NewError(clioutput.Conflict, "This server does not support SHA-256 verification.", nil)
		}
	}
	items := make([]submissionItem, 0, len(requests))
	for i, request := range requests {
		item := submissionItem{InputIndex: i + 1, ClientRef: request.ClientRef, Status: "not_submitted"}
		if request.inputFailure != nil {
			item.Error = request.inputFailure
			items = append(items, item)
			continue
		}
		data, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		r, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, baseURL+"/download", bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("Idempotency-Key", itemIdempotencyKey(key, request.ClientRef, i+1))
		}
		response, err := client.Do(r)
		if err != nil {
			item.Status = "unknown"
			err = uncertainSubmission()
		} else {
			receipt, receiptErr := readSubmissionReceipt(response)
			_ = response.Body.Close()
			item.ID, item.RequestID, item.Status = receipt.ID, receipt.RequestID, receipt.Status
			err = receiptErr
		}
		items = append(items, attachSubmissionError(item, err))
	}
	return items, nil
}
