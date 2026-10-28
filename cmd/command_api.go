package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/SurgeDM/Surge/internal/types"
)

// commandAPIRequest binds observation and mutation requests to command lifetime.
// The caller owns the response. The client enforces the normal API timeout and
// same-origin redirect policy; this helper never retries mutations.
func commandAPIRequest(ctx context.Context, client *http.Client, method, baseURL, token, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return client.Do(req)
}

func getLiveDownloads(ctx context.Context, client *http.Client, baseURL, token string) ([]types.DownloadStatus, error) {
	resp, err := commandAPIRequest(ctx, client, http.MethodGet, baseURL, token, "/list")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, remoteStatusError(resp.StatusCode, fmt.Errorf("server returned HTTP %d", resp.StatusCode))
	}
	var statuses []types.DownloadStatus
	if err := decodeCommandResponse(resp.Body, &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

// Command snapshots are bounded independently of a server's diagnostics. Require
// exactly one JSON document so a truncated or concatenated response cannot pass.
func decodeCommandResponse(reader io.Reader, target any) error {
	const maxSnapshotBytes = 8 << 20
	data, err := io.ReadAll(io.LimitReader(reader, maxSnapshotBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxSnapshotBytes {
		return fmt.Errorf("server snapshot exceeds %d bytes", maxSnapshotBytes)
	}
	return json.Unmarshal(data, target)
}
