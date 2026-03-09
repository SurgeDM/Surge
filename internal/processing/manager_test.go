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

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/types"
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
	return &LifecycleManager{settings: settings}
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
