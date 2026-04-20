package backup

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/state"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/testutil"
)

func setupBackupEnv(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("APPDATA", filepath.Join(root, "config"))

	if err := config.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	state.CloseDB()
	state.Configure(filepath.Join(config.GetStateDir(), "surge.db"))
	if _, err := state.GetDB(); err != nil {
		t.Fatalf("state.GetDB failed: %v", err)
	}
	t.Cleanup(state.CloseDB)
	return root
}

func seedPausedDownload(t *testing.T, downloadRoot string) (string, string) {
	t.Helper()

	destPath := filepath.Join(downloadRoot, "nested", "video.bin")
	if err := config.SaveSettings(&config.Settings{
		General: config.GeneralSettings{
			DefaultDownloadDir: downloadRoot,
		},
		Network:     config.DefaultSettings().Network,
		Performance: config.DefaultSettings().Performance,
		Categories:  config.DefaultSettings().Categories,
		Extension:   config.DefaultSettings().Extension,
	}); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if _, err := testutil.CreateSurgeFile(filepath.Dir(destPath), "video.bin", 1024, 256); err != nil {
		t.Fatalf("CreateSurgeFile failed: %v", err)
	}

	saved := &types.DownloadState{
		ID:         "paused-download",
		URL:        "https://example.com/video.bin",
		DestPath:   destPath,
		Filename:   "video.bin",
		TotalSize:  1024,
		Downloaded: 256,
		Tasks: []types.Task{
			{Offset: 256, Length: 768},
		},
		CreatedAt: 1,
		PausedAt:  2,
		Elapsed:   3,
	}
	if err := state.SaveStateWithOptions(saved.URL, saved.DestPath, saved, state.SaveStateOptions{SkipFileHash: true}); err != nil {
		t.Fatalf("SaveStateWithOptions failed: %v", err)
	}
	return saved.URL, destPath
}

func TestApplyImport_WithoutPartialsDowngradesPausedDownloads(t *testing.T) {
	root := setupBackupEnv(t)
	sourceRoot := filepath.Join(root, "source-downloads")
	url, _ := seedPausedDownload(t, sourceRoot)

	var buf bytes.Buffer
	if _, err := Export(t.Context(), &buf, ExportOptions{}, nil); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	importRoot := filepath.Join(root, "imported")
	result, err := ApplyImport(t.Context(), bytes.NewReader(buf.Bytes()), ImportOptions{
		RootDir: importRoot,
		Replace: true,
	}, nil)
	if err != nil {
		t.Fatalf("ApplyImport failed: %v", err)
	}
	if result.Preview.ResumableJobsDowngradedToQueue != 1 {
		t.Fatalf("downgraded=%d, want 1", result.Preview.ResumableJobsDowngradedToQueue)
	}

	entry, err := state.GetDownload("paused-download")
	if err != nil {
		t.Fatalf("GetDownload failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected imported entry")
	}
	if entry.Status != "queued" {
		t.Fatalf("status=%q, want queued", entry.Status)
	}
	if entry.Downloaded != 0 {
		t.Fatalf("downloaded=%d, want 0", entry.Downloaded)
	}

	wantDest := filepath.Join(importRoot, "nested", "video.bin")
	if filepath.Clean(entry.DestPath) != filepath.Clean(wantDest) {
		t.Fatalf("dest=%q, want %q", entry.DestPath, wantDest)
	}
	saved, err := state.LoadState(url, wantDest)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if saved == nil {
		t.Fatal("expected queued metadata to remain importable")
	}
	if len(saved.Tasks) != 0 {
		t.Fatalf("tasks=%d, want 0 without exported partial state", len(saved.Tasks))
	}
}

func TestPreviewImport_UsesImportOptions(t *testing.T) {
	root := setupBackupEnv(t)
	sourceRoot := filepath.Join(root, "source-downloads")
	url, _ := seedPausedDownload(t, sourceRoot)

	var buf bytes.Buffer
	if _, err := Export(t.Context(), &buf, ExportOptions{}, nil); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	importRoot := filepath.Join(root, "imported")
	if err := state.AddToMasterList(types.DownloadEntry{
		ID:       "existing-download",
		URL:      url,
		URLHash:  state.URLHash(url),
		DestPath: filepath.Join(importRoot, "nested", "video.bin"),
		Filename: "video.bin",
		Status:   "queued",
	}); err != nil {
		t.Fatalf("AddToMasterList failed: %v", err)
	}

	previewMerge, err := PreviewImport(t.Context(), bytes.NewReader(buf.Bytes()), ImportOptions{
		RootDir: importRoot,
	})
	if err != nil {
		t.Fatalf("PreviewImport merge failed: %v", err)
	}
	if filepath.Clean(previewMerge.RootDir) != filepath.Clean(importRoot) {
		t.Fatalf("merge root=%q, want %q", previewMerge.RootDir, importRoot)
	}
	if previewMerge.DuplicatesSkipped != 1 {
		t.Fatalf("merge duplicates=%d, want 1", previewMerge.DuplicatesSkipped)
	}

	previewReplace, err := PreviewImport(t.Context(), bytes.NewReader(buf.Bytes()), ImportOptions{
		RootDir: importRoot,
		Replace: true,
	})
	if err != nil {
		t.Fatalf("PreviewImport replace failed: %v", err)
	}
	if previewReplace.DuplicatesSkipped != 0 {
		t.Fatalf("replace duplicates=%d, want 0", previewReplace.DuplicatesSkipped)
	}
	if previewReplace.ImportsByStatus["queued"] != 1 {
		t.Fatalf("replace queued imports=%d, want 1", previewReplace.ImportsByStatus["queued"])
	}
}

func TestApplyImport_WithPartialsRestoresPausedState(t *testing.T) {
	root := setupBackupEnv(t)
	sourceRoot := filepath.Join(root, "source-downloads")
	url, _ := seedPausedDownload(t, sourceRoot)

	var buf bytes.Buffer
	if _, err := Export(t.Context(), &buf, ExportOptions{IncludePartials: true}, nil); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	importRoot := filepath.Join(root, "imported")
	_, err := ApplyImport(t.Context(), bytes.NewReader(buf.Bytes()), ImportOptions{
		RootDir: importRoot,
		Replace: true,
	}, nil)
	if err != nil {
		t.Fatalf("ApplyImport failed: %v", err)
	}

	wantDest := filepath.Join(importRoot, "nested", "video.bin")
	entry, err := state.GetDownload("paused-download")
	if err != nil {
		t.Fatalf("GetDownload failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected imported entry")
	}
	if entry.Status != "paused" {
		t.Fatalf("status=%q, want paused", entry.Status)
	}

	saved, err := state.LoadState(url, wantDest)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if saved == nil {
		t.Fatal("expected restored resumable state")
	}
	if saved.Downloaded != 256 {
		t.Fatalf("downloaded=%d, want 256", saved.Downloaded)
	}
	if !testutil.FileExists(wantDest + types.IncompleteSuffix) {
		t.Fatalf("expected restored partial file %s", wantDest+types.IncompleteSuffix)
	}
}

func TestApplyImport_WithLogsRestoresLogFiles(t *testing.T) {
	setupBackupEnv(t)

	logDir := config.GetLogsDir()
	originalOne := []byte("first log line\n")
	originalTwo := []byte("second log line\n")
	if err := os.WriteFile(filepath.Join(logDir, "session-1.log"), originalOne, 0o644); err != nil {
		t.Fatalf("WriteFile session-1 failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "session-2.log"), originalTwo, 0o644); err != nil {
		t.Fatalf("WriteFile session-2 failed: %v", err)
	}

	var buf bytes.Buffer
	if _, err := Export(t.Context(), &buf, ExportOptions{IncludeLogs: true}, nil); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "session-1.log"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile stale session-1 failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "stale.log"), []byte("remove me"), 0o644); err != nil {
		t.Fatalf("WriteFile stale log failed: %v", err)
	}

	result, err := ApplyImport(t.Context(), bytes.NewReader(buf.Bytes()), ImportOptions{Replace: true}, nil)
	if err != nil {
		t.Fatalf("ApplyImport failed: %v", err)
	}
	if result.LogsRestored != 2 {
		t.Fatalf("logs restored=%d, want 2", result.LogsRestored)
	}

	gotOne, err := os.ReadFile(filepath.Join(logDir, "session-1.log"))
	if err != nil {
		t.Fatalf("ReadFile session-1 failed: %v", err)
	}
	if !bytes.Equal(gotOne, originalOne) {
		t.Fatalf("session-1 content=%q, want %q", string(gotOne), string(originalOne))
	}

	gotTwo, err := os.ReadFile(filepath.Join(logDir, "session-2.log"))
	if err != nil {
		t.Fatalf("ReadFile session-2 failed: %v", err)
	}
	if !bytes.Equal(gotTwo, originalTwo) {
		t.Fatalf("session-2 content=%q, want %q", string(gotTwo), string(originalTwo))
	}

	if _, err := os.Stat(filepath.Join(logDir, "stale.log")); !os.IsNotExist(err) {
		t.Fatalf("stale.log should be removed, stat err=%v", err)
	}
}

func TestRestoreBundleFile_RejectsPathOutsideAllowedRoot(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("logs/test.log")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if _, err := w.Write([]byte("log")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	allowedRoot := t.TempDir()
	destPath := filepath.Join(allowedRoot, "..", "escape.log")
	if err := restoreBundleFile(reader, "logs/test.log", destPath, allowedRoot); err == nil {
		t.Fatal("expected restoreBundleFile to reject paths outside the allowed root")
	}
}
