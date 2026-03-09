package processing

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/surge-downloader/surge/internal/engine/types"
)

func TestFinalizeCompletedFile_CopiesAcrossDevicesOnEXDEV(t *testing.T) {
	tempDir := t.TempDir()
	finalPath := filepath.Join(tempDir, "video.mp4")
	surgePath := finalPath + types.IncompleteSuffix
	if err := os.WriteFile(surgePath, []byte("partial"), 0o644); err != nil {
		t.Fatalf("failed to create working file: %v", err)
	}

	origRename := renameCompletedFile
	origCopy := copyCompletedFile
	t.Cleanup(func() {
		renameCompletedFile = origRename
		copyCompletedFile = origCopy
	})

	var copied bool
	renameCompletedFile = func(string, string) error {
		return &os.LinkError{Op: "rename", Old: surgePath, New: finalPath, Err: syscall.EXDEV}
	}
	copyCompletedFile = func(src, dst string) error {
		copied = true
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	}

	if err := finalizeCompletedFile(finalPath); err != nil {
		t.Fatalf("finalizeCompletedFile failed: %v", err)
	}
	if !copied {
		t.Fatal("expected copy fallback to run on EXDEV")
	}

	data, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("failed to read finalized file: %v", err)
	}
	if string(data) != "partial" {
		t.Fatalf("final data = %q, want partial", string(data))
	}
	if _, err := os.Stat(surgePath); !os.IsNotExist(err) {
		t.Fatalf("expected working file removal after copy fallback, stat err: %v", err)
	}
}
