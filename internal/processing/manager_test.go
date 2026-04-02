package processing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/types"
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
	settings.General.CategoryEnabled = false
	return &LifecycleManager{settings: settings, settingsRefreshedAt: time.Now()}
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

	id, err := mgr.Enqueue(context.Background(), req)
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

	id, err := mgr.EnqueueWithID(context.Background(), req, expectedID)
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

	_, err := mgr.Enqueue(context.Background(), &DownloadRequest{
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

	id, err := mgr.Enqueue(context.Background(), &DownloadRequest{
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

	id, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
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

	_, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
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

	_, err := mgr.Enqueue(context.Background(), &DownloadRequest{
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
	initial.General.CategoryEnabled = false
	if err := config.SaveSettings(initial); err != nil {
		t.Fatalf("SaveSettings(initial) failed: %v", err)
	}

	mgr := NewLifecycleManager(nil, nil)

	updated := config.DefaultSettings()
	updated.General.CategoryEnabled = true
	if err := config.SaveSettings(updated); err != nil {
		t.Fatalf("SaveSettings(updated) failed: %v", err)
	}

	origTTL := settingsRefreshTTL
	t.Cleanup(func() {
		settingsRefreshTTL = origTTL
	})
	settingsRefreshTTL = 0

	settings := mgr.GetSettings()
	if !settings.General.CategoryEnabled {
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

	_, err := mgr.Enqueue(ctx, &DownloadRequest{
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

	_, err := mgr.EnqueueWithID(context.Background(), &DownloadRequest{
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
	_, err := mgr.Enqueue(context.Background(), &DownloadRequest{
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
	_, err := mgr.Enqueue(context.Background(), &DownloadRequest{
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

	_, err := mgr.Enqueue(ctx, &DownloadRequest{
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
