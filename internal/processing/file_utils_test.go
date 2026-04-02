package processing_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
)

func TestInferFilenameFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://test.com/file.zip", "file.zip"},
		{"http://test.com/file.zip?query=1", "file.zip"},
		{"http://test.com/download?filename=custom.zip", "custom.zip"},
		{"http://test.com/download?file=another.tar.gz", "another.tar.gz"},
		{"http://test.com/", ""},
		{"http://test.com", ""},
	}

	for _, tt := range tests {
		actual := processing.InferFilenameFromURL(tt.url)
		if actual != tt.expected {
			t.Errorf("InferFilenameFromURL(%q) = %q; want %q", tt.url, actual, tt.expected)
		}
	}
}

func TestGetUniqueFilename(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Doesn't exist
	if name := processing.GetUniqueFilename(tmpDir, "test.txt", nil); name != "test.txt" {
		t.Errorf("Expected test.txt, got %s", name)
	}

	// 2. Exists on disk
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if name := processing.GetUniqueFilename(tmpDir, "test.txt", nil); name != "test(1).txt" {
		t.Errorf("Expected test(1).txt, got %s", name)
	}

	// 3. Exists on disk with .surge
	if err := os.WriteFile(filepath.Join(tmpDir, "partial.zip"+types.IncompleteSuffix), []byte("data"), 0o644); err != nil {
		t.Fatalf("failed to create partial file: %v", err)
	}
	if name := processing.GetUniqueFilename(tmpDir, "partial.zip", nil); name != "partial(1).zip" {
		t.Errorf("Expected partial(1).zip, got %s", name)
	}

	// 4. Exists in active downloads function
	activeDownloads := func(dir, name string) bool {
		return dir == tmpDir && name == "memory.bin"
	}
	if name := processing.GetUniqueFilename(tmpDir, "memory.bin", activeDownloads); name != "memory(1).bin" {
		t.Errorf("Expected memory(1).bin, got %s", name)
	}

	// 5. Same filename in a different directory should not conflict
	otherDir := filepath.Join(tmpDir, "other")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("failed to create other dir: %v", err)
	}
	dirAwareActive := func(dir, name string) bool {
		return dir == otherDir && name == "video.mp4"
	}
	if name := processing.GetUniqueFilename(tmpDir, "video.mp4", dirAwareActive); name != "video.mp4" {
		t.Errorf("Expected video.mp4, got %s", name)
	}

	// 6. After exhausting 100 numbered candidates, return empty so the caller can fail cleanly
	overflowActive := func(dir, name string) bool {
		if dir != tmpDir {
			return false
		}
		if name == "overflow.bin" {
			return true
		}
		for i := 1; i <= 100; i++ {
			if name == "overflow("+strconv.Itoa(i)+").bin" {
				return true
			}
		}
		return false
	}
	if name := processing.GetUniqueFilename(tmpDir, "overflow.bin", overflowActive); name != "" {
		t.Errorf("Expected empty result after exhaustion, got %s", name)
	}
}

func TestGetCategoryPath(t *testing.T) {
	tmpDir := t.TempDir()

	settings := config.DefaultSettings()
	settings.General.CategoryEnabled = true
	settings.General.Categories = []config.Category{
		{
			Name:    "Images",
			Pattern: "\\.(jpg|png)$",
			Path:    filepath.Join(tmpDir, "Images"),
		},
	}

	// Match
	path, err := processing.GetCategoryPath("test.jpg", tmpDir, settings)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := filepath.Join(tmpDir, "Images")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}

	// No Match
	path, err = processing.GetCategoryPath("test.txt", tmpDir, settings)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if path != tmpDir {
		t.Errorf("Expected %s, got %s", tmpDir, path)
	}

	// Disabled
	settings.General.CategoryEnabled = false
	path, err = processing.GetCategoryPath("test.jpg", tmpDir, settings)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if path != tmpDir {
		t.Errorf("Expected %s, got %s", tmpDir, path)
	}

	// No side effects: routing should not create the directory before reservation.
	missingDir := filepath.Join(tmpDir, "missing")
	settings.General.CategoryEnabled = true
	settings.General.Categories = []config.Category{
		{
			Name:    "Programs",
			Pattern: `(?i)\.bin$`,
			Path:    missingDir,
		},
	}
	path, err = processing.GetCategoryPath("tool.bin", tmpDir, settings)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if path != missingDir {
		t.Fatalf("Expected %s, got %s", missingDir, path)
	}
	if _, err := os.Stat(missingDir); !os.IsNotExist(err) {
		t.Fatalf("expected routing to avoid creating %s, stat err: %v", missingDir, err)
	}
}

func TestResolveDestination_Priority(t *testing.T) {
	settings := config.DefaultSettings()
	settings.General.CategoryEnabled = false
	defaultDir := "/downloads"

	// 1. User defined beats all
	_, name, _ := processing.ResolveDestination("http://example.com/file.zip", "user.txt", defaultDir, false, settings, &processing.ProbeResult{Filename: "probe.zip"}, nil)
	if name != "user.txt" {
		t.Errorf("Expected user.txt as candidate priority, got %s", name)
	}

	// 2. Probe beats URL fallback
	_, name, _ = processing.ResolveDestination("http://example.com/file.zip", "", defaultDir, false, settings, &processing.ProbeResult{Filename: "probe.zip"}, nil)
	if name != "probe.zip" {
		t.Errorf("Expected probe.zip, got %s", name)
	}

	// 3. URL Fallback when probe is nil
	_, name, _ = processing.ResolveDestination("http://example.com/another.tar.gz", "", defaultDir, false, settings, nil, nil)
	if name != "another.tar.gz" {
		t.Errorf("Expected another.tar.gz, got %s", name)
	}

	// 4. URL Fallback when probe has empty filename
	_, name, _ = processing.ResolveDestination("http://example.com/some.rar", "", defaultDir, false, settings, &processing.ProbeResult{Filename: ""}, nil)
	if name != "some.rar" {
		t.Errorf("Expected some.rar, got %s", name)
	}
}

func TestResolveDestination_ErrorsWhenUniqueNameExhausted(t *testing.T) {
	settings := config.DefaultSettings()
	settings.General.CategoryEnabled = false

	overflowActive := func(dir, name string) bool {
		if name == "overflow.bin" {
			return true
		}
		for i := 1; i <= 100; i++ {
			if name == "overflow("+strconv.Itoa(i)+").bin" {
				return true
			}
		}
		return false
	}

	_, _, err := processing.ResolveDestination(
		"http://example.com/overflow.bin",
		"overflow.bin",
		"/downloads",
		false,
		settings,
		nil,
		overflowActive,
	)
	if err == nil {
		t.Fatal("expected unique-name exhaustion error")
	}
}
