package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/adrg/xdg"
)

func TestGetRuntimeDir_FallsBackToStateDirWhenXDGUnsetOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-specific behavior")
	}

	tmp := t.TempDir()
	oldStateHome := xdg.StateHome
	oldRuntimeDir := xdg.RuntimeDir
	xdg.StateHome = tmp
	xdg.RuntimeDir = filepath.Join(tmp, "xdg-runtime")
	t.Cleanup(func() {
		xdg.StateHome = oldStateHome
		xdg.RuntimeDir = oldRuntimeDir
	})

	t.Setenv("XDG_RUNTIME_DIR", "")

	got := GetRuntimeDir()
	want := filepath.Join(GetStateDir(), "runtime")
	if got != want {
		t.Fatalf("GetRuntimeDir() = %q, want %q", got, want)
	}
}

func TestGetRuntimeDir_UsesXDGWhenSet(t *testing.T) {
	tmp := t.TempDir()

	oldRuntimeDir := xdg.RuntimeDir
	xdg.RuntimeDir = tmp
	t.Cleanup(func() {
		xdg.RuntimeDir = oldRuntimeDir
	})

	t.Setenv("XDG_RUNTIME_DIR", tmp)

	got := GetRuntimeDir()
	want := filepath.Join(tmp, "surge")
	if got != want {
		t.Fatalf("GetRuntimeDir() = %q, want %q", got, want)
	}
}

func TestGetDownloadsDir_FallbackBehavior(t *testing.T) {
	tmp := t.TempDir()

	oldDownload := xdg.UserDirs.Download
	xdg.UserDirs.Download = filepath.Join(tmp, "missing-downloads-dir")
	t.Cleanup(func() {
		xdg.UserDirs.Download = oldDownload
	})

	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	if got := GetDownloadsDir(); got != "" {
		t.Fatalf("GetDownloadsDir() = %q, want empty for missing dirs", got)
	}

	downloadsDir := filepath.Join(tmp, "Downloads")
	if err := os.MkdirAll(downloadsDir, 0o755); err != nil {
		t.Fatalf("failed to create fallback downloads dir: %v", err)
	}

	if got := GetDownloadsDir(); got != downloadsDir {
		t.Fatalf("GetDownloadsDir() = %q, want %q", got, downloadsDir)
	}
}

func TestWindowsPathsKeepLegacyAppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific behavior")
	}

	tmp := t.TempDir()
	oldConfigHome := xdg.ConfigHome
	oldStateHome := xdg.StateHome
	xdg.ConfigHome = filepath.Join(tmp, "xdg-config")
	xdg.StateHome = filepath.Join(tmp, "xdg-state")
	t.Cleanup(func() {
		xdg.ConfigHome = oldConfigHome
		xdg.StateHome = oldStateHome
	})

	t.Setenv("APPDATA", tmp)

	want := filepath.Join(tmp, "surge")
	if got := GetSurgeDir(); got != want {
		t.Fatalf("GetSurgeDir() = %q, want %q", got, want)
	}
	if got := GetStateDir(); got != want {
		t.Fatalf("GetStateDir() = %q, want %q", got, want)
	}
}
