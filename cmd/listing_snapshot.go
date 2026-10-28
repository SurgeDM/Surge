package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

type downloadSnapshot struct {
	downloads []types.DownloadStatus
	source    string
	warning   string
}

type listingResult struct {
	clioutput.Result[downloadItem]
	Source string `json:"source"`
	Stale  bool   `json:"stale"`
}

func (snapshot downloadSnapshot) result() listingResult {
	items := downloadItems(snapshot.downloads)
	if snapshot.source == "history" {
		for index := range items {
			items[index].SpeedBytesPerSecond = nil
		}
	}
	return listingResult{Result: clioutput.Success("ls", items), Source: snapshot.source, Stale: snapshot.source == "history"}
}

// Only an unavailable local server permits a saved-history fallback. An
// authentication, protocol, or explicit remote failure must stay visible.
func canUseHistory(err error, strictRemote bool) bool {
	if strictRemote {
		return false
	}
	code := clioutput.Describe(err).Code
	return code == clioutput.Unavailable || code == clioutput.Timeout
}

func readHistorySnapshot() (downloadSnapshot, error) {
	records, err := store.ListAllDownloads()
	if err != nil {
		return downloadSnapshot{}, fmt.Errorf("error listing saved downloads: %w", err)
	}
	snapshot := downloadSnapshot{source: "history", warning: "Using saved history; transfer status may be stale and live rates are unavailable.", downloads: make([]types.DownloadStatus, 0, len(records))}
	for _, record := range records {
		snapshot.downloads = append(snapshot.downloads, statusFromRecord(record))
	}
	return snapshot, nil
}

func readDownloadSnapshot(ctx context.Context, baseURL, token string, strictRemote bool) (downloadSnapshot, error) {
	if baseURL != "" {
		statuses, err := getRemoteDownloadsContext(ctx, baseURL, token)
		if err == nil {
			for index := range statuses {
				statuses[index] = withoutInactiveRates(statuses[index])
			}
			return downloadSnapshot{downloads: statuses, source: "live"}, nil
		}
		if ctx.Err() != nil {
			return downloadSnapshot{}, ctx.Err()
		}
		if !canUseHistory(err, strictRemote) {
			return downloadSnapshot{}, fmt.Errorf("error listing remote downloads: %w", err)
		}
	}
	if strictRemote {
		return downloadSnapshot{}, clioutput.NewError(clioutput.Unavailable, "The requested server is unavailable.", nil)
	}
	return readHistorySnapshot()
}

func readDownloadDetails(ctx context.Context, partialID, baseURL, token string, strictRemote bool) (downloadSnapshot, error) {
	snapshot := downloadSnapshot{source: "live"}
	id := partialID
	// Full IDs can query the detail endpoint directly. Prefixes resolve against
	// one authoritative snapshot, never a mixture of live and saved IDs.
	if baseURL == "" || len(partialID) < 32 {
		var err error
		snapshot, err = readDownloadSnapshot(ctx, baseURL, token, strictRemote)
		if err != nil {
			return downloadSnapshot{}, err
		}
		ids := make([]string, 0, len(snapshot.downloads))
		appendCandidateIDs(&ids, snapshot.downloads)
		id, err = resolveIDFromCandidates(partialID, ids)
		if err != nil {
			return downloadSnapshot{}, err
		}
	}
	if snapshot.source == "live" {
		status, err := readRemoteDownloadDetail(ctx, id, baseURL, token)
		if err == nil {
			snapshot.downloads = []types.DownloadStatus{withoutInactiveRates(status)}
			return snapshot, nil
		}
		if ctx.Err() != nil {
			return downloadSnapshot{}, ctx.Err()
		}
		if !canUseHistory(err, strictRemote) {
			return downloadSnapshot{}, err
		}
		snapshot, err = readHistorySnapshot()
		if err != nil {
			return downloadSnapshot{}, err
		}
	}
	for _, status := range snapshot.downloads {
		if status.ID == id {
			snapshot.downloads = []types.DownloadStatus{status}
			return snapshot, nil
		}
	}
	return downloadSnapshot{}, clioutput.NewError(clioutput.NotFound, "Download not found.", fmt.Errorf("download not found: %s", partialID))
}

func readRemoteDownloadDetail(ctx context.Context, id, baseURL, token string) (types.DownloadStatus, error) {
	resp, err := doAPIRequestContext(ctx, http.MethodGet, baseURL, token, "/download?id="+url.QueryEscape(id), nil)
	if err != nil {
		return types.DownloadStatus{}, fmt.Errorf("error fetching remote download details: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return types.DownloadStatus{}, remoteStatusError(resp.StatusCode, fmt.Errorf("remote server returned %s", resp.Status))
	}
	var status types.DownloadStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return types.DownloadStatus{}, fmt.Errorf("error decoding remote download details: %w", err)
	}
	if status.ID != id {
		return types.DownloadStatus{}, fmt.Errorf("server returned a different download ID")
	}
	return status, nil
}

func withoutInactiveRates(status types.DownloadStatus) types.DownloadStatus {
	if status.Status != "downloading" {
		status.Speed = 0
		status.ETA = 0
		status.Connections = 0
	}
	return status
}
