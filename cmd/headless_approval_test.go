package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/download"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
)

func TestHandleDownload_HeadlessMode_AutoApprovesNonDuplicate(t *testing.T) {
	setupIsolatedCmdState(t)

	// Simulation: headless mode (no TUI)
	origServerProgram := serverProgram
	serverProgram = nil
	t.Cleanup(func() { serverProgram = origServerProgram })

	origLifecycle := GlobalLifecycle
	origPool := GlobalPool
	origProgress := GlobalProgressCh
	origService := GlobalService
	t.Cleanup(func() {
		GlobalLifecycle = origLifecycle
		GlobalPool = origPool
		GlobalProgressCh = origProgress
		GlobalService = origService
	})

	// Enable ExtensionPrompt (default is true, but let's be explicit)
	settings := config.DefaultSettings()
	settings.Extension.ExtensionPrompt = true
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Mock server for probe
	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 10))
	}))
	defer probeServer.Close()

	progressCh := make(chan any, 10)
	GlobalProgressCh = progressCh
	GlobalPool = download.NewWorkerPool(progressCh, 1)

	// Mock lifecycle to bypass real downloads
	GlobalLifecycle = processing.NewLifecycleManager(func(url, path, filename string, _ []string, headers map[string]string, explicit bool, totalSize int64, supportsRange bool) (string, error) {
		return "queued-id", nil
	}, nil)

	svc := core.NewLocalDownloadService(GlobalPool)
	GlobalService = svc

	// Verify it auto-approves even with ExtensionPrompt=true
	body := fmt.Sprintf(`{"url": %q, "skip_approval": false}`, probeServer.URL)
	req := httptest.NewRequest(http.MethodPost, "/download", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handleDownload(rec, req, t.TempDir(), svc)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK in headless mode for non-duplicate, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDownload_HeadlessMode_RejectsDuplicateWithWarn(t *testing.T) {
	setupIsolatedCmdState(t)

	// Simulation: headless mode
	origServerProgram := serverProgram
	serverProgram = nil
	t.Cleanup(func() { serverProgram = origServerProgram })

	origPool := GlobalPool
	origProgress := GlobalProgressCh
	origService := GlobalService
	origLifecycle := GlobalLifecycle
	t.Cleanup(func() {
		GlobalPool = origPool
		GlobalProgressCh = origProgress
		GlobalService = origService
		GlobalLifecycle = origLifecycle
	})

	// Enable WarnOnDuplicate
	settings := config.DefaultSettings()
	settings.General.WarnOnDuplicate = true
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	progressCh := make(chan any, 10)
	GlobalProgressCh = progressCh
	GlobalPool = download.NewWorkerPool(progressCh, 1)

	// Seed the DB with a "duplicate" entry
	url := "http://example.com/duplicate.bin"
	_ = state.AddToMasterList(types.DownloadEntry{
		ID:       "dup-id",
		URL:      url,
		Filename: "duplicate.bin",
		Status:   "completed",
	})

	svc := core.NewLocalDownloadService(GlobalPool)
	GlobalService = svc

	// Verify it still rejects duplicates when WarnOnDuplicate is on
	body := fmt.Sprintf(`{"url": %q, "skip_approval": false}`, url)
	req := httptest.NewRequest(http.MethodPost, "/download", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handleDownload(rec, req, t.TempDir(), svc)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for duplicate in headless mode, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDownload_HeadlessMode_RejectsExtensionPromptDuplicate(t *testing.T) {
	setupIsolatedCmdState(t)
	origServerProgram := serverProgram
	serverProgram = nil
	t.Cleanup(func() { serverProgram = origServerProgram })

	origPool := GlobalPool
	origProgress := GlobalProgressCh
	origService := GlobalService
	origLifecycle := GlobalLifecycle
	t.Cleanup(func() {
		GlobalPool = origPool
		GlobalProgressCh = origProgress
		GlobalService = origService
		GlobalLifecycle = origLifecycle
	})

	settings := config.DefaultSettings()
	settings.Extension.ExtensionPrompt = true
	settings.General.WarnOnDuplicate = false
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	url := "http://example.com/already-downloaded.bin"
	_ = state.AddToMasterList(types.DownloadEntry{
		ID: "ext-dup-id", URL: url, Filename: "already-downloaded.bin", Status: "completed",
	})

	progressCh := make(chan any, 10)
	GlobalProgressCh = progressCh
	GlobalPool = download.NewWorkerPool(progressCh, 1)
	svc := core.NewLocalDownloadService(GlobalPool)
	GlobalService = svc

	body := fmt.Sprintf(`{"url": %q, "skip_approval": false}`, url)
	req := httptest.NewRequest(http.MethodPost, "/download", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handleDownload(rec, req, t.TempDir(), svc)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate with ExtensionPrompt=true, got %d: %s", rec.Code, rec.Body.String())
	}
}
