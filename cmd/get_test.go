package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/download"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
)

func startAuthedTestServer(t *testing.T, service core.DownloadService, token string) string {
	t.Helper()

	mux := http.NewServeMux()
	registerHTTPRoutes(mux, 0, "", service)
	handler := corsMiddleware(authMiddleware(token, mux))

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}

func TestCLI_DeleteEndpoint_CleansPausedStateAndPartialFile(t *testing.T) {
	tempDir := setupXDGEnvIsolation(t)

	state.CloseDB()
	if err := initializeGlobalState(); err != nil {
		t.Fatalf("initializeGlobalState failed: %v", err)
	}

	GlobalProgressCh = make(chan any, 100)
	GlobalPool = download.NewWorkerPool(GlobalProgressCh, 2)

	// Start server
	svc := core.NewLocalDownloadService(GlobalPool)
	t.Cleanup(func() { _ = svc.Shutdown() })

	lifecycle := processing.NewLifecycleManager(nil, nil)
	stream, streamCleanup, err := svc.StreamEvents(context.Background())
	if err != nil {
		t.Fatalf("failed to open event stream: %v", err)
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		lifecycle.StartEventWorker(context.Background(), stream)
	}()
	t.Cleanup(func() {
		streamCleanup()
		<-workerDone
	})

	const authToken = "test-token-delete-endpoint"
	baseURL := startAuthedTestServer(t, svc, authToken)
	client := &http.Client{Timeout: 3 * time.Second}

	doRequest := func(method, url string) (*http.Response, error) {
		r, reqErr := http.NewRequestWithContext(context.Background(), method, url, http.NoBody)
		if reqErr != nil {
			return nil, reqErr
		}
		r.Header.Set("Authorization", "Bearer "+authToken)
		r.Header.Set("Content-Type", "application/json")
		return client.Do(r)
	}

	id := "paused-delete-test-id"
	url := "https://example.com/file.bin"
	downloadDir := filepath.Join(tempDir, "downloads")
	if mkErr := os.MkdirAll(downloadDir, 0o750); mkErr != nil {
		t.Fatalf("failed to create download dir: %v", mkErr)
	}
	destPath := filepath.Join(downloadDir, "file.bin")
	incompletePath := destPath + types.IncompleteSuffix
	if wrErr := os.WriteFile(incompletePath, []byte("partial-data"), 0o600); wrErr != nil {
		t.Fatalf("failed to create partial file: %v", wrErr)
	}

	if saveErr := state.SaveState(context.Background(), url, destPath, &types.DownloadState{
		ID:         id,
		URL:        url,
		DestPath:   destPath,
		Filename:   "file.bin",
		TotalSize:  1000,
		Downloaded: 250,
		Tasks: []types.Task{
			{Offset: 250, Length: 750},
		},
	}); saveErr != nil {
		t.Fatalf("failed to seed paused state: %v", saveErr)
	}

	resp, err := doRequest(http.MethodDelete, baseURL+"/delete?id="+id)
	if err != nil {
		t.Fatalf("Failed to request delete: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if result["status"] != "deleted" {
		t.Fatalf("Expected status 'deleted', got %v", result["status"])
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, statErr := os.Stat(incompletePath)
		entry, dbErr := state.GetDownload(context.Background(), id)
		if dbErr != nil {
			t.Fatalf("failed to query entry after delete: %v", dbErr)
		}
		if os.IsNotExist(statErr) && entry == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, sErr := os.Stat(incompletePath); !os.IsNotExist(sErr) {
		t.Fatalf("expected partial file to be deleted, stat err: %v", sErr)
	}
	entry, getErr := state.GetDownload(context.Background(), id)
	if getErr != nil {
		t.Fatalf("failed to query entry after delete: %v", getErr)
	}
	if entry != nil {
		t.Fatalf("expected download entry removed from DB, found: %+v", entry)
	}

	listResp, listErr := doRequest(http.MethodGet, baseURL+"/list")
	if listErr != nil {
		t.Fatalf("failed to request list: %v", listErr)
	}
	defer func() { _ = listResp.Body.Close() }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /list, got %d", listResp.StatusCode)
	}

	var statuses []types.DownloadStatus
	if err := json.NewDecoder(listResp.Body).Decode(&statuses); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}
	for _, st := range statuses {
		if st.ID == id {
			t.Fatalf("expected deleted download to be absent from list, found status: %+v", st)
		}
	}
}
