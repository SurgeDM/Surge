package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
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
	settings.Extension.ExtensionPrompt.Value = true
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

	progressCh := make(chan types.DownloadEvent, 10)
	GlobalProgressCh = progressCh
	GlobalPool = scheduler.New(progressCh, 1)

	eventBus := orchestrator.NewEventBus()
	getAll := func() []types.DownloadRecord { return GlobalPool.GetAll() }
	GlobalLifecycle = orchestrator.NewLifecycleManager(GlobalPool, eventBus, buildActiveDownloadChecker(getAll))

	svc := service.NewLocalDownloadService(GlobalLifecycle)
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
	settings.General.WarnOnDuplicate.Value = true
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	progressCh := make(chan types.DownloadEvent, 10)
	GlobalProgressCh = progressCh
	GlobalPool = scheduler.New(progressCh, 1)

	// Seed the DB with a "duplicate" entry
	url := "http://example.com/duplicate.bin"
	_ = store.AddToMasterList(types.DownloadRecord{
		ID:       "dup-id",
		URL:      url,
		Filename: "duplicate.bin",
		Status:   "completed",
	})

	eventBus := orchestrator.NewEventBus()
	getAll := func() []types.DownloadRecord { return GlobalPool.GetAll() }
	GlobalLifecycle = orchestrator.NewLifecycleManager(GlobalPool, eventBus, buildActiveDownloadChecker(getAll))
	svc := service.NewLocalDownloadService(GlobalLifecycle)
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
	settings.Extension.ExtensionPrompt.Value = true
	settings.General.WarnOnDuplicate.Value = false
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	url := "http://example.com/already-downloaded.bin"
	_ = store.AddToMasterList(types.DownloadRecord{
		ID: "ext-dup-id", URL: url, Filename: "already-downloaded.bin", Status: "completed",
	})

	progressCh := make(chan types.DownloadEvent, 10)
	GlobalProgressCh = progressCh
	GlobalPool = scheduler.New(progressCh, 1)
	eventBus := orchestrator.NewEventBus()
	getAll := func() []types.DownloadRecord { return GlobalPool.GetAll() }
	GlobalLifecycle = orchestrator.NewLifecycleManager(GlobalPool, eventBus, buildActiveDownloadChecker(getAll))
	svc := service.NewLocalDownloadService(GlobalLifecycle)
	GlobalService = svc

	body := fmt.Sprintf(`{"url": %q, "skip_approval": false}`, url)
	req := httptest.NewRequest(http.MethodPost, "/download", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handleDownload(rec, req, t.TempDir(), svc)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate with ExtensionPrompt=true, got %d: %s", rec.Code, rec.Body.String())
	}
}
