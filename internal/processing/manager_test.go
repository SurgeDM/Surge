package processing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/testutil"
)

func newProbeTestServer(t *testing.T, size int64) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=0-0" {
			t.Fatalf("Range header = %q, want bytes=0-0", got)
		}

		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", size))
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
}

func newLifecycleManagerForTest() *LifecycleManager {
	settings := config.DefaultSettings()
	settings.Categories.CategoryEnabled = false
	sem := make(chan struct{}, maxConcurrentProbes)
	for i := 0; i < maxConcurrentProbes; i++ {
		sem <- struct{}{}
	}
	return &LifecycleManager{
		settings:            settings,
		settingsRefreshedAt: time.Now(),
		probeSem:            sem,
	}
}

func TestLifecycleManager_Enqueue_PrecreatesWorkingFileBeforeDispatch(t *testing.T) {
	server := newProbeTestServer(t, 1234)
	defer server.Close()

	tempDir := t.TempDir()
	expectedFile := "archive.zip"
	expectedID := "enqueue-id"

	mgr := newLifecycleManagerForTest()
	mgr.addFunc = func(url, path, filename string, _ []string, _ map[string]string, explicit bool, totalSize int64, supportsRange bool) (string, error) {
		if url != server.URL {
			t.Fatalf("url = %q, want %q", url, server.URL)
		}
		if path != tempDir {
			t.Fatalf("path = %q, want %q", path, tempDir)
		}
		if filename != expectedFile {
			t.Fatalf("filename = %q, want %q", filename, expectedFile)
		}
		if !explicit {
			t.Fatal("expected explicit category flag to be preserved")
		}
		if totalSize != 1234 {
			t.Fatalf("totalSize = %d, want 1234", totalSize)
		}
		if !supportsRange {
			t.Fatal("expected range support from probe")
		}

		surgePath := filepath.Join(path, filename) + types.IncompleteSuffix
		if _, err := os.Stat(surgePath); err != nil {
			t.Fatalf("expected working file to exist before dispatch: %v", err)
		}

		return expectedID, nil
	}

	req := &DownloadRequest{
		URL:                server.URL,
		Filename:           expectedFile,
		Path:               tempDir,
		IsExplicitCategory: true,
	}

	id, _, err := mgr.Enqueue(context.Background(), req)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if id != expectedID {
		t.Fatalf("id = %q, want %q", id, expectedID)
	}

	surgePath := filepath.Join(tempDir, expectedFile) + types.IncompleteSuffix
	if _, err := os.Stat(surgePath); err != nil {
		t.Fatalf("expected working file to remain after queueing: %v", err)
	}
}

func TestLifecycleManager_EnqueueWithID_PrecreatesWorkingFileBeforeDispatch(t *testing.T) {
	server := newProbeTestServer(t, 4321)
	defer server.Close()

	tempDir := t.TempDir()
	expectedFile := "archive.zip"
	expectedID := "request-id"

	mgr := newLifecycleManagerForTest()
	mgr.addWithIDFunc = func(url, path, filename string, _ []string, _ map[string]string, requestID string, totalSize int64, supportsRange bool) (string, error) {
		if url != server.URL {
			t.Fatalf("url = %q, want %q", url, server.URL)
		}
		if path != tempDir {
			t.Fatalf("path = %q, want %q", path, tempDir)
		}
		if filename != expectedFile {
			t.Fatalf("filename = %q, want %q", filename, expectedFile)
		}
		if requestID != expectedID {
			t.Fatalf("requestID = %q, want %q", requestID, expectedID)
		}
		if totalSize != 4321 {
			t.Fatalf("totalSize = %d, want 4321", totalSize)
		}
		if !supportsRange {
			t.Fatal("expected range support from probe")
		}

		surgePath := filepath.Join(path, filename) + types.IncompleteSuffix
		if _, err := os.Stat(surgePath); err != nil {
			t.Fatalf("expected working file to exist before dispatch: %v", err)
		}

		return requestID, nil
	}

	req := &DownloadRequest{
		URL:                server.URL,
		Filename:           expectedFile,
		Path:               tempDir,
		IsExplicitCategory: true,
	}

	id, _, err := mgr.EnqueueWithID(context.Background(), req, expectedID)
	if err != nil {
		t.Fatalf("EnqueueWithID failed: %v", err)
	}
	if id != expectedID {
		t.Fatalf("id = %q, want %q", id, expectedID)
	}

	surgePath := filepath.Join(tempDir, expectedFile) + types.IncompleteSuffix
	if _, err := os.Stat(surgePath); err != nil {
		t.Fatalf("expected working file to remain after queueing: %v", err)
	}
}

func TestLifecycleManager_EnqueueWithID_ProbeFailureStillDispatches(t *testing.T) {
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	expectedID := "request-id"

	mgr := newLifecycleManagerForTest()
	mgr.addWithIDFunc = func(url, path, filename string, _ []string, _ map[string]string, requestID string, totalSize int64, supportsRange bool) (string, error) {
		if url != server.URL {
			t.Fatalf("url = %q, want %q", url, server.URL)
		}
		if path != tempDir {
			t.Fatalf("path = %q, want %q", path, tempDir)
		}
		if filename != "fallback.bin" {
			t.Fatalf("filename = %q, want fallback.bin", filename)
		}
		if requestID != expectedID {
			t.Fatalf("requestID = %q, want %q", requestID, expectedID)
		}
		if totalSize != 0 {
			t.Fatalf("totalSize = %d, want 0", totalSize)
		}
		if !supportsRange {
			t.Fatal("expected optimistic concurrent flag after probe failure")
		}
		return requestID, nil
	}

	id, filename, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
		URL:      server.URL,
		Filename: "fallback.bin",
		Path:     tempDir,
	}, expectedID)
	if err != nil {
		t.Fatalf("EnqueueWithID failed: %v", err)
	}
	if id != expectedID {
		t.Fatalf("id = %q, want %q", id, expectedID)
	}
	if filename != "fallback.bin" {
		t.Fatalf("filename = %q, want fallback.bin", filename)
	}
}

func TestLifecycleManager_Enqueue_RemovesWorkingFileOnDispatchError(t *testing.T) {
	server := newProbeTestServer(t, 2048)
	defer server.Close()

	tempDir := t.TempDir()
	expectedFile := "broken.zip"
	expectedErr := errors.New("dispatch failed")

	mgr := newLifecycleManagerForTest()
	mgr.addFunc = func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
		return "", expectedErr
	}

	_, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
		URL:                server.URL,
		Filename:           expectedFile,
		Path:               tempDir,
		IsExplicitCategory: true,
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("err = %v, want %v", err, expectedErr)
	}

	surgePath := filepath.Join(tempDir, expectedFile) + types.IncompleteSuffix
	if _, statErr := os.Stat(surgePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected working file cleanup after dispatch failure, stat err: %v", statErr)
	}
}

func TestLifecycleManager_Enqueue_RetriesWhenWorkingFileReservationCollides(t *testing.T) {
	server := newProbeTestServer(t, 1024)
	defer server.Close()

	tempDir := t.TempDir()

	origReserve := reserveWorkingFile
	t.Cleanup(func() {
		reserveWorkingFile = origReserve
	})

	var reserveCalls int
	reserveWorkingFile = func(destPath, filename string) error {
		reserveCalls++
		if reserveCalls == 1 {
			surgePath := filepath.Join(destPath, filename) + types.IncompleteSuffix
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				t.Fatalf("failed to create temp dir for collision: %v", err)
			}
			if err := os.WriteFile(surgePath, []byte("occupied"), 0o644); err != nil {
				t.Fatalf("failed to seed colliding working file: %v", err)
			}
			return fmt.Errorf("collision: %w", os.ErrExist)
		}
		return precreateWorkingFile(destPath, filename)
	}

	mgr := newLifecycleManagerForTest()
	var dispatchedFilename string
	mgr.addFunc = func(url, path, filename string, _ []string, _ map[string]string, explicit bool, totalSize int64, supportsRange bool) (string, error) {
		dispatchedFilename = filename
		if path != tempDir {
			t.Fatalf("path = %q, want %q", path, tempDir)
		}
		if explicit != true {
			t.Fatal("expected explicit category flag to be preserved")
		}
		if totalSize != 1024 || !supportsRange {
			t.Fatalf("unexpected probe metadata: total=%d range=%v", totalSize, supportsRange)
		}
		return "retry-id", nil
	}

	id, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
		URL:                server.URL,
		Filename:           "archive.zip",
		Path:               tempDir,
		IsExplicitCategory: true,
	})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if id != "retry-id" {
		t.Fatalf("id = %q, want retry-id", id)
	}
	if dispatchedFilename != "archive(1).zip" {
		t.Fatalf("filename = %q, want archive(1).zip", dispatchedFilename)
	}
	if reserveCalls < 2 {
		t.Fatalf("reserve calls = %d, want at least 2", reserveCalls)
	}

	firstSurgePath := filepath.Join(tempDir, "archive.zip") + types.IncompleteSuffix
	if _, err := os.Stat(firstSurgePath); err != nil {
		t.Fatalf("expected first reservation to remain in place: %v", err)
	}
	retriedSurgePath := filepath.Join(tempDir, "archive(1).zip") + types.IncompleteSuffix
	if _, err := os.Stat(retriedSurgePath); err != nil {
		t.Fatalf("expected retried reservation to exist: %v", err)
	}
}

func TestLifecycleManager_EnqueueWithID_RetriesWhenWorkingFileReservationCollides(t *testing.T) {
	server := newProbeTestServer(t, 1024)
	defer server.Close()

	tempDir := t.TempDir()
	requestID := "request-id"

	origReserve := reserveWorkingFile
	t.Cleanup(func() {
		reserveWorkingFile = origReserve
	})

	var reserveCalls int
	reserveWorkingFile = func(destPath, filename string) error {
		reserveCalls++
		if reserveCalls == 1 {
			surgePath := filepath.Join(destPath, filename) + types.IncompleteSuffix
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				t.Fatalf("failed to create temp dir for collision: %v", err)
			}
			if err := os.WriteFile(surgePath, []byte("occupied"), 0o644); err != nil {
				t.Fatalf("failed to seed colliding working file: %v", err)
			}
			return fmt.Errorf("collision: %w", os.ErrExist)
		}
		return precreateWorkingFile(destPath, filename)
	}

	mgr := newLifecycleManagerForTest()
	var dispatchedFilename string
	mgr.addWithIDFunc = func(url, path, filename string, _ []string, _ map[string]string, gotRequestID string, totalSize int64, supportsRange bool) (string, error) {
		dispatchedFilename = filename
		if path != tempDir {
			t.Fatalf("path = %q, want %q", path, tempDir)
		}
		if gotRequestID != requestID {
			t.Fatalf("requestID = %q, want %q", gotRequestID, requestID)
		}
		if totalSize != 1024 || !supportsRange {
			t.Fatalf("unexpected probe metadata: total=%d range=%v", totalSize, supportsRange)
		}
		return gotRequestID, nil
	}

	id, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
		URL:                server.URL,
		Filename:           "archive.zip",
		Path:               tempDir,
		IsExplicitCategory: true,
	}, requestID)
	if err != nil {
		t.Fatalf("EnqueueWithID failed: %v", err)
	}
	if id != requestID {
		t.Fatalf("id = %q, want %q", id, requestID)
	}
	if dispatchedFilename != "archive(1).zip" {
		t.Fatalf("filename = %q, want archive(1).zip", dispatchedFilename)
	}
	if reserveCalls < 2 {
		t.Fatalf("reserve calls = %d, want at least 2", reserveCalls)
	}

	firstSurgePath := filepath.Join(tempDir, "archive.zip") + types.IncompleteSuffix
	if _, err := os.Stat(firstSurgePath); err != nil {
		t.Fatalf("expected first reservation to remain in place: %v", err)
	}
	retriedSurgePath := filepath.Join(tempDir, "archive(1).zip") + types.IncompleteSuffix
	if _, err := os.Stat(retriedSurgePath); err != nil {
		t.Fatalf("expected retried reservation to exist: %v", err)
	}
}

func TestLifecycleManager_EnqueueWithID_RemovesWorkingFileOnDispatchError(t *testing.T) {
	server := newProbeTestServer(t, 2048)
	defer server.Close()

	tempDir := t.TempDir()
	expectedFile := "broken.zip"
	expectedErr := errors.New("dispatch failed")

	mgr := newLifecycleManagerForTest()
	mgr.addWithIDFunc = func(string, string, string, []string, map[string]string, string, int64, bool) (string, error) {
		return "", expectedErr
	}

	_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
		URL:                server.URL,
		Filename:           expectedFile,
		Path:               tempDir,
		IsExplicitCategory: true,
	}, "request-id")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("err = %v, want %v", err, expectedErr)
	}

	surgePath := filepath.Join(tempDir, expectedFile) + types.IncompleteSuffix
	if _, statErr := os.Stat(surgePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected working file cleanup after dispatch failure, stat err: %v", statErr)
	}
}

func TestLifecycleManager_Enqueue_FailsAfterReservationAttemptLimit(t *testing.T) {
	server := newProbeTestServer(t, 2048)
	defer server.Close()

	tempDir := t.TempDir()

	origReserve := reserveWorkingFile
	origTTL := settingsRefreshTTL
	t.Cleanup(func() {
		reserveWorkingFile = origReserve
		settingsRefreshTTL = origTTL
	})
	settingsRefreshTTL = time.Hour

	var reserveCalls int
	reserveWorkingFile = func(string, string) error {
		reserveCalls++
		return fmt.Errorf("collision: %w", os.ErrExist)
	}

	mgr := newLifecycleManagerForTest()
	mgr.addFunc = func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
		t.Fatal("dispatch should not run when reservation never succeeds")
		return "", nil
	}

	_, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
		URL:                server.URL,
		Filename:           "archive.zip",
		Path:               tempDir,
		IsExplicitCategory: true,
	})
	if err == nil {
		t.Fatal("expected reservation exhaustion error")
	}
	if reserveCalls != maxWorkingFileReservationAttempts {
		t.Fatalf("reserve calls = %d, want %d", reserveCalls, maxWorkingFileReservationAttempts)
	}
}

func TestLifecycleManager_GetSettings_RefreshesFromDiskAfterTTL(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	initial := config.DefaultSettings()
	initial.Categories.CategoryEnabled = false
	if err := config.SaveSettings(initial); err != nil {
		t.Fatalf("SaveSettings(initial) failed: %v", err)
	}

	mgr := NewLifecycleManager(nil, nil)

	updated := config.DefaultSettings()
	updated.Categories.CategoryEnabled = true
	if err := config.SaveSettings(updated); err != nil {
		t.Fatalf("SaveSettings(updated) failed: %v", err)
	}

	origTTL := settingsRefreshTTL
	t.Cleanup(func() {
		settingsRefreshTTL = origTTL
	})
	settingsRefreshTTL = 0

	settings := mgr.GetSettings()
	if !settings.Categories.CategoryEnabled {
		t.Fatal("expected GetSettings to pick up saved settings after TTL expiry")
	}
}

func TestLifecycleManager_GetSettings_KeepsCachedSnapshotWhenReloadFails(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	initial := config.DefaultSettings()
	initial.General.WarnOnDuplicate = false
	if err := config.SaveSettings(initial); err != nil {
		t.Fatalf("SaveSettings(initial) failed: %v", err)
	}

	mgr := NewLifecycleManager(nil, nil)

	badConfigHome := filepath.Join(tmpDir, "bad-config-home")
	if err := os.WriteFile(badConfigHome, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(badConfigHome) failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", badConfigHome)

	origTTL := settingsRefreshTTL
	t.Cleanup(func() {
		settingsRefreshTTL = origTTL
	})
	settingsRefreshTTL = 0

	settings := mgr.GetSettings()
	if settings.General.WarnOnDuplicate {
		t.Fatal("expected GetSettings to keep the cached snapshot when disk reload fails")
	}
}

func TestLifecycleManager_Enqueue_ContextCancellationDuringProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	mgr := newLifecycleManagerForTest()
	mgr.addFunc = func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
		t.Fatal("dispatch should not run when probe fails")
		return "", nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := mgr.Enqueue(ctx, &DownloadRequest{
		URL:      server.URL,
		Filename: "test.zip",
		Path:     t.TempDir(),
	})

	if err == nil {
		t.Fatal("expected error due to context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestLifecycleManager_EnqueueWithID_FailsAfterReservationAttemptLimit(t *testing.T) {
	server := newProbeTestServer(t, 2048)
	defer server.Close()

	tempDir := t.TempDir()

	origReserve := reserveWorkingFile
	origTTL := settingsRefreshTTL
	t.Cleanup(func() {
		reserveWorkingFile = origReserve
		settingsRefreshTTL = origTTL
	})
	settingsRefreshTTL = time.Hour

	var reserveCalls int
	reserveWorkingFile = func(string, string) error {
		reserveCalls++
		return fmt.Errorf("collision: %w", os.ErrExist)
	}

	mgr := newLifecycleManagerForTest()
	mgr.addWithIDFunc = func(string, string, string, []string, map[string]string, string, int64, bool) (string, error) {
		t.Fatal("dispatch should not run when reservation never succeeds")
		return "", nil
	}

	_, _, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
		URL:                server.URL,
		Filename:           "archive.zip",
		Path:               tempDir,
		IsExplicitCategory: true,
	}, "test-id")

	if err == nil {
		t.Fatal("expected reservation exhaustion error")
	}
	if reserveCalls != maxWorkingFileReservationAttempts {
		t.Fatalf("reserve calls = %d, want %d", reserveCalls, maxWorkingFileReservationAttempts)
	}
}

func TestLifecycleManager_Enqueue_EmptyURL(t *testing.T) {
	mgr := newLifecycleManagerForTest()
	_, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
		URL:  "",
		Path: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error with empty URL")
	}
}

func TestLifecycleManager_Enqueue_EmptyPath(t *testing.T) {
	server := newProbeTestServer(t, 2048)
	defer server.Close()
	mgr := newLifecycleManagerForTest()
	_, _, err := mgr.Enqueue(context.Background(), &DownloadRequest{
		URL:  server.URL,
		Path: "",
	})
	if err == nil {
		t.Fatal("expected error with empty path")
	}
}

func TestLifecycleManager_Enqueue_ContextCancellationBeforeReservation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/2048")
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
		// Cancel context so it takes effect right after probe returns
		cancel()
	}))
	defer server.Close()

	mgr := newLifecycleManagerForTest()
	mgr.addFunc = func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
		t.Fatal("dispatch should not run when context is canceled before reservation")
		return "", nil
	}

	_, _, err := mgr.Enqueue(ctx, &DownloadRequest{
		URL:      server.URL,
		Filename: "test.zip",
		Path:     t.TempDir(),
	})

	if err == nil {
		t.Fatal("expected error due to context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// --- LifecycleManager.Resume / Cancel / UpdateURL Tests ---

func TestLifecycleManager_Resume_HotPath(t *testing.T) {
	var extractCalled, addCalled bool
	var publishedEvent interface{}

	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		GetStatus: func(id string) *types.DownloadStatus {
			return &types.DownloadStatus{Status: "paused"}
		},
		ExtractPausedConfig: func(id string) *types.DownloadConfig {
			extractCalled = true
			return &types.DownloadConfig{ID: "hot-id", Filename: "hot-file.zip"}
		},
		AddConfig: func(cfg types.DownloadConfig) {
			addCalled = true
			if cfg.ID != "hot-id" {
				t.Errorf("AddConfig ID = %q, want hot-id", cfg.ID)
			}
			if !cfg.IsResume {
				t.Error("expected IsResume flag to be set")
			}
		},
		PublishEvent: func(msg interface{}) error {
			publishedEvent = msg
			return nil
		},
	})

	if err := mgr.Resume("hot-id"); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if !extractCalled {
		t.Error("Expected ExtractPausedConfig to be called for hot path")
	}
	if !addCalled {
		t.Error("Expected AddConfig to be called")
	}
	if publishedEvent == nil {
		t.Fatal("Expected PublishEvent to be called")
	}
	msg, ok := publishedEvent.(events.DownloadResumedMsg)
	if !ok {
		t.Fatalf("Expected DownloadResumedMsg, got %T", publishedEvent)
	}
	if msg.DownloadID != "hot-id" {
		t.Errorf("ResumedMsg.DownloadID = %q, want hot-id", msg.DownloadID)
	}
	if msg.Filename != "hot-file.zip" {
		t.Errorf("ResumedMsg.Filename = %q, want hot-file.zip", msg.Filename)
	}
}

func TestLifecycleManager_Resume_ColdPath(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)
	destPath := filepath.Join(tempDir, "cold-file.zip")

	testutil.SeedMasterList(t, types.DownloadEntry{
		ID:         "cold-id",
		URL:        "http://example.com/cold.zip",
		URLHash:    state.URLHash("http://example.com/cold.zip"),
		DestPath:   destPath,
		Filename:   "cold-file.zip",
		Status:     "paused",
		Downloaded: 500,
		TotalSize:  1000,
	})

	var addCalled bool
	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		ExtractPausedConfig: func(id string) *types.DownloadConfig { return nil },
		AddConfig: func(cfg types.DownloadConfig) {
			addCalled = true
			if cfg.ID != "cold-id" {
				t.Errorf("AddConfig ID = %q, want cold-id", cfg.ID)
			}
			if !cfg.IsResume {
				t.Error("expected IsResume flag")
			}
		},
	})

	if err := mgr.Resume("cold-id"); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if !addCalled {
		t.Error("Expected AddConfig to be called for cold path")
	}
}

func TestLifecycleManager_Resume_NotFound(t *testing.T) {
	testutil.SetupStateDB(t)

	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		ExtractPausedConfig: func(id string) *types.DownloadConfig { return nil },
	})

	err := mgr.Resume("missing-id")
	if err == nil {
		t.Fatal("expected error for unknown download")
	}
}

func TestLifecycleManager_Resume_Completed(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)

	testutil.SeedMasterList(t, types.DownloadEntry{
		ID:       "done-id",
		URL:      "http://example.com/done.zip",
		URLHash:  state.URLHash("http://example.com/done.zip"),
		DestPath: filepath.Join(tempDir, "done.zip"),
		Filename: "done.zip",
		Status:   "completed",
	})

	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		ExtractPausedConfig: func(id string) *types.DownloadConfig { return nil },
	})

	err := mgr.Resume("done-id")
	if err == nil {
		t.Fatal("expected error for completed download")
	}
}

func TestLifecycleManager_Resume_StillPausing(t *testing.T) {
	var extraCalled bool
	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		GetStatus: func(id string) *types.DownloadStatus {
			return &types.DownloadStatus{Status: "pausing"}
		},
		ExtractPausedConfig: func(id string) *types.DownloadConfig {
			extraCalled = true
			return nil
		},
	})

	err := mgr.Resume("pausing-id")
	if err == nil {
		t.Fatal("expected error when download is still pausing")
	}
	if extraCalled {
		t.Error("Expected ExtractPausedConfig to NOT be called while pausing")
	}
}

func TestLifecycleManager_Resume_HydratesFromDisk(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)
	destPath := filepath.Join(tempDir, "hydrated.zip")

	testutil.SeedMasterList(t, types.DownloadEntry{
		ID:       "hydrate-id",
		URL:      "http://example.com/hydrated.zip",
		URLHash:  state.URLHash("http://example.com/hydrated.zip"),
		DestPath: destPath,
		Filename: "hydrated.zip",
		Status:   "paused",
	})

	if err := state.SaveStateWithOptions("http://example.com/hydrated.zip", destPath, &types.DownloadState{
		ID: "hydrate-id", URL: "http://example.com/hydrated.zip", Filename: "hydrated.zip",
		DestPath: destPath, TotalSize: 5000,
		Tasks: []types.Task{{Offset: 0, Length: 2500}, {Offset: 2500, Length: 2500}},
	}, state.SaveStateOptions{SkipFileHash: true}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	var addedCfg *types.DownloadConfig
	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		GetStatus: func(id string) *types.DownloadStatus { return &types.DownloadStatus{Status: "paused"} },
		ExtractPausedConfig: func(id string) *types.DownloadConfig {
			return &types.DownloadConfig{ID: id, Filename: "hydrated.zip", URL: "http://example.com/hydrated.zip", DestPath: destPath}
		},
		AddConfig: func(cfg types.DownloadConfig) { addedCfg = &cfg },
	})

	if err := mgr.Resume("hydrate-id"); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if addedCfg == nil {
		t.Fatal("Expected AddConfig to be called")
	}
	if addedCfg.TotalSize != 5000 {
		t.Errorf("TotalSize = %d, want 5000", addedCfg.TotalSize)
	}
	if !addedCfg.SupportsRange {
		t.Error("Expected SupportsRange = true after loading tasks")
	}
}

func TestLifecycleManager_Cancel_FromPool(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)

	testutil.SeedMasterList(t, types.DownloadEntry{
		ID:       "active-cancel",
		URL:      "http://example.com/active.zip",
		URLHash:  state.URLHash("http://example.com/active.zip"),
		DestPath: filepath.Join(tempDir, "active.zip"),
		Filename: "active.zip",
		Status:   "downloading",
	})

	var publishedMsg interface{}
	cancelResult := types.CancelResult{Found: true, Filename: "active.zip", DestPath: filepath.Join(tempDir, "active.zip")}

	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		Cancel:       func(id string) types.CancelResult { return cancelResult },
		PublishEvent: func(msg interface{}) error { publishedMsg = msg; return nil },
	})

	if err := mgr.Cancel("active-cancel"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	if publishedMsg == nil {
		t.Fatal("Expected DownloadRemovedMsg to be published")
	}
	removed, ok := publishedMsg.(events.DownloadRemovedMsg)
	if !ok {
		t.Fatalf("Expected DownloadRemovedMsg, got %T", publishedMsg)
	}
	if removed.DownloadID != "active-cancel" {
		t.Errorf("RemovedMsg.DownloadID = %q, want active-cancel", removed.DownloadID)
	}
	if removed.Filename != "active.zip" {
		t.Errorf("RemovedMsg.Filename = %q, want active.zip", removed.Filename)
	}
}

func TestLifecycleManager_Cancel_DBOnly(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)
	destPath := filepath.Join(tempDir, "db-only.zip")

	testutil.SeedMasterList(t, types.DownloadEntry{
		ID:       "db-only",
		URL:      "http://example.com/db-only.zip",
		URLHash:  state.URLHash("http://example.com/db-only.zip"),
		DestPath: destPath,
		Filename: "db-only.zip",
		Status:   "paused",
	})

	var publishedMsg interface{}

	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		Cancel:       func(id string) types.CancelResult { return types.CancelResult{Found: false} },
		PublishEvent: func(msg interface{}) error { publishedMsg = msg; return nil },
	})

	if err := mgr.Cancel("db-only"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	removed, ok := publishedMsg.(events.DownloadRemovedMsg)
	if !ok {
		t.Fatalf("Expected DownloadRemovedMsg, got %T", publishedMsg)
	}
	if removed.DownloadID != "db-only" {
		t.Errorf("RemovedMsg.DownloadID = %q, want db-only", removed.DownloadID)
	}
	if removed.Filename != "db-only.zip" {
		t.Errorf("RemovedMsg.Filename = %q, want db-only.zip", removed.Filename)
	}
}

func TestLifecycleManager_Cancel_Completed(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)
	destPath := filepath.Join(tempDir, "completed.zip")

	testutil.SeedMasterList(t, types.DownloadEntry{
		ID:       "completed-cancel",
		URL:      "http://example.com/completed.zip",
		URLHash:  state.URLHash("http://example.com/completed.zip"),
		DestPath: destPath,
		Filename: "completed.zip",
		Status:   "completed",
	})

	var publishedMsg interface{}

	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		Cancel:       func(id string) types.CancelResult { return types.CancelResult{} },
		PublishEvent: func(msg interface{}) error { publishedMsg = msg; return nil },
	})

	if err := mgr.Cancel("completed-cancel"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	removed, ok := publishedMsg.(events.DownloadRemovedMsg)
	if !ok {
		t.Fatalf("Expected DownloadRemovedMsg, got %T", publishedMsg)
	}
	if !removed.Completed {
		t.Error("Expected Completed=true for a completed download")
	}
	if removed.Filename != "completed.zip" {
		t.Errorf("RemovedMsg.Filename = %q, want completed.zip", removed.Filename)
	}
}

func TestLifecycleManager_Cancel_NotFound(t *testing.T) {
	testutil.SetupStateDB(t)

	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		Cancel: func(id string) types.CancelResult { return types.CancelResult{Found: false} },
	})

	err := mgr.Cancel("ghost-id")
	if err == nil {
		t.Fatal("expected error for non-existent download")
	}
}

func TestLifecycleManager_UpdateURL_Success(t *testing.T) {
	tempDir := testutil.SetupStateDB(t)

	testutil.SeedMasterList(t, types.DownloadEntry{
		ID:       "update-id",
		URL:      "http://example.com/old.zip",
		URLHash:  state.URLHash("http://example.com/old.zip"),
		DestPath: filepath.Join(tempDir, "update.zip"),
		Filename: "update.zip",
		Status:   "paused",
	})

	var hookCalled bool
	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		UpdateURL: func(id, newURL string) error {
			hookCalled = true
			if newURL != "http://example.com/new.zip" {
				t.Errorf("UpdateURL newURL = %q", newURL)
			}
			return nil
		},
	})

	if err := mgr.UpdateURL("update-id", "http://example.com/new.zip"); err != nil {
		t.Fatalf("UpdateURL failed: %v", err)
	}
	if !hookCalled {
		t.Error("Expected UpdateURL hook to be called")
	}

	entry, err := state.GetDownload("update-id")
	if err != nil {
		t.Fatalf("GetDownload failed: %v", err)
	}
	if entry == nil || entry.URL != "http://example.com/new.zip" {
		t.Errorf("DB URL = %q, want http://example.com/new.zip", entry.URL)
	}
}

func TestLifecycleManager_UpdateURL_HookError(t *testing.T) {
	testutil.SetupStateDB(t)

	expectedErr := fmt.Errorf("not in pausable state")
	mgr := newLifecycleManagerForTest()
	mgr.SetEngineHooks(EngineHooks{
		UpdateURL: func(id, newURL string) error { return expectedErr },
	})

	err := mgr.UpdateURL("bad-id", "http://example.com/new.zip")
	if err == nil {
		t.Fatal("expected error from pool hook, got nil")
	}
}

// --- Probe semaphore tests ---

// newSlowProbeServer creates an httptest server that serves 206 Partial Content
// but delays each response by `delay` so we can observe concurrency behaviour.
func newSlowProbeServer(t *testing.T, size int64, delay time.Duration, inflight *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(inflight, 1)
		defer atomic.AddInt32(inflight, -1)
		time.Sleep(delay)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", size))
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
}

// TestLifecycleManager_ProbeSemaphore_LimitsInflight fires N concurrent Enqueue
// calls against a slow probe server and verifies the peak in-flight count never
// exceeds maxConcurrentProbes.
func TestLifecycleManager_ProbeSemaphore_LimitsInflight(t *testing.T) {
	const numDownloads = 9 // 3× the semaphore cap

	var peak int32 // highest observed in-flight count
	var inflight int32

	server := newSlowProbeServer(t, 1024, 50*time.Millisecond, &inflight)
	defer server.Close()

	// Wrap the probe so we can record the peak without changing production code.
	// We do this by polling inflight from a separate goroutine while probes run.
	stopPoller := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopPoller:
				return
			default:
				cur := atomic.LoadInt32(&inflight)
				for {
					prev := atomic.LoadInt32(&peak)
					if cur <= prev || atomic.CompareAndSwapInt32(&peak, prev, cur) {
						break
					}
				}
				time.Sleep(time.Millisecond)
			}
		}
	}()

	mgr := newLifecycleManagerForTest()
	mgr.addFunc = func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
		return "", fmt.Errorf("dispatch intentionally rejected for test")
	}
	settings := config.DefaultSettings()
	settings.Categories.CategoryEnabled = false
	mgr.ApplySettings(settings)

	tempDir := t.TempDir()

	var wg sync.WaitGroup
	errs := make([]error, numDownloads)
	for i := 0; i < numDownloads; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _, errs[idx] = mgr.Enqueue(context.Background(), &DownloadRequest{
				URL:      server.URL,
				Filename: fmt.Sprintf("file%d.zip", idx),
				Path:     tempDir,
			})
		}(i)
	}
	wg.Wait()
	close(stopPoller)

	// addFunc is nil so every enqueue fails after probe — that's fine;
	// we only care that the probe phase was throttled.
	for _, err := range errs {
		if err == nil {
			t.Error("expected Enqueue to fail (nil addFunc), got nil error")
		}
	}

	observed := atomic.LoadInt32(&peak)
	if observed > maxConcurrentProbes {
		t.Errorf("peak concurrent probes = %d, want ≤ %d", observed, maxConcurrentProbes)
	}
}

// TestLifecycleManager_ProbeSemaphore_CancelledContextAbortsWait verifies that
// a queued enqueue waiting on the semaphore returns immediately when its context
// is cancelled, without needing to wait for a slot to become available.
func TestLifecycleManager_ProbeSemaphore_CancelledContextAbortsWait(t *testing.T) {
	// Build a manager and fill its semaphore completely so the next Enqueue blocks.
	mgr := newLifecycleManagerForTest()
	mgr.addFunc = func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
		t.Fatal("dispatch should not run when context is cancelled")
		return "", nil
	}

	// Drain all slots from the semaphore to simulate maxConcurrentProbes in-flight.
	for i := 0; i < maxConcurrentProbes; i++ {
		<-mgr.probeSem
	}
	// Schedule slot return after a generous delay to confirm the test doesn't wait for it.
	go func() {
		time.Sleep(500 * time.Millisecond)
		for i := 0; i < maxConcurrentProbes; i++ {
			mgr.probeSem <- struct{}{}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	start := time.Now()
	_, _, err := mgr.Enqueue(ctx, &DownloadRequest{
		URL:  "http://127.0.0.1:0/dummy",
		Path: t.TempDir(),
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error due to cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// Should abort almost instantly, not wait for the delayed slot return.
	if elapsed > 200*time.Millisecond {
		t.Errorf("Enqueue took %v to abort — semaphore cancellation may be broken", elapsed)
	}
}
