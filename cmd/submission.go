package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/google/uuid"
)

// submissionItem correlates a receipt with its one-based position in the input.
// Approval request IDs are deliberately separate from queued download IDs.
type submissionItem struct {
	ClientRef  string             `json:"client_ref,omitempty"`
	InputIndex int                `json:"input_index"`
	Status     string             `json:"status"`
	ID         string             `json:"id,omitempty"`
	RequestID  string             `json:"request_id,omitempty"`
	Error      *clioutput.Failure `json:"error,omitempty"`
}

type submissionReceipt struct {
	ID string `json:"id"`
}

// requestSubmission does not retry POST requests. A lost response can hide a
// successful enqueue; callers must preserve that uncertainty in their receipt.
func requestSubmission(baseURL, token, endpoint string, request any) (submissionItem, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return submissionItem{Status: "not_submitted"}, err
	}
	resp, err := doAPIRequest(http.MethodPost, baseURL, token, endpoint, bytes.NewReader(data))
	if err != nil {
		return submissionItem{Status: "unknown"}, uncertainSubmission()
	}
	defer func() { _ = resp.Body.Close() }()

	return readSubmissionReceipt(resp)
}

func readSubmissionReceipt(resp *http.Response) (submissionItem, error) {
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		item := submissionItem{Status: "failed"}
		if id := resp.Header.Get("X-Surge-Download-ID"); id != "" {
			if _, err := uuid.Parse(id); err == nil {
				item.ID = id
				item.Status = "unknown"
			}
		}
		cause := fmt.Errorf("server rejected submission: HTTP %d", resp.StatusCode)
		if resp.StatusCode >= 500 {
			item.Status = "unknown"
			return item, uncertainSubmission()
		}
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity {
			return item, clioutput.NewError(clioutput.InvalidArguments, "The server rejected the download input.", cause)
		}
		return item, remoteStatusError(resp.StatusCode, cause)
	}

	// Bound a receipt independently of arbitrary server error bodies. Decode
	// exactly one document and require an ID before claiming acceptance.
	const maxReceiptBytes = 64 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReceiptBytes+1))
	var receipt submissionReceipt
	if err != nil || len(data) > maxReceiptBytes || json.Unmarshal(data, &receipt) != nil || strings.TrimSpace(receipt.ID) == "" {
		return submissionItem{Status: "unknown"}, uncertainSubmission()
	}
	if resp.StatusCode == http.StatusAccepted {
		return submissionItem{Status: "pending_approval", RequestID: receipt.ID}, nil
	}
	return submissionItem{Status: "queued", ID: receipt.ID}, nil
}

func uncertainSubmission() error {
	return clioutput.NewError(clioutput.OperationFailed,
		"No reliable receipt was received; the server may have accepted the request. Inspect the server before resubmitting.", nil)
}

func attachSubmissionError(item submissionItem, err error) submissionItem {
	if err != nil {
		failure := clioutput.Describe(err)
		item.Error = &failure
	}
	return item
}

func collectSubmissions(urls []string, output, baseURL, token string, confirm bool) []submissionItem {
	items := make([]submissionItem, 0, len(urls))
	for index, input := range urls {
		url, mirrors, err := parseAndNormalizeURLArg(input)
		item := submissionItem{Status: "not_submitted"}
		if err != nil || url == "" {
			err = clioutput.NewError(clioutput.InvalidArguments, "Input must be a valid HTTP or HTTPS download URL.", nil)
		} else {
			item, err = requestSubmission(baseURL, token, "/download", DownloadRequest{
				URL: url, Mirrors: mirrors, Path: output, SkipApproval: !confirm,
			})
		}
		item.InputIndex = index + 1
		items = append(items, attachSubmissionError(item, err))
	}
	return items
}

// Confirmed batches retain the existing single TUI prompt. A batch request ID
// identifies that approval request, not individual queued jobs.
func collectConfirmedBatch(urls []string, output, baseURL, token string) []submissionItem {
	request := BatchDownloadRequest{Path: output, SkipApproval: false}
	items := make([]submissionItem, len(urls))
	invalid := false
	for index, input := range urls {
		items[index] = submissionItem{InputIndex: index + 1, Status: "not_submitted"}
		url, mirrors, err := parseAndNormalizeURLArg(input)
		if err != nil || url == "" {
			invalid = true
			items[index] = attachSubmissionError(items[index], clioutput.NewError(clioutput.InvalidArguments, "Input must be a valid HTTP or HTTPS download URL.", nil))
		}
		request.Downloads = append(request.Downloads, DownloadRequest{URL: url, Mirrors: mirrors, Path: output})
	}
	if invalid {
		for index := range items {
			if items[index].Error == nil {
				items[index] = attachSubmissionError(items[index], clioutput.NewError(clioutput.InvalidArguments, "Batch not submitted because another input is invalid.", nil))
			}
		}
		return items
	}
	item, err := requestSubmission(baseURL, token, "/download/batch", request)
	// Older batch endpoints do not return per-job IDs. Never assign a batch ID
	// to every job when a server unexpectedly reports an immediate enqueue.
	if err == nil && item.Status != "pending_approval" {
		item, err = submissionItem{Status: "unknown"}, uncertainSubmission()
	}
	for index := range items {
		items[index] = attachSubmissionError(item, err)
		items[index].InputIndex = index + 1
	}
	return items
}

func submissionFailure(items []submissionItem) error {
	accepted, failed := 0, 0
	code := clioutput.Code("")
	for _, item := range items {
		if item.Error == nil {
			accepted++
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
	if accepted > 0 {
		return clioutput.NewError(clioutput.PartialFailure, "Some download requests were not accepted; inspect the per-input receipts.", nil)
	}
	return clioutput.NewError(code, "No download requests have confirmed acceptance; inspect the per-input receipts.", nil)
}
