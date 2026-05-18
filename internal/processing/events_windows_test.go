//go:build windows

package processing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/engine/types"
)

func TestFinalizeCompletedFile_OverwritesExistingDestinationOnWindows(t *testing.T) {
	tempDir := t.TempDir()
	finalPath := filepath.Join(tempDir, "video.mp4")
	surgePath := finalPath + types.IncompleteSuffix

	if err := os.WriteFile(finalPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to create existing final file: %v", err)
	}
	if err := os.WriteFile(surgePath, []byte("new"), 0o644); err != nil {
		t.Fatalf("failed to create working file: %v", err)
	}

	if err := finalizeCompletedFile(finalPath); err != nil {
		t.Fatalf("finalizeCompletedFile returned unexpected error: %v", err)
	}

	finalData, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("failed to read final file: %v", err)
	}
	if string(finalData) != "new" {
		t.Fatalf("final file content = %q, want %q", string(finalData), "new")
	}

	if _, err := os.Stat(surgePath); !os.IsNotExist(err) {
		t.Fatalf("expected working file to be removed after overwrite, stat err: %v", err)
	}
}
