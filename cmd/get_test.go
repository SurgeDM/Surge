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
		lifecycle.StartEventWorker(stream)
	}()
	t.Cleanup(func() {
		streamCleanup()
		<-workerDone
	})

	const authToken = "test-token-delete-endpoint"
	baseURL := startAuthedTestServer(t, svc, authToken)
	client := &http.Client{Timeout: 3 * time.Second}

	doRequest := func(method, url string) (*http.Response, error) {
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+authToken)
		req.Header.Set("Content-Type", "application/json")
		return client.Do(req)
	}

	id := "paused-delete-test-id"
	url := "https://example.com/file.bin"
	downloadDir := filepath.Join(tempDir, "downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("failed to create download dir: %v", err)
	}
	destPath := filepath.Join(downloadDir, "file.bin")
	incompletePath := destPath + types.IncompleteSuffix
	if err := os.WriteFile(incompletePath, []byte("partial-data"), 0o644); err != nil {
		t.Fatalf("failed to create partial file: %v", err)
	}

	if err := state.SaveState(url, destPath, &types.DownloadState{
		ID:         id,
		URL:        url,
		DestPath:   destPath,
		Filename:   "file.bin",
		TotalSize:  1000,
		Downloaded: 250,
		Tasks: []types.Task{
			{Offset: 250, Length: 750},
		},
	}); err != nil {
		t.Fatalf("failed to seed paused state: %v", err)
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
		entry, dbErr := state.GetDownload(id)
		if dbErr != nil {
			t.Fatalf("failed to query entry after delete: %v", dbErr)
		}
		if os.IsNotExist(statErr) && entry == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := os.Stat(incompletePath); !os.IsNotExist(err) {
		t.Fatalf("expected partial file to be deleted, stat err: %v", err)
	}
	entry, err := state.GetDownload(id)
	if err != nil {
		t.Fatalf("failed to query entry after delete: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected download entry removed from DB, found: %+v", entry)
	}

	listResp, err := doRequest(http.MethodGet, baseURL+"/list")
	if err != nil {
		t.Fatalf("failed to request list: %v", err)
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
