package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adrg/xdg"
	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/download"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
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

func TestCLI_NewEndpoints(t *testing.T) {
	tempDir := t.TempDir()

	// xdg package variables are initialized on load, so setting env vars
	// won't change the paths in tests. Directly override them instead.
	oldConfigHome := xdg.ConfigHome
	oldDataHome := xdg.DataHome
	oldStateHome := xdg.StateHome
	oldCacheHome := xdg.CacheHome
	oldRuntimeDir := xdg.RuntimeDir

	xdg.ConfigHome = tempDir
	xdg.DataHome = tempDir
	xdg.StateHome = tempDir
	xdg.CacheHome = tempDir
	xdg.RuntimeDir = tempDir

	t.Cleanup(func() {
		xdg.ConfigHome = oldConfigHome
		xdg.DataHome = oldDataHome
		xdg.StateHome = oldStateHome
		xdg.CacheHome = oldCacheHome
		xdg.RuntimeDir = oldRuntimeDir
	})

	t.Setenv("APPDATA", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", tempDir)
	t.Setenv("XDG_RUNTIME_DIR", tempDir)
	t.Setenv("HOME", tempDir)

	state.CloseDB()
	if err := config.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}
	initializeGlobalState()

	// Initialize GlobalPool for tests
	GlobalProgressCh = make(chan any, 100)
	GlobalPool = download.NewWorkerPool(GlobalProgressCh, 4)

	// Start server
	svc := core.NewLocalDownloadService(GlobalPool)
	const authToken = "test-token-new-endpoints"
	baseURL := startAuthedTestServer(t, svc, authToken)

	seedPausedState := func(id string) {
		testURL := baseURL + "/health?seed=" + id
		destPath := filepath.Join(tempDir, id+".bin")
		if err := state.SaveState(testURL, destPath, &types.DownloadState{
			ID:         id,
			URL:        testURL,
			DestPath:   destPath,
			Filename:   id + ".bin",
			TotalSize:  1024,
			Downloaded: 0,
			Tasks: []types.Task{
				{Offset: 0, Length: 1024},
			},
		}); err != nil {
			t.Fatalf("failed to seed paused state for %s: %v", id, err)
		}
	}

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

	t.Run("Pause Endpoint", func(t *testing.T) {
		id := "test-pause-id"
		seedPausedState(id)

		resp, err := doRequest(http.MethodPost, baseURL+"/pause?id="+id)
		if err != nil {
			t.Fatalf("Failed to request pause: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if result["status"] != "paused" {
			t.Errorf("Expected status 'paused', got %v", result["status"])
		}
	})

	t.Run("Resume Endpoint", func(t *testing.T) {
		id := "test-resume-id"
		seedPausedState(id)

		resp, err := doRequest(http.MethodPost, baseURL+"/resume?id="+id)
		if err != nil {
			t.Fatalf("Failed to request resume: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if result["status"] != "resumed" {
			t.Errorf("Expected status 'resumed', got %v", result["status"])
		}

		// Cleanup resumed download so it cannot interfere with following tests.
		cleanupResp, cleanupErr := doRequest(http.MethodPost, baseURL+"/delete?id="+id)
		if cleanupErr != nil {
			t.Fatalf("Failed to cleanup resumed download: %v", cleanupErr)
		}
		defer func() { _ = cleanupResp.Body.Close() }()
		if cleanupResp.StatusCode != http.StatusOK {
			t.Fatalf("Cleanup delete expected 200 OK, got %d", cleanupResp.StatusCode)
		}
	})

	t.Run("Delete Endpoint", func(t *testing.T) {
		id := "test-delete-id"
		seedPausedState(id)

		resp, err := doRequest(http.MethodPost, baseURL+"/delete?id="+id)
		if err != nil {
			t.Fatalf("Failed to request delete: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if result["status"] != "deleted" {
			t.Errorf("Expected status 'deleted', got %v", result["status"])
		}
	})

	t.Run("Delete Missing ID Endpoint", func(t *testing.T) {
		resp, err := doRequest(http.MethodPost, baseURL+"/delete") // Missing ID
		if err != nil {
			t.Fatalf("Failed request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for missing ID, got %d", resp.StatusCode)
		}
	})
}

func TestCLI_DeleteEndpoint_CleansPausedStateAndPartialFile(t *testing.T) {
	tempDir := t.TempDir()

	oldConfigHome := xdg.ConfigHome
	oldDataHome := xdg.DataHome
	oldStateHome := xdg.StateHome
	oldCacheHome := xdg.CacheHome
	oldRuntimeDir := xdg.RuntimeDir

	xdg.ConfigHome = tempDir
	xdg.DataHome = tempDir
	xdg.StateHome = tempDir
	xdg.CacheHome = tempDir
	xdg.RuntimeDir = tempDir

	t.Cleanup(func() {
		xdg.ConfigHome = oldConfigHome
		xdg.DataHome = oldDataHome
		xdg.StateHome = oldStateHome
		xdg.CacheHome = oldCacheHome
		xdg.RuntimeDir = oldRuntimeDir
		state.CloseDB()
	})

	t.Setenv("APPDATA", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", tempDir)
	t.Setenv("XDG_RUNTIME_DIR", tempDir)
	t.Setenv("HOME", tempDir)

	state.CloseDB()
	initializeGlobalState()

	GlobalProgressCh = make(chan any, 100)
	GlobalPool = download.NewWorkerPool(GlobalProgressCh, 2)

	// Start server
	svc := core.NewLocalDownloadService(GlobalPool)
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
