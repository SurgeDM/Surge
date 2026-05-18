package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/download"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
)

func TestHandleDownload_PathResolution(t *testing.T) {
	// Setup temporary directory for mocking XDG_CONFIG_HOME
	tempDir, err := os.MkdirTemp("", "surge-test-home")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	origLifecycle := GlobalLifecycle
	origLifecycleCleanup := GlobalLifecycleCleanup
	origService := GlobalService
	t.Cleanup(func() {
		GlobalLifecycle = origLifecycle
		GlobalLifecycleCleanup = origLifecycleCleanup
		GlobalService = origService
	})
	GlobalLifecycle = nil
	GlobalLifecycleCleanup = nil
	GlobalService = nil

	// Ensure a clean state DB for the test scope.
	state.CloseDB()
	state.Configure(filepath.Join(tempDir, "surge.db"))
	defer state.CloseDB()

	// Mock XDG_CONFIG_HOME to affect GetSurgeDir() on Linux
	originalConfigHome := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", tempDir)
	defer func() {
		if originalConfigHome == "" {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			_ = os.Setenv("XDG_CONFIG_HOME", originalConfigHome)
		}
	}()

	// Create surge config directory
	surgeConfigDir := filepath.Join(tempDir, "surge")
	if err := os.MkdirAll(surgeConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Setup default download directory
	defaultDownloadDir := filepath.Join(tempDir, "Downloads")
	if err := os.MkdirAll(defaultDownloadDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a temporary settings file
	settings := config.DefaultSettings()
	settings.General.DefaultDownloadDir = defaultDownloadDir
	settings.Extension.ExtensionPrompt = false

	if err := config.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	// Initialize GlobalPool (required by handleDownload)
	GlobalPool = download.NewWorkerPool(nil, 1)

	tests := []struct {
		name               string
		request            DownloadRequest
		expectedOutputPath string
	}{
		{
			name: "Absolute Path (Explicit)",
			request: DownloadRequest{
				URL:  "http://example.com/file1",
				Path: filepath.Join(tempDir, "absolute"),
			},
			expectedOutputPath: filepath.Join(tempDir, "absolute"),
		},
		{
			name: "Relative Path (No Flag)",
			request: DownloadRequest{
				URL:  "http://example.com/file2",
				Path: "relative",
			},
			expectedOutputPath: filepath.Join(mustGetwd(t), "relative"),
		},
		{
			name: "Relative to Default Dir",
			request: DownloadRequest{
				URL:                  "http://example.com/file3",
				Path:                 "subdir",
				RelativeToDefaultDir: true,
			},
			expectedOutputPath: filepath.Join(defaultDownloadDir, "subdir"),
		},
		{
			name: "Nested Relative to Default Dir",
			request: DownloadRequest{
				URL:                  "http://example.com/file4",
				Path:                 "nested/deep",
				RelativeToDefaultDir: true,
			},
			expectedOutputPath: filepath.Join(defaultDownloadDir, "nested", "deep"),
		},
		{
			name: "Empty Path (Default)",
			request: DownloadRequest{
				URL:  "http://example.com/file5",
				Path: "",
			},
			expectedOutputPath: defaultDownloadDir,
		},
		{
			name: "Windows Download Root Maps To Default Dir",
			request: DownloadRequest{
				URL:  "http://example.com/file6",
				Path: "C:/Users/me/Downloads",
			},
			expectedOutputPath: defaultDownloadDir,
		},
		{
			name: "Windows Nested Path Maps Under Default Dir",
			request: DownloadRequest{
				URL:  "http://example.com/file7",
				Path: "C:/Users/me/Downloads/surge-repro",
			},
			expectedOutputPath: filepath.Join(defaultDownloadDir, "surge-repro"),
		},
		{
			name: "Windows Nested Path Relative Flag Maps Under Default Dir",
			request: DownloadRequest{
				URL:                  "http://example.com/file8",
				Path:                 "C:/Users/me/Downloads/surge-repro",
				RelativeToDefaultDir: true,
			},
			expectedOutputPath: filepath.Join(defaultDownloadDir, "surge-repro"),
		},
		{
			name: "Unmatched Windows Path Falls Back To Default Dir",
			request: DownloadRequest{
				URL:                  "http://example.com/file9",
				Path:                 "E:/Torrents/complete",
				RelativeToDefaultDir: true,
			},
			expectedOutputPath: defaultDownloadDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/download", bytes.NewBuffer(body))
			w := httptest.NewRecorder()
			svc := core.NewLocalDownloadService(GlobalPool)

			// We pass defaultDownloadDir as a fallback to handleDownload, but since we mocked settings,
			// it should prioritize settings.General.DefaultDownloadDir
			handleDownload(w, req, defaultDownloadDir, svc)

			if w.Code != http.StatusOK && w.Code != http.StatusConflict {
				t.Errorf("Expected OK, got %d. Body: %s", w.Code, w.Body.String())
			}

			// GlobalPool access
			configs := GlobalPool.GetAll()
			found := false
			for _, cfg := range configs {
				if cfg.URL == tt.request.URL {
					found = true
					t.Logf("OutputPath for %s: %s", tt.name, cfg.OutputPath)

					if !filepath.IsAbs(cfg.OutputPath) {
						t.Errorf("Expected absolute path, got %s", cfg.OutputPath)
					}

					if cfg.OutputPath != tt.expectedOutputPath {
						t.Errorf("Expected path %s, got %s", tt.expectedOutputPath, cfg.OutputPath)
					}
					break
				}
			}
			if !found {
				t.Errorf("Download was not queued")
			}
		})
	}
}

func TestShouldFallbackUnmappedWindowsPath(t *testing.T) {
	tests := []struct {
		name                 string
		relativeToDefaultDir bool
		hostOS               string
		want                 bool
	}{
		{
			name:                 "relative request falls back on windows",
			relativeToDefaultDir: true,
			hostOS:               "windows",
			want:                 true,
		},
		{
			name:                 "relative request falls back on linux",
			relativeToDefaultDir: true,
			hostOS:               "linux",
			want:                 true,
		},
		{
			name:                 "explicit request does not fall back on windows",
			relativeToDefaultDir: false,
			hostOS:               "windows",
			want:                 false,
		},
		{
			name:                 "explicit request falls back on linux",
			relativeToDefaultDir: false,
			hostOS:               "linux",
			want:                 true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldFallbackUnmappedWindowsPath(tt.relativeToDefaultDir, tt.hostOS); got != tt.want {
				t.Fatalf("shouldFallbackUnmappedWindowsPath(%v, %q) = %v, want %v", tt.relativeToDefaultDir, tt.hostOS, got, tt.want)
			}
		})
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	return wd
}

func TestHandleDownload_SkipApprovalUsesLifecycleEnqueue(t *testing.T) {
	setupIsolatedCmdState(t)

	progressCh := make(chan any, 10)
	GlobalProgressCh = progressCh
	GlobalPool = download.NewWorkerPool(progressCh, 1)

	origLifecycle := GlobalLifecycle
	origService := GlobalService
	t.Cleanup(func() {
		GlobalLifecycle = origLifecycle
		GlobalService = origService
		GlobalPool = nil
		GlobalProgressCh = nil
	})

	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=0-0" {
			t.Fatalf("Range header = %q, want bytes=0-0", got)
		}
		w.Header().Set("Content-Range", "bytes 0-0/7")
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer probeServer.Close()

	tempDir := t.TempDir()
	expectedFile := "from-extension.bin"

	var addCalls int
	GlobalLifecycle = processing.NewLifecycleManager(func(url, path, filename string, _ []string, headers map[string]string, explicit bool, totalSize int64, supportsRange bool) (string, error) {
		addCalls++
		if url != probeServer.URL {
			t.Fatalf("url = %q, want %q", url, probeServer.URL)
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
		if totalSize != 7 {
			t.Fatalf("totalSize = %d, want 7", totalSize)
		}
		if !supportsRange {
			t.Fatal("expected probe to preserve range support")
		}
		if headers["Authorization"] != "Bearer test" {
			t.Fatalf("headers were not forwarded to lifecycle addFunc")
		}

		surgePath := filepath.Join(path, filename) + types.IncompleteSuffix
		if _, err := os.Stat(surgePath); err != nil {
			t.Fatalf("expected pre-created working file before addFunc: %v", err)
		}

		return "queued-id", nil
	}, nil)

	svc := core.NewLocalDownloadService(nil)
	GlobalService = svc
	t.Cleanup(func() {
		_ = svc.Shutdown()
	})

	body := fmt.Sprintf(`{
		"url": %q,
		"filename": %q,
		"path": %q,
		"skip_approval": true,
		"is_explicit_category": true,
		"headers": {"Authorization": "Bearer test"}
	}`, probeServer.URL, expectedFile, tempDir)

	req := httptest.NewRequest(http.MethodPost, "/download", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handleDownload(rec, req, "", svc)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if addCalls != 1 {
		t.Fatalf("expected lifecycle addFunc to be called once, got %d", addCalls)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["id"] != "queued-id" {
		t.Fatalf("response id = %q, want queued-id", resp["id"])
	}
}

func TestHandleDownload_EnqueueError_RecordsPreflightError(t *testing.T) {
	setupIsolatedCmdState(t)

	progressCh := make(chan any, 10)
	GlobalProgressCh = progressCh
	GlobalPool = download.NewWorkerPool(progressCh, 1)

	origLifecycle := GlobalLifecycle
	origService := GlobalService
	t.Cleanup(func() {
		GlobalLifecycle = origLifecycle
		GlobalService = origService
		GlobalPool = nil
		GlobalProgressCh = nil
	})

	// Create a lifecycle manager whose addFunc should never be reached
	// because the probe will fail first (invalid URL scheme).
	GlobalLifecycle = processing.NewLifecycleManager(func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
		t.Fatal("addFunc should not be called when probe fails")
		return "", nil
	}, nil)

	svc := core.NewLocalDownloadService(nil)
	GlobalService = svc
	t.Cleanup(func() {
		_ = svc.Shutdown()
	})

	// Use a URL with an invalid scheme so ProbeServer fails immediately.
	body := `{"url": "badscheme://example.com/file.bin", "path": "/tmp", "skip_approval": true}`
	req := httptest.NewRequest(http.MethodPost, "/download", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handleDownload(rec, req, "", svc)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify that the error was persisted in the master list.
	list, err := state.LoadMasterList()
	if err != nil {
		t.Fatalf("LoadMasterList failed: %v", err)
	}

	found := false
	for _, entry := range list.Downloads {
		if strings.Contains(entry.URL, "badscheme://example.com/file.bin") && entry.Status == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected errored download entry in master list after probe failure via HTTP API")
	}
}

type failingPublishService struct {
	fakeRemoteDownloadService
	publishErr error
}

func (f *failingPublishService) Publish(msg interface{}) error {
	return f.publishErr
}

func TestHandleDownload_PublishError_RecordsPreflightError(t *testing.T) {
	setupIsolatedCmdState(t)

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

	GlobalPool = download.NewWorkerPool(nil, 1)

	origServerProgram := serverProgram
	serverProgram = &tea.Program{}
	t.Cleanup(func() { serverProgram = origServerProgram })

	settings := config.DefaultSettings()
	settings.Extension.ExtensionPrompt = true
	settings.General.WarnOnDuplicate = false
	if err := config.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc := &failingPublishService{publishErr: errors.New("publish failed")}
	GlobalService = svc

	outDir := t.TempDir()
	body := fmt.Sprintf(`{"url": %q, "path": %q}`, "http://example.com/file.bin", outDir)
	req := httptest.NewRequest(http.MethodPost, "/download", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handleDownload(rec, req, "", svc)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}

	list, err := state.LoadMasterList()
	if err != nil {
		t.Fatalf("LoadMasterList failed: %v", err)
	}

	found := false
	for _, entry := range list.Downloads {
		if strings.Contains(entry.URL, "http://example.com/file.bin") && entry.Status == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected errored download entry in master list after publish failure via HTTP API")
	}
}
